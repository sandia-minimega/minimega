// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Limits on what one cc client can send. They apply to all clients, under
// both cc policies.
const (
	// MaxMessageSize is the maximum encoded size of one message from a
	// client.
	MaxMessageSize = 16 << 20

	// MaxHandshakeSize is the maximum encoded size of the handshake message
	// from a client.
	MaxHandshakeSize = 256 << 10

	// LargeMessageSize is the encoded size above which a message from a
	// client must wait for one of MaxLargeMessages decode slots, which all
	// clients share. A client must send the rest of the message in
	// LargeMessageTimeout.
	LargeMessageSize    = 1 << 20
	MaxLargeMessages    = 2
	LargeMessageTimeout = 30 * time.Second

	// MaxQueuedResponses is the maximum encoded size of the client updates
	// that wait for the response handler, for all clients.
	MaxQueuedResponses = 64 << 20

	// WriteTimeout is the time that a write to a client can take. minimega
	// closes a client that does not read its messages in this time.
	WriteTimeout = 15 * time.Second

	// MaxFileChunk is the maximum data in one file message from a client.
	MaxFileChunk = 2 * PART_SIZE

	// MaxUploadFileSize is the maximum size of one file that a client sends.
	MaxUploadFileSize = 1 << 30

	// MaxUploadPerHour is the maximum number of bytes that one client can
	// write to the responses directory (files, stdout, and stderr) in one
	// clock hour, counted from its first write in that hour.
	MaxUploadPerHour = 4 << 30

	// MaxUploadEntriesPerHour is the maximum number of files that one client
	// can start to send in the same hour. Each directory in the name of a
	// file also counts as one.
	MaxUploadEntriesPerHour = 65536

	// MaxVMTags and MaxVMTagBytes are the maximum number of tags and the
	// maximum total size of the tag keys and values that the clients can
	// set on one VM.
	MaxVMTags     = 1024
	MaxVMTagBytes = 1 << 20

	// MaxTunnelStreams is the maximum number of open tunnel streams for one
	// client.
	MaxTunnelStreams = 256

	// MaxPipes is the maximum number of open pipe readers and writers for one
	// client.
	MaxPipes = 64

	// MaxBoundConns is the maximum number of open connections on one listener
	// that ListenUnixBound creates.
	MaxBoundConns = 4

	// HandshakeTimeout is the time that a client connection accepted on a
	// listener has to complete the handshake.
	HandshakeTimeout = 30 * time.Second
)

// cc policies, returned by the optional CCPolicy method of a VM.
const (
	CCPolicyStrict = "strict"
	CCPolicyLegacy = "legacy"
)

const (
	maxNameLen     = 4096
	maxFieldLen    = 256
	maxTagValueLen = 64 << 10
	maxListLen     = 1024
	maxProcesses   = 4096
	maxResponses   = 4096
	maxErrorLog    = 256

	refuseLogInterval = time.Minute
)

// ccPolicyVM is implemented by VMs that set their own cc policy. VMs that do
// not implement it use the legacy policy.
type ccPolicyVM interface {
	CCPolicy() string
}

var (
	errInvalidMessage  = errors.New("invalid client message")
	errMessageTooLarge = errors.New("client message too large")
)

var uuidRE = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func invalidf(format string, arg ...interface{}) error {
	return fmt.Errorf("%w: %v", errInvalidMessage, fmt.Sprintf(format, arg...))
}

// writeTimeout holds WriteTimeout. Tests change it while goroutines of other
// tests can still write to clients, so it is atomic.
var writeTimeout = newDuration(WriteTimeout)

func newDuration(d time.Duration) *atomic.Int64 {
	v := new(atomic.Int64)
	v.Store(int64(d))

	return v
}

// largeMessages holds the decode slots for messages above LargeMessageSize.
var largeMessages = make(chan struct{}, MaxLargeMessages)

