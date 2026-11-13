// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const testTimeout = 5 * time.Second

// recv returns the next value from ch, or fails the test after testTimeout.
func recv[T any](t testing.TB, ch <-chan T, what string) T {
	t.Helper()

	select {
	case v := <-ch:
		return v
	case <-time.After(testTimeout):
		t.Fatalf("timeout waiting for %s", what)
		var zero T
		return zero
	}
}

// logBuffer collects minilog output for one test.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

// lines returns the log lines that contain every one of substrs.
func (l *logBuffer) lines(substrs ...string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	var res []string
outer:
	for _, line := range strings.Split(l.b.String(), "\n") {
		for _, s := range substrs {
			if !strings.Contains(line, s) {
				continue outer
			}
		}
		res = append(res, line)
	}

	return res
}

func captureLog(t *testing.T) *logBuffer {
	t.Helper()

	lb := &logBuffer{}
	name := "test:" + t.Name()
	log.AddLogger(name, lb, log.DEBUG, false)
	t.Cleanup(func() { log.DelLogger(name) })

	return lb
}

// collector records the callbacks of one Proxy.
type collector struct {
	msgs   chan ClientMessage
	pastes chan string
}

func newCollector() *collector {
	return &collector{
		msgs:   make(chan ClientMessage, 4096),
		pastes: make(chan string, 64),
	}
}

func (c *collector) options(name string, extendedClipboard bool) ProxyOptions {
	return ProxyOptions{
		Name:              name,
		ExtendedClipboard: extendedClipboard,
		OnMessage:         func(m ClientMessage) { c.msgs <- m },
		OnPaste:           func(s string) { c.pastes <- s },
	}
}

func (c *collector) next(t *testing.T) ClientMessage {
	t.Helper()

	select {
	case m := <-c.msgs:
		return m
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for a routed message")
		return nil
	}
}

// drain returns the routed messages that are queued now.
func (c *collector) drain() []ClientMessage {
	var res []ClientMessage
	for {
		select {
		case m := <-c.msgs:
			res = append(res, m)
		default:
			return res
		}
	}
}

// proxyRun is one Proxy between a test client and a test server.
type proxyRun struct {
	client net.Conn // the test acts as the VNC client here
	server net.Conn // the test acts as QEMU here
	cancel context.CancelFunc
	done   chan error
}

// startProxy starts Proxy with server as the server side of the shim, and
// returns the test ends.
func startProxy(t *testing.T, opts ProxyOptions, shimServer, server net.Conn) *proxyRun {
	t.Helper()

	client, shimClient := net.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	r := &proxyRun{client: client, server: server, cancel: cancel, done: make(chan error, 1)}

	deadline := time.Now().Add(testTimeout)
	client.SetDeadline(deadline)
	server.SetDeadline(deadline)

	go func() { r.done <- Proxy(ctx, shimClient, shimServer, opts) }()

	t.Cleanup(func() {
		cancel()
		client.Close()
		server.Close()
		select {
		case <-r.done:
		case <-time.After(testTimeout):
			t.Error("Proxy did not return")
		}
	})

	return r
}

func startPipeProxy(t *testing.T, opts ProxyOptions) *proxyRun {
	t.Helper()

	shimServer, server := net.Pipe()
	return startProxy(t, opts, shimServer, server)
}

func (r *proxyRun) wait(t *testing.T) error {
	t.Helper()

	select {
	case err := <-r.done:
		r.done <- err
		return err
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for Proxy to return")
		return nil
	}
}

