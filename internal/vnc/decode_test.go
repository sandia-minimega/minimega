// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"image"
	"net"
	"testing"
)

// T16: the decoder skips cursor rectangles and does not give pixel data for
// rectangles that are not pixel data.
func TestDecodeSkipsPseudoRectangles(t *testing.T) {
	reply := fbUpdate(
		cursorRect(3, 2),
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
	if !ok || len(u.Rectangles) != 3 {
		t.Fatalf("message = %#v, want an update with 3 rectangles", msg)
	}

	for i, enc := range []int32{CursorPseudoEncoding, DesktopSizePseudoEncoding} {
		if r := u.Rectangles[i]; r.EncodingType != enc || r.RGBA != nil {
			t.Errorf("rectangle %d: encoding %d, has pixels %v; want encoding %d and no pixels", i, r.EncodingType, r.RGBA != nil, enc)
		}
	}
	if r := u.Rectangles[2]; r.RGBA == nil || r.RGBA.Rect != image.Rect(0, 0, 2, 2) {
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
