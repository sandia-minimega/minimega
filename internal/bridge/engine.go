// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	"github.com/sandia-minimega/minimega/v2/internal/version"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcap"
)

const (
	// DefaultSnapLen is the snaplen of a capture that does not set one.
	DefaultSnapLen = 1600

	// MinSnapLen and MaxSnapLen are the limits of a capture snaplen.
	MinSnapLen = 14
	MaxSnapLen = 262144

	captureTimeout    = 100 * time.Millisecond
	captureBufferSize = 32 << 20
	flushInterval     = 100 * time.Millisecond
	statsInterval     = time.Second
	drainLimit        = 2 * time.Second
)

// Capture directions for CaptureConfig.Direction.
const (
	DirectionBoth = "both"
	DirectionIn   = "in"
	DirectionOut  = "out"
)

// CaptureState is the state of a capture.
type CaptureState string

const (
	CaptureRunning CaptureState = "running"
	CaptureFailed  CaptureState = "failed"
	CaptureFull    CaptureState = "full"
	CaptureStopped CaptureState = "stopped"
)

// CaptureConfig holds the settings of a pcap capture.
type CaptureConfig struct {
	// Filter is a BPF string to apply to all packets. See `man pcap-filter`
	// for the syntax and semantics.
	Filter string

	// SnapLen controls how many bytes to capture for each packet. When zero,
	// we use DefaultSnapLen.
	SnapLen uint32

	// MaxSize stops the capture as full when the file reaches this many
	// bytes. Zero means no limit.
	MaxSize int64

	// Direction limits the capture to frames that the interface receives
	// (in) or sends (out). Empty means both.
	Direction string

	// Format is the file format, pcap (the default) or pcapng.
	Format string

	// Description, Comment and Names are written to a pcapng file: the
	// interface description, the section comment and the known addresses.
	Description string
	Comment     string
	Names       []pcapfile.Name
}

// format returns the file format to use for c.
func (c CaptureConfig) format() string {
	if c.Format == "" {
		return pcapfile.FormatPcap
	}

	return c.Format
}

// snapLen returns the snaplen to use for c.
func (c CaptureConfig) snapLen() uint32 {
	if c.SnapLen == 0 {
		return DefaultSnapLen
	}

	return c.SnapLen
}

// Validate checks the values of c.
func (c CaptureConfig) Validate() error {
	if c.SnapLen != 0 {
		if err := ValidateSnapLen(c.SnapLen); err != nil {
			return err
		}
	}

	if err := ValidateFilter(c.Filter, c.snapLen()); err != nil {
		return err
	}

	switch c.Direction {
	case "", DirectionBoth, DirectionIn, DirectionOut:
	default:
		return fmt.Errorf("invalid direction %q: use in, out or both", c.Direction)
	}

	if c.MaxSize < 0 {
		return errors.New("maxsize must not be negative")
	}

	return ValidateFormat(c.Format)
}

// ValidateFormat checks that f is a capture file format. Empty means pcap.
func ValidateFormat(f string) error {
	switch f {
	case "", pcapfile.FormatPcap, pcapfile.FormatPcapng:
		return nil
	}

	return fmt.Errorf("invalid format %q: use pcap or pcapng", f)
}

// NewCaptureWriter writes the header of a capture file in format to w.
func NewCaptureWriter(w io.Writer, format string, snaplen uint32, comment string) (pcapfile.Writer, error) {
	if format != pcapfile.FormatPcapng {
		return pcapfile.NewClassic(w, snaplen)
	}

	return pcapfile.NewNg(w, pcapfile.Section{
		UserAppl: "minimega " + version.Version,
		OS:       runtime.GOOS,
		Comment:  comment,
	})
}

// ValidateSnapLen checks that v is a valid snaplen.
func ValidateSnapLen(v uint32) error {
	if v < MinSnapLen || v > MaxSnapLen {
		return fmt.Errorf("snaplen must be from %v to %v", MinSnapLen, MaxSnapLen)
	}

	return nil
}

// ValidateFilter compiles the BPF filter f to check its syntax. An empty
// filter is valid.
func ValidateFilter(f string, snaplen uint32) error {
	if f == "" {
		return nil
	}

	if _, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, int(snaplen), f); err != nil {
		return fmt.Errorf("invalid filter %q: %v", f, err)
	}

	return nil
}