// send writes in on src and checks that dst reads want.
func send(t *testing.T, src, dst net.Conn, in, want []byte) {
	t.Helper()

	errc := make(chan error, 1)
	go func() {
		_, err := src.Write(in)
		errc <- err
	}()

	got := make([]byte, len(want))
	if _, err := io.ReadFull(dst, got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if err := recv(t, errc, "write"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got\n% x\nwant\n% x", got, want)
	}
}

func pass(t *testing.T, src, dst net.Conn, b []byte) {
	t.Helper()
	send(t, src, dst, b, b)
}

func rfbVersion(minor int) []byte {
	return []byte("RFB 003.00" + string(rune('0'+minor)) + "\n")
}

type step struct {
	fromServer bool
	b          []byte
}

func fromS(b ...byte) step { return step{true, b} }
func fromC(b ...byte) step { return step{false, b} }

func (r *proxyRun) run(t *testing.T, steps ...step) {
	t.Helper()

	for _, s := range steps {
		if s.fromServer {
			pass(t, r.server, r.client, s.b)
		} else {
			pass(t, r.client, r.server, s.b)
		}
	}
}

func handshake38() []step {
	return []step{
		fromS(rfbVersion(8)...),
		fromC(rfbVersion(8)...),
		fromS(1, securityNone),
		fromC(securityNone),
		fromS(0, 0, 0, 0),
		fromC(1),
	}
}

func encode(t testing.TB, msgs ...ClientMessage) []byte {
	t.Helper()

	var b bytes.Buffer
	for _, m := range msgs {
		if err := m.Write(&b); err != nil {
			t.Fatalf("write %T: %v", m, err)
		}
	}

	return b.Bytes()
}

// noVNCEncodings is the SetEncodings list of noVNC 1.7.0 for a 24-bit screen
// in a browser with no H.264 decoder (core/rfb.js _sendEncodings).
func noVNCEncodings(quality, compression int32) []int32 {
	return []int32{
		1, 7, -260, 16, 21, 5, 2, 6, 0,
		-32 + quality, -256 + compression,
		-223, -224, -258, -261, -308, -309, -312, -313, -307,
		ExtendedClipboardPseudoEncoding, -316, 0x574d5664, -239,
	}
}

func withoutEncoding(encs []int32, v int32) []int32 {
	var res []int32
	for _, e := range encs {
		if e != v {
			res = append(res, e)
		}
	}

	return res
}

// TestProxyHandshake is T3.
func TestProxyHandshake(t *testing.T) {
	cases := []struct {
		name   string
		steps  []step
		framed bool
	}{
		{"3.3", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(3)...), fromS(0, 0, 0, securityNone), fromC(1)}, true},
		{"3.4 as 3.3", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(4)...), fromS(0, 0, 0, securityNone), fromC(1)}, true},
		{"3.5 as 3.3", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(5)...), fromS(0, 0, 0, securityNone), fromC(1)}, true},
		{"3.7", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(7)...), fromS(1, securityNone), fromC(securityNone), fromC(1)}, true},
		{"3.8", handshake38(), true},
		{"3.8 two types", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(8)...), fromS(2, 2, securityNone), fromC(securityNone), fromS(0, 0, 0, 0), fromC(0)}, true},
		{"3.3 VNC auth", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(3)...), fromS(0, 0, 0, 2)}, false},
		{"3.7 VNC auth", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(7)...), fromS(1, 2), fromC(2)}, false},
		{"3.8 VNC auth", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(8)...), fromS(1, 2), fromC(2)}, false},
		{"3.8 failed result", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(8)...), fromS(1, securityNone), fromC(securityNone), fromS(0, 0, 0, 1)}, false},
		{"3.8 no types", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(8)...), fromS(0)}, false},
		{"3.6", []step{fromS(rfbVersion(8)...), fromC(rfbVersion(6)...)}, false},
		{"4.0", []step{fromS(rfbVersion(8)...), fromC([]byte("RFB 004.000\n")...)}, false},
		{"malformed", []step{fromS(rfbVersion(8)...), fromC([]byte("hello world\n")...)}, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLog(t)
			col := newCollector()
			name := "vm-" + t.Name()
			r := startPipeProxy(t, col.options(name, false))

			r.run(t, c.steps...)

			key := encode(t, &KeyEvent{DownFlag: 1, Key: 'a'})
			pass(t, r.client, r.server, key)

			if c.framed {
				if m, ok := col.next(t).(*KeyEvent); !ok || m.Key != 'a' {
					t.Fatalf("routed %#v, want KeyEvent a", m)
				}
			}

			r.client.Close()
			if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
				t.Fatalf("Proxy returned %v, want ErrClientClosed", err)
			}

			warnings := logs.lines("WARN", name)
			if c.framed {
				if len(warnings) != 0 {
					t.Fatalf("got warnings: %q", warnings)
				}
				if len(logs.lines("INFO", name, "connected, RFB")) != 1 {
					t.Fatalf("no connect log line")
				}
			} else {
				if len(warnings) != 1 {
					t.Fatalf("got %d warnings, want 1: %q", len(warnings), warnings)
				}
				if msgs := col.drain(); len(msgs) != 0 {
					t.Fatalf("passthrough routed %d messages", len(msgs))
				}
			}
		})
	}
}

