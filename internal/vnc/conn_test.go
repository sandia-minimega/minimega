// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image/color"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"
)

// fakeRFB is a fake RFB 3.3 server for tests of the minimega client.
type fakeRFB struct {
	// security is the security message. Empty means the None type.
	security []byte

	width, height uint16
	pf            PixelFormat

	// handle, if set, is called for each client message after ServerInit.
	// An error closes the connection.
	handle func(c net.Conn, m ClientMessage) error

	msgs  chan ClientMessage // each client message after ServerInit
	ended chan struct{}      // gets one value when a connection ends

	active atomic.Int32 // count of open connections

	mu    sync.Mutex
	conns []net.Conn
}

// closeConns closes each connection, as QEMU does when a VM is killed.
func (f *fakeRFB) closeConns() {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, c := range f.conns {
		c.Close()
	}
}

// listen starts the server on a loopback port and returns its address.
func (f *fakeRFB) listen(t *testing.T) string {
	t.Helper()

	f.msgs = make(chan ClientMessage, 4096)
	f.ended = make(chan struct{}, 64)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		ln.Close()

		f.mu.Lock()
		defer f.mu.Unlock()
		for _, c := range f.conns {
			c.Close()
		}
	})

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}

			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			f.active.Add(1)

			go func() {
				defer func() { f.ended <- struct{}{} }()
				defer f.active.Add(-1)
				defer c.Close()

				_ = f.serve(c)
			}()
		}
	}()

	return ln.Addr().String()
}

func (f *fakeRFB) serve(c net.Conn) error {
	if _, err := io.WriteString(c, "RFB 003.003\n"); err != nil {
		return err
	}
	if _, err := io.ReadFull(c, make([]byte, 12)); err != nil {
		return err
	}

	security := f.security
	if len(security) == 0 {
		security = []byte{0, 0, 0, 1}
	}
	if _, err := c.Write(security); err != nil {
		return err
	}
	if !bytes.Equal(security, []byte{0, 0, 0, 1}) {
		return nil
	}

	if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
		return err
	}

	name := "fake"
	init := struct {
		Width, Height uint16
		PixelFormat   PixelFormat
		NameLength    uint32
	}{f.width, f.height, f.pf, uint32(len(name))}
	if err := binary.Write(c, binary.BigEndian, &init); err != nil {
		return err
	}
	if _, err := io.WriteString(c, name); err != nil {
		return err
	}

	for {
		m, err := ReadClientMessage(c)
		if err != nil {
			return err
		}

		select {
		case f.msgs <- m:
		default:
		}

		if f.handle != nil {
			if err := f.handle(c, m); err != nil {
				return err
			}
		}
	}
}

// oneByteConn is a net.Conn that returns at most one byte from each Read.
type oneByteConn struct {
	net.Conn
	r io.Reader
}

func (c oneByteConn) Read(p []byte) (int, error) { return c.r.Read(p) }

// rect returns the header of a FramebufferUpdate rectangle.
func rect(x, y, w, h uint16, enc int32) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, []uint16{x, y, w, h})
	binary.Write(&b, binary.BigEndian, enc)
	return b.Bytes()
}

// rawRect returns a Raw rectangle with every pixel set to px, in
// clientFormat.
func rawRect(x, y, w, h uint16, px uint32) []byte {
	b := rect(x, y, w, h, RawEncoding)
	for i := 0; i < int(w)*int(h); i++ {
		b = binary.LittleEndian.AppendUint32(b, px)
	}
	return b
}

// fbUpdate returns a FramebufferUpdate message with the given rectangles.
func fbUpdate(rects ...[]byte) []byte {
	b := []byte{TypeFramebufferUpdate, 0}
	b = binary.BigEndian.AppendUint16(b, uint16(len(rects)))
	for _, r := range rects {
		b = append(b, r...)
	}
	return b
}