// ParseSize parses a size in bytes with an optional K, M or G suffix (powers
// of 1024).
func ParseSize(size string) (int64, error) {
	s, mult := size, int64(1)

	switch strings.ToUpper(s[len(s)-min(len(s), 1):]) {
	case "K":
		mult = 1 << 10
	case "M":
		mult = 1 << 20
	case "G":
		mult = 1 << 30
	}

	if mult > 1 {
		s = s[:len(s)-1]
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 || v > (1<<62)/mult {
		return 0, fmt.Errorf("invalid size %q: use a number of bytes with an optional K, M or G suffix", size)
	}

	return v * mult, nil
}

// CaptureInfo describes one capture.
type CaptureInfo struct {
	ID     int
	Tap    string
	Path   string
	Format string
	Config CaptureConfig
	Start  time.Time
	State  CaptureState
	Error  string

	Packets     uint64 // frames written to the file
	Bytes       uint64 // original length of the frames written
	KernelDrops uint64 // frames that the kernel dropped
	Size        int64  // bytes in the file
}

// packetSource is the source of a capture: a pcap handle, or a fake in tests.
type packetSource interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	Stats() (*pcap.Stats, error)
	Close()
}

// errSourceTimeout is the timeout error of a packetSource.
var errSourceTimeout = pcap.NextErrorTimeoutExpired

// openSource opens a pcap handle on device with the settings of c.
func openSource(device string, c CaptureConfig) (packetSource, error) {
	inactive, err := pcap.NewInactiveHandle(device)
	if err != nil {
		return nil, err
	}
	defer inactive.CleanUp()

	if err := inactive.SetSnapLen(int(c.snapLen())); err != nil {
		return nil, err
	}
	if err := inactive.SetPromisc(true); err != nil {
		return nil, err
	}
	if err := inactive.SetTimeout(captureTimeout); err != nil {
		return nil, err
	}
	if err := inactive.SetBufferSize(captureBufferSize); err != nil {
		return nil, err
	}

	handle, err := inactive.Activate()
	if err != nil {
		return nil, err
	}

	switch c.Direction {
	case DirectionIn:
		err = handle.SetDirection(pcap.DirectionIn)
	case DirectionOut:
		err = handle.SetDirection(pcap.DirectionOut)
	}

	if err == nil && c.Filter != "" {
		err = handle.SetBPFFilter(c.Filter)
	}

	if err != nil {
		handle.Close()
		return nil, err
	}

	return handle, nil
}

// fileSink is a capture file.
type fileSink struct {
	f       *os.File
	w       pcapfile.Writer
	path    string
	release func()
}

// capturePaths holds the regular files that running captures write, guarded
// by bridgeLock.
var capturePaths = map[string]bool{}

// ClaimCapturePath reserves path for a capture. It fails when another running
// capture writes path. Files that are not regular files, such as /dev/null,
// are never reserved. The returned function releases path.
func ClaimCapturePath(path string) (func(), error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	return claimCapturePath(path)
}

func claimCapturePath(path string) (func(), error) {
	path = filepath.Clean(path)

	if fi, err := os.Stat(path); err == nil && !fi.Mode().IsRegular() {
		return func() {}, nil
	}

	if capturePaths[path] {
		return nil, fmt.Errorf("file %v is in use by another capture", path)
	}

	capturePaths[path] = true

	return func() {
		bridgeLock.Lock()
		defer bridgeLock.Unlock()

		delete(capturePaths, path)
	}, nil
}

// createFileSink claims path, creates its directory and the file, and writes
// the file header. Assumes bridgeLock is held.
func createFileSink(path, iface string, c CaptureConfig) (*fileSink, error) {
	release, err := claimCapturePath(path)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		release()
		return nil, err
	}

	f, err := os.Create(path)
	if err != nil {
		release()
		return nil, err
	}

	w, err := NewCaptureWriter(f, c.format(), c.snapLen(), c.Comment)
	if err == nil {
		_, err = w.AddInterface(pcapfile.Interface{
			Name:        iface,
			Description: c.Description,
			Filter:      c.Filter,
			SnapLen:     c.snapLen(),
		})
	}
	if err == nil && len(c.Names) > 0 {
		err = w.WriteNames(c.Names)
	}
	if err == nil {
		err = w.Flush()
	}
	if err != nil {
		f.Close()
		release()
		return nil, err
	}

	return &fileSink{f: f, w: w, path: path, release: release}, nil
}

func (s *fileSink) close() error {
	err := s.w.Close()
	if err2 := s.f.Close(); err == nil {
		err = err2
	}

	// release in a goroutine: the caller can hold bridgeLock
	go s.release()

	return err
}

// engine reads frames from a packetSource and writes them to a fileSink in
// its own goroutine. It never holds bridgeLock while it waits for frames.
type engine struct {
	src     packetSource
	sink    *fileSink
	maxSize int64
	recOpts pcapfile.RecordOptions // options for every record
	started time.Time

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once

	// onEnd runs in a new goroutine after the engine ended on its own, for
	// example after an error or when the file is full.
	onEnd func()

	mu    sync.Mutex
	state CaptureState
	err   string

	packets, bytes, kdrops atomic.Uint64
	size                   atomic.Int64
}