// TestProxyEncodingPolicy is T4.
func TestProxyEncodingPolicy(t *testing.T) {
	for _, ext := range []bool{false, true} {
		name := "paste as keys"
		if ext {
			name = "bidirectional"
		}

		t.Run(name, func(t *testing.T) {
			logs := captureLog(t)
			col := newCollector()
			vm := "vm-" + t.Name()
			r := startPipeProxy(t, col.options(vm, ext))
			r.run(t, handshake38()...)

			for _, quality := range []int32{6, 3} {
				encs := noVNCEncodings(quality, 2)
				if len(encs) != 24 {
					t.Fatalf("noVNC list has %d encodings", len(encs))
				}

				want := encs
				if !ext {
					want = withoutEncoding(encs, ExtendedClipboardPseudoEncoding)
				}

				in := encode(t, &SetEncodings{Encodings: encs})
				out := encode(t, &SetEncodings{Encodings: want})
				send(t, r.client, r.server, in, out)

				if n := binary.BigEndian.Uint16(out[2:]); int(n) != len(want) || (!ext && n != 23) {
					t.Fatalf("n = %d", n)
				}

				m, ok := col.next(t).(*SetEncodings)
				if !ok || !reflect.DeepEqual(m.Encodings, want) {
					t.Fatalf("routed %#v, want the forwarded encodings", m)
				}
			}

			// A SetEncodings with no rule match goes through unchanged.
			plain := encode(t, &SetEncodings{Encodings: []int32{0, -223}})
			pass(t, r.client, r.server, plain)
			col.next(t)

			rewrites := logs.lines("DEBUG", vm, "removed ExtendedClipboard")
			if ext && len(rewrites) != 0 {
				t.Fatalf("bidirectional mode rewrote SetEncodings: %q", rewrites)
			}
			if !ext && len(rewrites) != 2 {
				t.Fatalf("got %d rewrite log lines, want 2", len(rewrites))
			}
		})
	}
}

func TestEncodingRules(t *testing.T) {
	var found bool
	for _, r := range EncodingRules {
		if r.Value == ExtendedClipboardPseudoEncoding {
			found = true
			if r.Keep(ProxyOptions{ExtendedClipboard: false}) || !r.Keep(ProxyOptions{ExtendedClipboard: true}) {
				t.Fatal("ExtendedClipboard rule does not follow the copy/paste mode")
			}
		}
	}

	if !found || len(EncodingRules) != 1 {
		t.Fatalf("got %d rules, want only ExtendedClipboard", len(EncodingRules))
	}
	if v := ExtendedClipboardPseudoEncoding; uint32(v) != 0xc0a1e5ce {
		t.Fatalf("ExtendedClipboardPseudoEncoding is %#x", uint32(v))
	}
}

// TestProxyUnknownPassthrough is T5.
func TestProxyUnknownPassthrough(t *testing.T) {
	key := encode(t, &KeyEvent{DownFlag: 1, Key: 'a'})

	for _, unknown := range [][]byte{{7}, {TypeQEMU, 9}, {TypeQEMU, QEMUAudio, 0, 9}} {
		t.Run(hex.EncodeToString(unknown), func(t *testing.T) {
			logs := captureLog(t)
			col := newCollector()
			vm := "vm-" + t.Name()
			r := startPipeProxy(t, col.options(vm, false))
			r.run(t, handshake38()...)

			pass(t, r.client, r.server, key)
			col.next(t)

			// After the unknown type, the bytes are not framed, even when
			// they look like messages.
			rest := append(append([]byte{}, unknown...), 0xde, 0xad)
			rest = append(rest, key...)
			rest = append(rest, cutTextHeader(math.MinInt32)...)
			pass(t, r.client, r.server, rest)

			r.client.Close()
			if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
				t.Fatalf("Proxy returned %v, want ErrClientClosed", err)
			}

			if msgs := col.drain(); len(msgs) != 0 {
				t.Fatalf("routed %d messages after the unknown type", len(msgs))
			}
			if w := logs.lines("WARN", vm); len(w) != 1 {
				t.Fatalf("got %d warnings, want 1: %q", len(w), w)
			}
		})
	}
}