// limitReader returns errMessageTooLarge when more than limit bytes are read
// between two calls to reset. When slots is set, a read past small bytes
// first takes a slot, which release gives back.
type limitReader struct {
	r     io.Reader
	limit int64
	n     int64

	small int64
	slots chan struct{}
	done  <-chan struct{}
	held  bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errMessageTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}

	if l.slots != nil && !l.held {
		read := l.limit - l.n
		if read >= l.small {
			if err := l.acquire(); err != nil {
				return 0, err
			}
		} else if int64(len(p)) > l.small-read {
			p = p[:l.small-read]
		}
	}

	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func (l *limitReader) acquire() error {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return io.ErrClosedPipe
	}
	l.held = true

	if d, ok := l.r.(interface{ SetReadDeadline(time.Time) error }); ok {
		d.SetReadDeadline(time.Now().Add(LargeMessageTimeout))
	}
	return nil
}

// release gives back the slot that the current message holds, if any.
func (l *limitReader) release() {
	if !l.held {
		return
	}
	l.held = false
	<-l.slots

	if d, ok := l.r.(interface{ SetReadDeadline(time.Time) error }); ok {
		d.SetReadDeadline(time.Time{})
	}
}

// size returns the number of bytes read since the last reset.
func (l *limitReader) size() int64 {
	return l.limit - l.n
}

func (l *limitReader) reset() {
	l.release()
	l.n = l.limit
}

// rateLimiter allows one event for each key in each refuseLogInterval.
type rateLimiter struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// refusals limits the log entries for refused or malformed client requests.
var refusals rateLimiter

func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if t, ok := r.last[key]; ok && now.Sub(t) < refuseLogInterval {
		return false
	}

	if r.last == nil {
		r.last = make(map[string]time.Time)
	} else if len(r.last) > 4096 {
		for k, t := range r.last {
			if now.Sub(t) >= refuseLogInterval {
				delete(r.last, k)
			}
		}
	}

	r.last[key] = now
	return true
}

// warnLimited logs a warning for key at most once per refuseLogInterval.
func warnLimited(key, format string, arg ...interface{}) {
	if refusals.allow(key) {
		log.Warn(format, arg...)
	}
}

// recoverConn recovers a panic in a connection handler, logs it, and closes
// the connection. It must be deferred directly.
func recoverConn(what string, conn io.Closer) {
	if r := recover(); r != nil {
		log.Error("%v: recovered from panic: %v\n%s", what, r, debug.Stack())
		if conn != nil {
			conn.Close()
		}
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// validField reports whether s is a short, printable identifier.
func validField(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return false
		}
	}
	return true
}

func validValue(s string, max int) bool {
	return len(s) <= max && !strings.ContainsRune(s, 0)
}