func newEngine(src packetSource, sink *fileSink, maxSize int64) *engine {
	e := &engine{
		src:     src,
		sink:    sink,
		maxSize: maxSize,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		state:   CaptureRunning,
		started: time.Now(),
	}
	e.size.Store(sink.w.Size())

	return e
}

func (e *engine) start() {
	go e.run()
}

// end sets the final state of the engine, unless it already has one, and
// reports whether it set it.
func (e *engine) end(state CaptureState, err string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state != CaptureRunning {
		return false
	}

	e.state = state
	e.err = err

	return true
}

// fail marks the engine failed with err and tells it to stop, without a wait.
func (e *engine) fail(err string) {
	e.end(CaptureFailed, err)
	e.signal()
}

func (e *engine) signal() {
	e.stopOnce.Do(func() { close(e.stop) })
}

// Stop tells the engine to stop and waits until the file is closed.
func (e *engine) Stop() {
	e.signal()
	<-e.done
}

func (e *engine) status() (CaptureState, string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.state, e.err
}

func (e *engine) stopped() bool {
	select {
	case <-e.stop:
		return true
	default:
		return false
	}
}

func (e *engine) run() {
	var (
		buf       []byte
		lastFlush = time.Now()
		lastStats = time.Now()
		drainEnd  time.Time
		selfEnded bool
	)

	fail := func(err error) {
		if e.end(CaptureFailed, err.Error()) {
			selfEnded = true
		}
	}

	for {
		if e.stopped() && drainEnd.IsZero() {
			// read what is already queued, but not for long
			drainEnd = time.Now().Add(drainLimit)
		}
		if !drainEnd.IsZero() && time.Now().After(drainEnd) {
			break
		}

		data, ci, err := e.src.ReadPacketData()
		if err == errSourceTimeout {
			if !drainEnd.IsZero() {
				break
			}
		} else if err != nil {
			if !e.stopped() {
				fail(err)
			}
			break
		} else {
			size := e.sink.w.Size() + int64(pcapfile.RecordHeaderLen+len(data))
			if e.maxSize > 0 && size > e.maxSize {
				if e.end(CaptureFull, fmt.Sprintf("file reached maxsize %v", e.maxSize)) {
					selfEnded = true
				}
				break
			}

			buf = pcapfile.AppendRecord(buf[:0], ci.Timestamp, ci.Length, data)
			if err := e.sink.w.WriteRecord(0, buf, e.recOpts); err != nil {
				fail(err)
				break
			}

			e.packets.Add(1)
			e.bytes.Add(uint64(ci.Length))
			e.size.Store(e.sink.w.Size())
		}

		if time.Since(lastFlush) >= flushInterval {
			lastFlush = time.Now()
			if err := e.sink.w.Flush(); err != nil {
				fail(err)
				break
			}
		}

		if time.Since(lastStats) >= statsInterval {
			lastStats = time.Now()
			e.readStats()
		}
	}

	e.readStats()
	e.src.Close()

	stats := pcapfile.Stats{
		Start:     e.started,
		End:       time.Now(),
		Received:  e.packets.Load() + e.kdrops.Load(),
		IfDrop:    e.kdrops.Load(),
		Delivered: e.packets.Load(),
	}
	if err := e.sink.w.WriteStats(0, stats, ""); err != nil {
		fail(err)
	}

	if err := e.sink.close(); err != nil {
		fail(err)
	}
	e.size.Store(e.sink.w.Size())

	e.end(CaptureStopped, "")

	close(e.done)

	if selfEnded && e.onEnd != nil {
		go e.onEnd()
	}
}

func (e *engine) readStats() {
	if s, err := e.src.Stats(); err == nil {
		e.kdrops.Store(uint64(s.PacketsDropped) + uint64(s.PacketsIfDropped))
	}
}

// capture is one pcap capture on a bridge.
type capture struct {
	id     int
	tap    string
	mirror bool // tap is a host tap with a mirror that this capture made
	config CaptureConfig
	start  time.Time

	engine   *engine
	released bool
}

func (c *capture) info() CaptureInfo {
	state, err := c.engine.status()

	return CaptureInfo{
		ID:          c.id,
		Tap:         c.tap,
		Path:        c.engine.sink.path,
		Format:      c.engine.sink.w.Format(),
		Config:      c.config,
		Start:       c.start,
		State:       state,
		Error:       err,
		Packets:     c.engine.packets.Load(),
		Bytes:       c.engine.bytes.Load(),
		KernelDrops: c.engine.kdrops.Load(),
		Size:        c.engine.size.Load(),
	}
}