func TestProxyPaste(t *testing.T) {
	cut := encode(t, &ClientCutText{Text: []byte("caf\xe9")})
	ext := encode(t, &ExtendedClientCutText{Data: []byte{0x08, 0, 0, 0x01}})

	t.Run("paste as keys", func(t *testing.T) {
		logs := captureLog(t)
		col := newCollector()
		vm := "vm-" + t.Name()
		r := startPipeProxy(t, col.options(vm, false))
		r.run(t, handshake38()...)

		pass(t, r.client, r.server, cut)
		select {
		case s := <-col.pastes:
			if s != "café" {
				t.Fatalf("pasted %q, want café", s)
			}
		case <-time.After(testTimeout):
			t.Fatal("no paste")
		}

		pass(t, r.client, r.server, ext)
		pass(t, r.client, r.server, encode(t, &ClientCutText{Text: []byte{}}))
		r.client.Close()
		r.wait(t)

		if len(col.pastes) != 0 {
			t.Fatal("extended or empty cut text was pasted")
		}
		if len(logs.lines("DEBUG", vm, "paste of 4 characters")) != 1 {
			t.Fatal("paste length is not logged")
		}
		if l := logs.lines("caf"); len(l) != 0 {
			t.Fatalf("pasted text is in the log: %q", l)
		}
	})

	t.Run("bidirectional", func(t *testing.T) {
		col := newCollector()
		r := startPipeProxy(t, col.options("vm-"+t.Name(), true))
		r.run(t, handshake38()...)

		pass(t, r.client, r.server, cut)
		r.client.Close()
		r.wait(t)

		if len(col.pastes) != 0 {
			t.Fatal("bidirectional mode pasted as keys")
		}
	})
}

func TestProxyCutTextLimit(t *testing.T) {
	logs := captureLog(t)
	col := newCollector()
	vm := "vm-" + t.Name()
	r := startPipeProxy(t, col.options(vm, false))
	r.run(t, handshake38()...)

	go r.client.Write(cutTextHeader(math.MinInt32))

	if err := r.wait(t); !errors.Is(err, ErrMessageLimit) {
		t.Fatalf("Proxy returned %v, want ErrMessageLimit", err)
	}

	if n, err := r.server.Read(make([]byte, 16)); err != io.EOF {
		t.Fatalf("server read %d bytes, %v; want io.EOF", n, err)
	}
	if w := logs.lines("WARN", vm); len(w) != 1 {
		t.Fatalf("got %d warnings, want 1", len(w))
	}
}

func TestProxyContextCancel(t *testing.T) {
	r := startPipeProxy(t, ProxyOptions{Name: "vm-" + t.Name()})
	r.run(t, handshake38()...)

	r.cancel()

	if err := r.wait(t); !errors.Is(err, context.Canceled) {
		t.Fatalf("Proxy returned %v, want context.Canceled", err)
	}
	if _, err := r.client.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("client read: %v, want io.EOF", err)
	}
	if _, err := r.server.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("server read: %v, want io.EOF", err)
	}
}

// A client that stops in the handshake ends Proxy at the handshake
// deadline.
func TestProxyHandshakeDeadline(t *testing.T) {
	opts := ProxyOptions{Name: "vm-" + t.Name(), handshakeTimeout: 100 * time.Millisecond}
	r := startPipeProxy(t, opts)
	r.run(t, fromS(rfbVersion(8)...))

	if err := r.wait(t); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("Proxy returned %v, want a deadline error", err)
	}
}

// Proxy clears the handshake deadline after the handshake.
func TestProxyClearsHandshakeDeadline(t *testing.T) {
	opts := ProxyOptions{Name: "vm-" + t.Name(), handshakeTimeout: 100 * time.Millisecond}
	r := startPipeProxy(t, opts)
	r.run(t, handshake38()...)

	time.Sleep(200 * time.Millisecond)

	r.run(t, fromC(encode(t, &KeyEvent{DownFlag: 1, Key: 'a'})...), fromS(TypeBell))
	select {
	case err := <-r.done:
		t.Fatalf("Proxy returned %v after the handshake", err)
	default:
	}
}

// unixPair returns the two ends of a unix socket connection: the shim end and
// the fake QEMU end.
func unixPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()

	// A short path, because unix socket paths have a small length limit.
	dir, err := os.MkdirTemp("", "vnc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	l, err := net.Listen("unix", filepath.Join(dir, "vnc"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- c
	}()

	shim, err := net.Dial("unix", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	select {
	case qemu, ok := <-accepted:
		if !ok {
			t.Fatal("accept failed")
		}
		return shim, qemu
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for accept")
		return nil, nil
	}
}

func startQEMUProxy(t *testing.T, opts ProxyOptions) *proxyRun {
	t.Helper()

	shim, qemu := unixPair(t)
	r := startProxy(t, opts, shim, qemu)
	r.run(t, handshake38()...)

	return r
}