// The client against a fake server.
func TestConnSecurityType(t *testing.T) {
	tests := []struct {
		name     string
		security []byte
		want     string
	}{
		{"vnc auth", []byte{0, 0, 0, 2}, "security type 2"},
		{"failed", append([]byte{0, 0, 0, 0, 0, 0, 0, 8}, "too many"...), "too many"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeRFB{security: tt.security, width: 4, height: 4, pf: clientFormat}
			addr := f.listen(t)

			c, err := Dial(addr)
			if err == nil {
				c.Close()
				t.Fatal("Dial succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Dial error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

// Short reads, the sent pixel format with a 16 bpp server, and the
// ServerCutText padding.
func TestConnShortReads16bpp(t *testing.T) {
	pf16 := PixelFormat{
		BitsPerPixel: 16, Depth: 16, TrueColorFlag: 1,
		RedMax: 31, GreenMax: 63, BlueMax: 31,
		RedShift: 11, GreenShift: 5, BlueShift: 0,
	}

	reply := fbUpdate(rawRect(1, 2, 2, 1, 0x00112233))
	reply = append(reply, TypeServerCutText, 0, 0, 0, 0, 0, 0, 3, 'a', 'b', 'c')
	reply = append(reply, TypeBell)

	f := &fakeRFB{width: 8, height: 8, pf: pf16}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			_, err := c.Write(reply)
			return err
		}
		return nil
	}
	addr := f.listen(t)

	raw, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })

	c, err := newConn(oneByteConn{raw, iotest.OneByteReader(raw)}, time.Now().Add(testTimeout))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}

	if c.s.Width != 8 || c.s.Height != 8 || c.s.BitsPerPixel != 16 || string(c.s.Name) != "fake" {
		t.Errorf("server init = %dx%d %d bpp %q, want 8x8 16 bpp \"fake\"", c.s.Width, c.s.Height, c.s.BitsPerPixel, c.s.Name)
	}

	m := recv(t, f.msgs, "SetPixelFormat")
	if spf, ok := m.(*SetPixelFormat); !ok || spf.PixelFormat != clientFormat {
		t.Fatalf("first message = %#v, want SetPixelFormat %#v", m, clientFormat)
	}

	msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read update: %v", err)
	}
	u, ok := msg.(*FramebufferUpdate)
	if !ok || len(u.Rectangles) != 1 || u.Rectangles[0].RGBA == nil {
		t.Fatalf("message = %#v, want one Raw rectangle", msg)
	}
	want := color.RGBA{0x11, 0x22, 0x33, 0xff}
	for _, p := range [][2]int{{1, 2}, {2, 2}} {
		if got := u.Rectangles[0].RGBAAt(p[0], p[1]); got != want {
			t.Errorf("pixel %v = %v, want %v", p, got, want)
		}
	}

	msg, err = c.ReadMessage()
	if err != nil {
		t.Fatalf("read cut text: %v", err)
	}
	if ct, ok := msg.(*ServerCutText); !ok || string(ct.Text) != "abc" {
		t.Fatalf("message = %#v, want ServerCutText \"abc\"", msg)
	}

	msg, err = c.ReadMessage()
	if err != nil {
		t.Fatalf("read bell: %v", err)
	}
	if _, ok := msg.(*Bell); !ok {
		t.Fatalf("message = %#v, want Bell", msg)
	}
}

// Dial stops at the deadline when the server accepts the connection and
// sends nothing.
func TestDialHandshakeDeadline(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		if c, err := ln.Accept(); err == nil {
			accepted <- c
		}
	}()
	t.Cleanup(func() {
		select {
		case c := <-accepted:
			c.Close()
		default:
		}
	})

	err = within(t, "dial", func() error {
		c, err := dial(ln.Addr().String(), 100*time.Millisecond)
		if err == nil {
			c.Close()
		}
		return err
	})
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("dial error = %v, want a deadline error", err)
	}
}