// validToken reports whether u is safe to use as one path element.
func validToken(u string) bool {
	if u == "" || len(u) > 128 || u == "." || u == ".." {
		return false
	}
	for _, r := range u {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func validFileName(name string) bool {
	return len(name) <= maxNameLen && !strings.ContainsRune(name, 0) && filepath.IsLocal(name)
}

func validPipeName(name string) bool {
	return name != "" && validField(name, maxFieldLen) && !strings.Contains(name, "/")
}

// validateClient checks the client-supplied fields of cl, except the UUID.
func validateClient(cl *Client) error {
	if cl == nil {
		return invalidf("missing client")
	}

	for name, v := range map[string]string{
		"hostname": cl.Hostname,
		"arch":     cl.Arch,
		"os":       cl.OS,
		"version":  cl.Version,
		"sha256":   cl.SHA256,
	} {
		if !validField(v, maxFieldLen) {
			return invalidf("%v %q", name, truncate(v, maxErrorLog))
		}
	}

	if len(cl.IPs) > maxListLen || len(cl.MACs) > maxListLen {
		return invalidf("%d IPs and %d MACs", len(cl.IPs), len(cl.MACs))
	}
	for _, v := range append(append([]string{}, cl.IPs...), cl.MACs...) {
		if !validField(v, maxFieldLen) {
			return invalidf("address %q", truncate(v, maxErrorLog))
		}
	}

	if len(cl.Capabilities) > maxListLen {
		return invalidf("%d capabilities", len(cl.Capabilities))
	}
	for _, v := range cl.Capabilities {
		if !validField(v, maxFieldLen) {
			return invalidf("capability %q", truncate(v, maxErrorLog))
		}
	}

	if len(cl.Processes) > maxProcesses {
		return invalidf("%d processes", len(cl.Processes))
	}
	for pid, p := range cl.Processes {
		if p == nil {
			return invalidf("empty process %d", pid)
		}
	}

	if len(cl.Tags) > maxListLen {
		return invalidf("%d tags", len(cl.Tags))
	}
	for k, v := range cl.Tags {
		if !validField(k, maxFieldLen) || !validValue(v, maxTagValueLen) {
			return invalidf("tag %q", truncate(k, maxErrorLog))
		}
	}

	if len(cl.Responses) > maxResponses {
		return invalidf("%d responses", len(cl.Responses))
	}
	for _, r := range cl.Responses {
		if r == nil || r.ID < 0 {
			return invalidf("response")
		}
	}

	return nil
}

// uuidMatches reports whether claimed is expected or its mangled form.
func uuidMatches(claimed, expected string) bool {
	if strings.EqualFold(claimed, expected) {
		return true
	}

	return uuidRE.MatchString(claimed) && strings.EqualFold(unmangle(claimed), expected)
}

// vmPolicy returns the cc policy of vm.
func vmPolicy(vm VM) string {
	if pv, ok := vm.(ccPolicyVM); ok && pv.CCPolicy() == CCPolicyStrict {
		return CCPolicyStrict
	}

	return CCPolicyLegacy
}

type uploadWindow struct {
	start   time.Time
	bytes   int64
	entries int64
}

// window returns the current upload window of uuid. The caller must hold
// uploadLock.
func (s *Server) window(uuid string) *uploadWindow {
	if s.uploads == nil {
		s.uploads = make(map[string]*uploadWindow)
	}

	now := time.Now()
	w := s.uploads[uuid]
	if w == nil || now.Sub(w.start) >= time.Hour {
		w = &uploadWindow{start: now}
		s.uploads[uuid] = w
	}

	return w
}

// chargeUpload counts n bytes against the hourly upload limit of uuid. It
// returns false when the limit does not allow them.
func (s *Server) chargeUpload(uuid string, n int64) bool {
	s.uploadLock.Lock()
	defer s.uploadLock.Unlock()

	w := s.window(uuid)
	if w.bytes+n > MaxUploadPerHour {
		return false
	}

	w.bytes += n
	return true
}

// chargeEntries counts n files and directories against the hourly limit of
// uuid. It returns false when the limit does not allow them.
func (s *Server) chargeEntries(uuid string, n int64) bool {
	s.uploadLock.Lock()
	defer s.uploadLock.Unlock()

	w := s.window(uuid)
	if w.entries+n > MaxUploadEntriesPerHour {
		return false
	}

	w.entries += n
	return true
}

func reverseKey(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// allowReverse records a reverse tunnel that the operator created, so that
// the strict policy accepts the CONNECT requests that it causes.
func (c *client) allowReverse(host string, port int) {
	c.reverseLock.Lock()
	defer c.reverseLock.Unlock()

	if c.reverses == nil {
		c.reverses = make(map[string]int)
	}
	c.reverses[reverseKey(host, port)]++
}

func (c *client) disallowReverse(host string, port int) {
	c.reverseLock.Lock()
	defer c.reverseLock.Unlock()

	k := reverseKey(host, port)
	if c.reverses[k] <= 1 {
		delete(c.reverses, k)
	} else {
		c.reverses[k]--
	}
}

func (c *client) reverseAllowed(host string, port int) bool {
	c.reverseLock.Lock()
	defer c.reverseLock.Unlock()

	return c.reverses[reverseKey(host, port)] > 0
}

// tunnelPolicy returns the minitunnel policy for requests from c.
func (s *Server) tunnelPolicy(c *client) minitunnel.Policy {
	uuid := c.uuid

	p := minitunnel.Policy{
		MaxStreams: MaxTunnelStreams,
		MaxMessage: MaxMessageSize,
		OnRefuse: func(reason string) {
			warnLimited(uuid+":tunnel", "cc client %v: %v", uuid, reason)
		},
	}

	if c.policy == CCPolicyStrict {
		p.DenyForward = true
		p.AllowConnect = c.reverseAllowed
	}

	return p
}