// TestProxyIntegration is T6.
func TestProxyIntegration(t *testing.T) {
	t.Run("stream after policy", func(t *testing.T) {
		logs := captureLog(t)
		col := newCollector()
		vm := "vm-" + t.Name()
		r := startQEMUProxy(t, col.options(vm, false))

		// QEMU sends ServerInit unchanged through the shim.
		serverInit := []byte{0x04, 0, 0x03, 0, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0, 0, 0, 0, 4, 'Q', 'E', 'M', 'U'}
		pass(t, r.server, r.client, serverInit)

		encs := noVNCEncodings(6, 2)
		in := []ClientMessage{
			&SetPixelFormat{PixelFormat: PixelFormat{BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1, RedMax: 255, GreenMax: 255, BlueMax: 255, GreenShift: 8, BlueShift: 16}},
			&SetEncodings{Encodings: encs},
			&FramebufferUpdateRequest{Width: 1024, Height: 768},
			&SetDesktopSize{Width: 1280, Height: 800, Screens: []Screen{{Width: 1280, Height: 800}}},
			&KeyEvent{DownFlag: 1, Key: 'x'},
			&QEMUExtendedKeyEvent{DownFlag: 1, Keysym: 'a', Keycode: 0x1e},
			&QEMUExtendedKeyEvent{DownFlag: 0, Keysym: 'a', Keycode: 0x1e},
			&PointerEvent{ButtonMask: 1, XPosition: 5, YPosition: 6},
			&ClientCutText{Text: []byte("pwd")},
			&FramebufferUpdateRequest{Incremental: 1, Width: 1280, Height: 800},
		}
		want := append([]ClientMessage{}, in...)
		want[1] = &SetEncodings{Encodings: withoutEncoding(encs, ExtendedClipboardPseudoEncoding)}

		send(t, r.client, r.server, encode(t, in...), encode(t, want...))

		for i, w := range want {
			got := col.next(t)
			if !reflect.DeepEqual(got, w) {
				t.Fatalf("message %d: routed %#v, want %#v", i, got, w)
			}
		}

		select {
		case s := <-col.pastes:
			if s != "pwd" {
				t.Fatalf("pasted %q", s)
			}
		case <-time.After(testTimeout):
			t.Fatal("no paste")
		}

		r.client.Close()
		if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
			t.Fatalf("Proxy returned %v", err)
		}
		if w := logs.lines("WARN", vm); len(w) != 0 {
			t.Fatalf("got warnings: %q", w)
		}
	})

	t.Run("QEMU close closes client", func(t *testing.T) {
		r := startQEMUProxy(t, ProxyOptions{Name: "vm-" + t.Name()})

		r.server.Close()

		if _, err := r.client.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("client read: %v, want io.EOF", err)
		}
		if err := r.wait(t); !errors.Is(err, ErrServerClosed) {
			t.Fatalf("Proxy returned %v, want ErrServerClosed", err)
		}
	})

	t.Run("client close closes QEMU", func(t *testing.T) {
		r := startQEMUProxy(t, ProxyOptions{Name: "vm-" + t.Name()})

		r.client.Close()

		if _, err := r.server.Read(make([]byte, 1)); err != io.EOF {
			t.Fatalf("QEMU read: %v, want io.EOF", err)
		}
		if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
			t.Fatalf("Proxy returned %v, want ErrClientClosed", err)
		}
	})
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary accept error" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

// flakyListener fails Accept a number of times, and then returns conns.
type flakyListener struct {
	mu       sync.Mutex
	failures int
	conns    chan net.Conn
	closed   chan struct{}
	once     sync.Once
}

func (l *flakyListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if l.failures > 0 {
		l.failures--
		l.mu.Unlock()
		return nil, temporaryError{}
	}
	l.mu.Unlock()

	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *flakyListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *flakyListener) Addr() net.Addr { return &net.TCPAddr{} }

// TestServeRetriesAccept is the B9 part of T6.
func TestServeRetriesAccept(t *testing.T) {
	l := &flakyListener{failures: 3, conns: make(chan net.Conn, 1), closed: make(chan struct{})}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	l.conns <- a

	handled := make(chan net.Conn, 1)
	served := make(chan struct{})
	go func() {
		Serve(l, func(c net.Conn) { handled <- c })
		close(served)
	}()

	select {
	case c := <-handled:
		if c != a {
			t.Fatal("handled the wrong connection")
		}
	case <-time.After(testTimeout):
		t.Fatal("Serve did not retry after a temporary Accept error")
	}

	l.Close()

	select {
	case <-served:
	case <-time.After(testTimeout):
		t.Fatal("Serve did not return after Close")
	}
}

// readHexFixture reads a hex file and ignores whitespace and comments.
func readHexFixture(t testing.TB, path string) []byte {
	t.Helper()

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var digits strings.Builder
	s := bufio.NewScanner(f)
	for s.Scan() {
		line, _, _ := strings.Cut(s.Text(), "#")
		digits.WriteString(strings.Join(strings.Fields(line), ""))
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}

	b, err := hex.DecodeString(digits.String())
	if err != nil {
		t.Fatalf("%v: %v", path, err)
	}

	return b
}