// Dial uses one deadline for the connection and the handshake, so a slow
// connection leaves less time for the handshake.
func TestDialOneDeadline(t *testing.T) {
	f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
	addr := f.listen(t)

	const slow = 50 * time.Millisecond

	var dc *deadlineConn
	old := dialTCP
	t.Cleanup(func() { dialTCP = old })
	dialTCP = func(host string, deadline time.Time) (net.Conn, error) {
		time.Sleep(slow)
		c, err := old(host, deadline)
		if err != nil {
			return nil, err
		}
		dc = &deadlineConn{Conn: c}
		return dc, nil
	}

	start := time.Now()
	c, err := dial(addr, handshakeTimeout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	dc.mu.Lock()
	defer dc.mu.Unlock()

	if len(dc.set) == 0 {
		t.Fatal("dial set no deadline")
	}
	if d := dc.set[0].Sub(start); d < handshakeTimeout || d >= handshakeTimeout+slow {
		t.Errorf("handshake deadline is %v after the start of dial, want %v", d, handshakeTimeout)
	}
}

// deadlineConn is a net.Conn that keeps each deadline that is set on it.
// When short is not zero, it uses a deadline of short from now in place of
// each deadline that is not zero.
type deadlineConn struct {
	net.Conn
	short time.Duration

	mu  sync.Mutex
	set []time.Time
}

func (c *deadlineConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.set = append(c.set, t)
	c.mu.Unlock()

	if !t.IsZero() && c.short != 0 {
		t = time.Now().Add(c.short)
	}

	return c.Conn.SetDeadline(t)
}

// check fails the test unless the deadlines that were set are one deadline
// of timeout after start, and then no deadline.
func (c *deadlineConn) check(t *testing.T, name string, start time.Time, timeout time.Duration) {
	t.Helper()

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.set) != 2 || !c.set[1].IsZero() {
		t.Fatalf("%v deadlines = %v, want a deadline and then no deadline", name, c.set)
	}
	if d := c.set[0].Sub(start); d < timeout || d > timeout+testTimeout {
		t.Errorf("%v handshake deadline is %v after the start, want %v", name, d, timeout)
	}
}

// The handshake has a deadline, and the connection has no deadline after the
// handshake.
func TestDialClearsDeadline(t *testing.T) {
	f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			_, err := c.Write([]byte{TypeBell})
			return err
		}
		return nil
	}
	addr := f.listen(t)

	nc, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	dc := &deadlineConn{Conn: nc}

	start := time.Now()
	c, err := newConn(dc, start.Add(handshakeTimeout))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	dc.check(t, "client", start, handshakeTimeout)

	err = within(t, "read", func() error {
		msg, err := c.ReadMessage()
		if err == nil {
			if _, ok := msg.(*Bell); !ok {
				err = fmt.Errorf("message = %#v, want Bell", msg)
			}
		}
		return err
	})
	if err != nil {
		t.Errorf("read after the handshake: %v", err)
	}
}

// A server name above maxCutText gives an error before the client allocates
// the name.
func TestConnServerNameLimit(t *testing.T) {
	client, server := net.Pipe()
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})

	go func() {
		if _, err := io.WriteString(server, "RFB 003.003\n"); err != nil {
			return
		}
		if _, err := io.ReadFull(server, make([]byte, 12)); err != nil {
			return
		}
		if _, err := server.Write([]byte{0, 0, 0, securityNone}); err != nil {
			return
		}
		if _, err := io.ReadFull(server, make([]byte, 1)); err != nil {
			return
		}

		init := struct {
			Width, Height uint16
			PixelFormat   PixelFormat
			NameLength    uint32
		}{4, 4, clientFormat, maxCutText + 1}
		_ = binary.Write(server, binary.BigEndian, &init)
	}()

	err := within(t, "handshake", func() error {
		c, err := newConn(client, time.Now().Add(testTimeout))
		if err == nil {
			c.Close()
		}
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("handshake = %v, want a limit error", err)
	}
}

// dialTCP stops at the deadline. A deadline that has passed gives a timeout
// error at once, also for a server that accepts.
func TestDialTCPDeadline(t *testing.T) {
	f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
	addr := f.listen(t)

	start := time.Now()
	err := within(t, "dialTCP", func() error {
		c, err := dialTCP(addr, start.Add(-time.Second))
		if err == nil {
			c.Close()
		}
		return err
	})
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("dialTCP with a deadline that has passed = %v, want a timeout error", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("dialTCP took %v, want it to stop at once", d)
	}

	c, err := dialTCP(addr, time.Now().Add(testTimeout))
	if err != nil {
		t.Fatalf("dialTCP with a deadline in the future: %v", err)
	}
	c.Close()
}
