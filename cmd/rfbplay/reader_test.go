// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/vnc"
)

const testTimeout = 5 * time.Second

// fbFrames is the number of updates that fakeFBServer sends.
const fbFrames = 5

// fakeFBServer accepts one connection, completes an RFB 3.3 handshake, and
// answers each FramebufferUpdateRequest with a 4x3 Raw update whose pixels
// have the value of the update number. It closes the connection after
// fbFrames updates and then closes done.
func fakeFBServer(t *testing.T) (string, <-chan struct{}) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	done := make(chan struct{})
	go func() {
		defer close(done)

		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()

		if _, err := io.WriteString(c, "RFB 003.003\n"); err != nil {
			return
		}
		if _, err := io.ReadFull(c, make([]byte, 12)); err != nil {
			return
		}
		if err := binary.Write(c, binary.BigEndian, uint32(1)); err != nil {
			return
		}
		if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
			return
		}

		init := struct {
			Width, Height uint16
			PixelFormat   vnc.PixelFormat
			NameLength    uint32
		}{Width: 640, Height: 480}
		if err := binary.Write(c, binary.BigEndian, &init); err != nil {
			return
		}

		for n := 1; n <= fbFrames; {
			m, err := vnc.ReadClientMessage(c)
			if err != nil {
				return
			}
			if _, ok := m.(*vnc.FramebufferUpdateRequest); !ok {
				continue
			}

			b := []byte{0, 0, 0, 1}
			b = binary.BigEndian.AppendUint16(b, 0)
			b = binary.BigEndian.AppendUint16(b, 0)
			b = binary.BigEndian.AppendUint16(b, 4)
			b = binary.BigEndian.AppendUint16(b, 3)
			b = binary.BigEndian.AppendUint32(b, vnc.RawEncoding)
			for i := 0; i < 4*3; i++ {
				b = binary.LittleEndian.AppendUint32(b, uint32(n))
			}
			if _, err := c.Write(b); err != nil {
				return
			}
			n++
		}
	}()

	return ln.Addr().String(), done
}

// gzipComplete reports whether path holds a complete gzip stream.
func gzipComplete(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	zr, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	_, err = io.Copy(io.Discard, zr)
	return err == nil
}

// T10: rfbplay reads every frame of an fb recording that minimega made.
func TestReadFBRecording(t *testing.T) {
	addr, done := fakeFBServer(t)
	path := filepath.Join(t.TempDir(), "fb")

	rec := vnc.NewRecorder()
	if err := rec.RecordFB("vm", addr, path); err != nil {
		t.Fatal(err)
	}

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatal("timeout waiting for the server to send all frames")
	}

	// the recorder reads to EOF after the server closes, then ends the file
	deadline := time.Now().Add(testTimeout)
	for !gzipComplete(path) {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for a complete fb file")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := rec.StopFB("vm"); err != nil {
		t.Fatalf("StopFB: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	updates, err := readFile(context.Background(), f)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}

	n := 0
	timeout := time.After(testTimeout)
	for {
		select {
		case u, ok := <-updates:
			if !ok {
				if n != fbFrames {
					t.Errorf("read %d frames, want %d", n, fbFrames)
				}
				return
			}

			n++
			if len(u.Rectangles) != 1 {
				t.Fatalf("frame %d has %d rectangles, want 1", n, len(u.Rectangles))
			}
			r := u.Rectangles[0]
			if r.Rect != image.Rect(0, 0, 4, 3) {
				t.Errorf("frame %d bounds = %v, want (0,0)-(4,3)", n, r.Rect)
			}
			if got, want := r.RGBAAt(3, 2), (color.RGBA{0, 0, uint8(n), 255}); got != want {
				t.Errorf("frame %d pixel = %v, want %v", n, got, want)
			}
		case <-timeout:
			t.Fatal("timeout reading frames")
		}
	}
}

// ReadRectangle refuses a rectangle that ends outside maxRectDim, before it
// allocates the image.
func TestReadRectangleLimit(t *testing.T) {
	tests := []struct {
		x, y, w, h uint16
		ok         bool
	}{
		{0xffff, 0xffff, 0xffff, 0xffff, false},
		{0, 0, 0xffff, 1, false},
		{0, 0, 1, 0xffff, false},
		{maxRectDim, 0, 1, 1, false},
		{0, maxRectDim, 1, 1, false},
		{maxRectDim - 1, maxRectDim - 1, 1, 1, true},
	}

	for _, tt := range tests {
		b := binary.BigEndian.AppendUint16(nil, tt.x)
		b = binary.BigEndian.AppendUint16(b, tt.y)
		b = binary.BigEndian.AppendUint16(b, tt.w)
		b = binary.BigEndian.AppendUint16(b, tt.h)
		b = binary.BigEndian.AppendUint32(b, uint32(RawEncoding))

		rect, err := ReadRectangle(bytes.NewReader(b))
		if tt.ok && err != nil {
			t.Errorf("%dx%d at (%d, %d): %v", tt.w, tt.h, tt.x, tt.y, err)
		}
		if !tt.ok && (err == nil || rect.RGBA != nil) {
			t.Errorf("%dx%d at (%d, %d): got no error", tt.w, tt.h, tt.x, tt.y)
		}
	}
}