// noVNCSession returns the messages of the T26 fixture after the 14-byte
// client handshake, and each message by itself.
func noVNCSession(tb testing.TB) ([]byte, [][]byte) {
	tb.Helper()

	capture := readHexFixture(tb, filepath.Join("testdata", "novnc-1.7.0-session.hex"))
	if len(capture) < 14 {
		tb.Fatal("fixture is too short")
	}
	stream := capture[14:]

	var msgs [][]byte
	mr := newMessageReader(bytes.NewReader(stream))
	for {
		raw, _, err := mr.next()
		if err == io.EOF {
			return stream, msgs
		} else if err != nil {
			tb.Fatalf("fixture: %v", err)
		}
		msgs = append(msgs, append([]byte{}, raw...))
	}
}

// TestProxyNoVNCSession is T26. The fixture is a synthetic noVNC 1.7.0
// stream. See testdata/README.md.
func TestProxyNoVNCSession(t *testing.T) {
	capture := readHexFixture(t, filepath.Join("testdata", "novnc-1.7.0-session.hex"))
	wantKB, err := os.ReadFile(filepath.Join("testdata", "novnc-1.7.0-session.kb"))
	if err != nil {
		t.Fatal(err)
	}

	// The client handshake for RFB 3.8 with security None is 14 bytes:
	// ProtocolVersion, the security type, and ClientInit.
	if len(capture) < 14 || string(capture[:12]) != string(rfbVersion(8)) {
		t.Fatal("fixture does not start with an RFB 3.8 handshake")
	}
	messages := capture[14:]

	// The bytes at QEMU are the capture after the policy.
	var want bytes.Buffer
	var setEncodings int
	mr := newMessageReader(bytes.NewReader(messages))
	for {
		raw, msg, err := mr.next()
		if err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("fixture: %v", err)
		}

		if m, ok := msg.(*SetEncodings); ok {
			setEncodings++
			want.Write(encode(t, &SetEncodings{Encodings: withoutEncoding(m.Encodings, ExtendedClipboardPseudoEncoding)}))
			continue
		}
		want.Write(raw)
	}
	if setEncodings == 0 || want.Len() != len(messages)-4*setEncodings {
		t.Fatalf("policy changed %d bytes in %d SetEncodings", len(messages)-want.Len(), setEncodings)
	}

	rec, kbPath := newTestKBRecorder(t, "vm")
	logs := captureLog(t)
	pastes := make(chan string, 8)
	opts := ProxyOptions{
		Name:      "vm-" + t.Name(),
		OnMessage: func(m ClientMessage) { rec.Route("vm", m) },
		OnPaste:   func(s string) { pastes <- s },
	}

	shim, qemu := unixPair(t)
	r := startProxy(t, opts, shim, qemu)
	r.run(t,
		fromS(rfbVersion(8)...),
		fromC(capture[:12]...),
		fromS(1, securityNone),
		fromC(capture[12]),
		fromS(0, 0, 0, 0),
		fromC(capture[13]),
	)

	send(t, r.client, r.server, messages, want.Bytes())

	r.client.Close()
	if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("Proxy returned %v", err)
	}
	if w := logs.lines("WARN"); len(w) != 0 {
		t.Fatalf("got warnings: %q", w)
	}
	if s := recv(t, pastes, "paste"); s != "pwd" {
		t.Fatalf("pasted %q", s)
	}

	if err := rec.StopKB("vm"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(kbPath)
	if err != nil {
		t.Fatal(err)
	}

	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(string(got)), "\n") {
		_, event, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("bad kb line %q", line)
		}
		lines = append(lines, event)
	}

	if g, w := strings.Join(lines, "\n"), strings.TrimSpace(string(wantKB)); g != w {
		t.Fatalf("kb recording:\n%s\nwant:\n%s", g, w)
	}
}

// newTestKBRecorder returns a Recorder with a kb recording for id that
// writes to a file and has no VNC connection.
func newTestKBRecorder(t *testing.T, id string) (*Recorder, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kb")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	rec := NewRecorder()
	rec.kb[id] = &kbRecorder{
		recorder: &recorder{file: f, start: time.Now()},
		last:     time.Now(),
	}

	return rec, path
}

// fuzzGen turns fuzz input into a sequence of valid client messages.
type fuzzGen struct{ b []byte }

func (g *fuzzGen) u8() uint8 {
	if len(g.b) == 0 {
		return 0
	}
	v := g.b[0]
	g.b = g.b[1:]
	return v
}

