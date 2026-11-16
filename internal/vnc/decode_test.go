// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"fmt"
	"image"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The decoder gives pixel data only for Raw rectangles, and a DesktopSize
// rectangle changes the size of the connection.
func TestDecodePseudoRectangles(t *testing.T) {
	reply := fbUpdate(
		rect(0, 0, 800, 600, DesktopSizePseudoEncoding),
		rawRect(0, 0, 2, 2, 0x00ff0000),
	)
	reply = append(reply, TypeBell)

	f := &fakeRFB{width: 640, height: 480, pf: clientFormat}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			_, err := c.Write(reply)
			return err
		}
		return nil
	}

	c, err := Dial(f.listen(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	msg, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read update: %v", err)
	}
	u, ok := msg.(*FramebufferUpdate)
	if !ok || len(u.Rectangles) != 2 {
		t.Fatalf("message = %#v, want an update with 2 rectangles", msg)
	}

	if r := u.Rectangles[0]; r.EncodingType != DesktopSizePseudoEncoding || r.RGBA != nil {
		t.Errorf("rectangle 0: encoding %d, has pixels %v; want DesktopSize and no pixels", r.EncodingType, r.RGBA != nil)
	}
	if r := u.Rectangles[1]; r.RGBA == nil || r.RGBA.Rect != image.Rect(0, 0, 2, 2) {
		t.Errorf("raw rectangle = %#v, want pixels at (0,0)-(2,2)", r)
	} else if px := r.RGBAAt(1, 1); px.R != 0xff || px.G != 0 || px.B != 0 {
		t.Errorf("raw pixel = %v, want red", px)
	}

	if w, h := c.size(); w != 800 || h != 600 {
		t.Errorf("size = %dx%d, want 800x600", w, h)
	}

	msg, err = c.ReadMessage()
	if err != nil {
		t.Fatalf("read bell: %v", err)
	}
	if _, ok := msg.(*Bell); !ok {
		t.Fatalf("message = %#v, want Bell", msg)
	}
}

// A Raw rectangle outside the desktop gives an error before the decoder
// allocates its pixels.
func TestDecodeRectOutsideDesktop(t *testing.T) {
	tests := []struct {
		name string
		rect []byte
	}{
		{"wide", rawRect(0, 0, 5, 1, 0)},
		{"tall", rawRect(0, 0, 1, 5, 0)},
		{"offset", rawRect(3, 3, 2, 1, 0)},
		{"huge", rect(0, 0, 0xffff, 0xffff, RawEncoding)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply := append(fbUpdate(tt.rect), TypeBell)

			f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
			f.handle = func(c net.Conn, m ClientMessage) error {
				if _, ok := m.(*SetEncodings); ok {
					_, err := c.Write(reply)
					return err
				}
				return nil
			}

			c, err := Dial(f.listen(t))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.Close() })

			msg, err := c.ReadMessage()
			if err == nil || !strings.Contains(err.Error(), "outside the 4 x 4 desktop") {
				t.Fatalf("ReadMessage = %#v, %v; want an outside the desktop error", msg, err)
			}
		})
	}
}

// countingConn counts the Read calls of a net.Conn.
type countingConn struct {
	net.Conn
	reads atomic.Int32
}

func (c *countingConn) Read(b []byte) (int, error) {
	c.reads.Add(1)
	return c.Conn.Read(b)
}

// The decoder reads the pixels through a buffer, not one pixel for each
// Read call.
func TestDecodeBufferedReads(t *testing.T) {
	const w, h = 64, 64
	reply := fbUpdate(rawRect(0, 0, w, h, 0x00ff0000))

	f := &fakeRFB{width: w, height: h, pf: clientFormat}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			_, err := c.Write(reply)
			return err
		}
		return nil
	}

	client, server := net.Pipe()
	served := make(chan struct{})
	go func() {
		defer close(served)
		defer server.Close()
		_ = f.serve(server)
	}()
	t.Cleanup(func() {
		client.Close()
		<-served
	})

	cc := &countingConn{Conn: client}
	c, err := newConn(cc, time.Now().Add(testTimeout))
	if err != nil {
		t.Fatal(err)
	}

	cc.reads.Store(0)
	msg, err := c.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	if u, ok := msg.(*FramebufferUpdate); !ok || len(u.Rectangles) != 1 || u.Rectangles[0].RGBAAt(w-1, h-1).R != 0xff {
		t.Fatalf("message = %#v, want one red rectangle", msg)
	}
	if n := cc.reads.Load(); n > 64 {
		t.Errorf("%d Read calls for %d pixels, want at most 64", n, w*h)
	}
}

// A ServerCutText above maxCutText gives an error before the decoder
// allocates the text. A ServerCutText at the limit is read.
func TestDecodeServerCutTextLimit(t *testing.T) {
	cutText := func(n uint32) []byte {
		return binary.BigEndian.AppendUint32([]byte{TypeServerCutText, 0, 0, 0}, n)
	}
	atLimit := append(cutText(maxCutText), make([]byte, maxCutText)...)

	f := &fakeRFB{width: 4, height: 4, pf: clientFormat}
	f.handle = func(c net.Conn, m ClientMessage) error {
		if _, ok := m.(*SetEncodings); ok {
			_, err := c.Write(append(atLimit, cutText(maxCutText+1)...))
			return err
		}
		return nil
	}

	c, err := Dial(f.listen(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	err = within(t, "read", func() error {
		msg, err := c.ReadMessage()
		if err != nil {
			return err
		}
		if ct, ok := msg.(*ServerCutText); !ok || len(ct.Text) != maxCutText {
			return fmt.Errorf("message = %T, want ServerCutText with %d bytes", msg, maxCutText)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read cut text at the limit: %v", err)
	}

	err = within(t, "read", func() error {
		_, err := c.ReadMessage()
		return err
	})
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("read cut text above the limit = %v, want a limit error", err)
	}
}