func (g *fuzzGen) u16() uint16 { return uint16(g.u8())<<8 | uint16(g.u8()) }
func (g *fuzzGen) u32() uint32 { return uint32(g.u16())<<16 | uint32(g.u16()) }

func (g *fuzzGen) bytes(n int) []byte {
	res := make([]byte, n)
	for i := range res {
		res[i] = g.u8()
	}
	return res
}

// messages returns the messages and, if the input asks for one, an unknown
// tail that the shim must pass through.
func (g *fuzzGen) messages() ([]ClientMessage, []byte) {
	known := noVNCEncodings(6, 2)

	var msgs []ClientMessage
	for len(g.b) > 0 && len(msgs) < 64 {
		switch g.u8() % 16 {
		case 0:
			msgs = append(msgs, &SetPixelFormat{PixelFormat: PixelFormat{BitsPerPixel: g.u8(), Depth: g.u8(), RedMax: g.u16()}})
		case 1:
			encs := make([]int32, g.u8()%32)
			for i := range encs {
				if v := g.u8(); v < 200 {
					encs[i] = known[int(v)%len(known)]
				} else {
					encs[i] = int32(g.u32())
				}
			}
			msgs = append(msgs, &SetEncodings{Encodings: encs})
		case 2:
			msgs = append(msgs, &FramebufferUpdateRequest{Incremental: g.u8(), X: g.u16(), Width: g.u16()})
		case 3:
			msgs = append(msgs, &KeyEvent{DownFlag: g.u8(), Key: g.u32()})
		case 4:
			msgs = append(msgs, &PointerEvent{ButtonMask: g.u8() & 0x7f, XPosition: g.u16(), YPosition: g.u16()})
		case 5:
			msgs = append(msgs, &ExtendedPointerEvent{ButtonMask: g.u8() | 0x80, XPosition: g.u16(), HighButtons: g.u8()})
		case 6:
			msgs = append(msgs, &ClientCutText{Text: g.bytes(int(g.u8()))})
		case 7:
			if n := int(g.u8()); n > 0 {
				msgs = append(msgs, &ExtendedClientCutText{Data: g.bytes(n)})
			}
		case 8:
			msgs = append(msgs, &EnableContinuousUpdates{Enable: g.u8(), Width: g.u16()})
		case 9:
			msgs = append(msgs, &ClientFence{Flags: g.u32(), Payload: g.bytes(int(g.u8() % (maxFencePayload + 1)))})
		case 10:
			msgs = append(msgs, &XVPMessage{Version: g.u8(), Op: g.u8()})
		case 11:
			screens := make([]Screen, g.u8()%4)
			for i := range screens {
				screens[i] = Screen{ID: g.u32(), Width: g.u16(), Flags: g.u32()}
			}
			msgs = append(msgs, &SetDesktopSize{Width: g.u16(), Height: g.u16(), Screens: screens})
		case 12:
			msgs = append(msgs, &QEMUExtendedKeyEvent{DownFlag: g.u16(), Keysym: g.u32(), Keycode: g.u32()})
		case 13:
			op := uint16(g.u8() % 3)
			m := &QEMUAudioMessage{Operation: op}
			if op == QEMUAudioSetFormat {
				m.Format, m.Channels, m.Frequency = g.u8(), g.u8(), g.u32()
			}
			msgs = append(msgs, m)
		case 14:
			// An unknown type, then the rest of the input unchanged.
			tail := append([]byte{[]byte{1, 7, 100, 200}[g.u8()%4]}, g.b...)
			g.b = nil
			return msgs, tail
		case 15:
			tail := append([]byte{TypeQEMU, 2 + g.u8()%250}, g.b...)
			g.b = nil
			return msgs, tail
		}
	}

	return msgs, nil
}

// generatedStream turns fuzz input into a client stream of valid messages
// and the bytes that the server must get for it.
func generatedStream(t testing.TB, in []byte, extendedClipboard bool) ([]byte, []byte) {
	g := &fuzzGen{b: in}
	msgs, tail := g.messages()

	var input, want bytes.Buffer
	input.Write(encode(t, msgs...))
	for _, m := range msgs {
		if e, ok := m.(*SetEncodings); ok && !extendedClipboard {
			m = &SetEncodings{Encodings: withoutEncoding(e.Encodings, ExtendedClipboardPseudoEncoding)}
		}
		want.Write(encode(t, m))
	}
	input.Write(tail)
	want.Write(tail)

	return input.Bytes(), want.Bytes()
}

// policyModel returns the bytes that the server must get for a raw client
// stream. It returns false when the shim must stop at a framing error.
func policyModel(t testing.TB, stream []byte, extendedClipboard bool) ([]byte, bool) {
	br := bytes.NewReader(stream)
	mr := newMessageReader(br)

	var want []byte
	for {
		raw, msg, err := mr.next()

		var unknown *UnknownMessageError
		switch {
		case err == io.EOF:
			return want, true
		case errors.As(err, &unknown):
			want = append(want, unknown.Raw...)
			return append(want, stream[len(stream)-br.Len():]...), true
		case err != nil:
			return want, false
		}

		if m, ok := msg.(*SetEncodings); ok && !extendedClipboard {
			encs := withoutEncoding(m.Encodings, ExtendedClipboardPseudoEncoding)
			if len(encs) != len(m.Encodings) {
				raw = encode(t, &SetEncodings{Encodings: encs})
			}
		}
		want = append(want, raw...)
	}
}

// FuzzProxyStream is T23: for random message sequences, the bytes at the
// server equal the input after the policy. When rawStream is true, the input
// is the client stream itself, and the shim must stop at a framing error.
func FuzzProxyStream(f *testing.F) {
	f.Add([]byte{}, false, false)
	f.Add([]byte{1, 24, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 3, 1}, false, false)
	f.Add([]byte{1, 3, 20, 20, 20, 6, 3, 'a', 'b', 'c', 12, 0, 1, 0, 0, 0, 'a', 0, 0, 0, 0x1e}, true, false)
	f.Add([]byte{11, 2, 0, 0, 0, 1, 4, 0, 0, 0, 0, 0, 0, 14, 0, 4, 1, 0, 0}, false, false)
	f.Add([]byte{3, 1, 0, 0, 0, 'a', 15, 0, 1, 2, 3}, true, false)

	session, _ := noVNCSession(f)
	f.Add(session, false, true)
	f.Add(session, true, true)

	f.Fuzz(func(t *testing.T, in []byte, extendedClipboard, rawStream bool) {
		var input, want []byte
		clean := true
		if rawStream {
			input = in
			want, clean = policyModel(t, in, extendedClipboard)
		} else {
			input, want = generatedStream(t, in, extendedClipboard)
		}

		opts := ProxyOptions{ExtendedClipboard: extendedClipboard}

		client, shimClient := net.Pipe()
		shimServer, server := net.Pipe()
		deadline := time.Now().Add(testTimeout)
		client.SetDeadline(deadline)
		server.SetDeadline(deadline)

		done := make(chan error, 1)
		go func() { done <- Proxy(context.Background(), shimClient, shimServer, opts) }()

		clientErr := make(chan error, 1)
		go func() {
			defer client.Close()

			buf := make([]byte, 12)
			for _, s := range []struct{ read, write []byte }{
				{rfbVersion(8), rfbVersion(8)},
				{[]byte{1, securityNone}, []byte{securityNone}},
				{[]byte{0, 0, 0, 0}, []byte{1}},
			} {
				if _, err := io.ReadFull(client, buf[:len(s.read)]); err != nil {
					clientErr <- err
					return
				}
				if _, err := client.Write(s.write); err != nil {
					clientErr <- err
					return
				}
			}

			_, err := client.Write(input)
			clientErr <- err
		}()

		var got bytes.Buffer
		func() {
			defer server.Close()

			buf := make([]byte, 12)
			for _, s := range []struct {
				write []byte
				read  int
			}{
				{rfbVersion(8), 12},
				{[]byte{1, securityNone}, 1},
				{[]byte{0, 0, 0, 0}, 1},
			} {
				if _, err := server.Write(s.write); err != nil {
					t.Fatalf("server write: %v", err)
				}
				if _, err := io.ReadFull(server, buf[:s.read]); err != nil {
					t.Fatalf("server read: %v", err)
				}
			}

			if _, err := io.Copy(&got, server); err != nil {
				t.Fatalf("server read: %v", err)
			}
		}()

		// After a framing error, the shim closes the client while it writes.
		if err := recv(t, clientErr, "client"); err != nil && clean {
			t.Fatalf("client: %v", err)
		}

		err := recv(t, done, "Proxy")
		if clean && !errors.Is(err, ErrClientClosed) {
			t.Fatalf("Proxy returned %v, want ErrClientClosed", err)
		}
		if !clean && (err == nil || errors.Is(err, ErrClientClosed)) {
			t.Fatalf("Proxy returned %v, want a framing error", err)
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Fatalf("server got %d bytes, want %d", got.Len(), len(want))
		}
	})
}
