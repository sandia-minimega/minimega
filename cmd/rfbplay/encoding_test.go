// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"compress/zlib"
	"context"
	"image"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecodeCLength(t *testing.T) {
	tests := []struct {
		in   []byte
		want uint
	}{
		{[]byte{0x05}, 5},
		{[]byte{0x90, 0x4e}, 10000},
		{[]byte{0xff, 0x7f}, 0x3fff},
		// the third byte uses all 8 bits
		{[]byte{0x80, 0x80, 0x81}, 0x81 << 14},
		{[]byte{0xff, 0xff, 0xff}, 0x3fffff},
	}

	for _, tt := range tests {
		r := bytes.NewReader(tt.in)
		got, err := DecodeCLength(r)
		if err != nil {
			t.Fatalf("% x: %v", tt.in, err)
		}
		if got != tt.want {
			t.Errorf("% x = %d, want %d", tt.in, got, tt.want)
		}
		if r.Len() != 0 {
			t.Errorf("% x: %d bytes not read", tt.in, r.Len())
		}
	}

	if _, err := DecodeCLength(bytes.NewReader([]byte{0x80})); err == nil {
		t.Error("short compact length: no error")
	}
}

// tightEncoder makes the zlib data of Tight rectangles the way QEMU does: one
// zlib stream for each stream ID, with a sync flush after each rectangle.
type tightEncoder struct {
	out [4]*bytes.Buffer
	zw  [4]*zlib.Writer
}

func (e *tightEncoder) reset(id int) {
	e.zw[id] = nil
}

// data returns the data part of a basic rectangle: the raw bytes when there
// are fewer than 12, else the compact length and the zlib data.
func (e *tightEncoder) data(t testing.TB, id int, raw []byte) []byte {
	t.Helper()

	if len(raw) < 12 {
		return raw
	}

	if e.zw[id] == nil {
		e.out[id] = new(bytes.Buffer)
		e.zw[id] = zlib.NewWriter(e.out[id])
	}
	if _, err := e.zw[id].Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := e.zw[id].Flush(); err != nil {
		t.Fatal(err)
	}

	z := bytes.Clone(e.out[id].Bytes())
	e.out[id].Reset()

	return append(compactLen(len(z)), z...)
}

// compactLen encodes n like tight_send_compact_size in QEMU.
func compactLen(n int) []byte {
	b := []byte{byte(n & 0x7f)}
	if n > 0x7f {
		b[0] |= 0x80
		b = append(b, byte(n>>7&0x7f))
		if n > 0x3fff {
			b[1] |= 0x80
			b = append(b, byte(n>>14))
		}
	}
	return b
}

// testColor returns a color that is different for each n below 256.
func testColor(n int) color.RGBA {
	return color.RGBA{byte(n), byte(255 - n), byte(n*7 + 3), 255}
}

func packTPixels(cs ...color.RGBA) []byte {
	var b []byte
	for _, c := range cs {
		b = append(b, c.R, c.G, c.B)
	}
	return b
}

func gradientImage(w, h int) []color.RGBA {
	cs := make([]color.RGBA, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cs[y*w+x] = color.RGBA{byte(x * 20), byte(y * 30), byte(x*y + 250), 255}
		}
	}
	return cs
}

// gradientFilter applies the Tight gradient filter to cs.
func gradientFilter(w, h int, cs []color.RGBA) []byte {
	v := func(x, y, c int) int {
		if x < 0 || y < 0 {
			return 0
		}
		p := cs[y*w+x]
		return int([]byte{p.R, p.G, p.B}[c])
	}

	var out []byte
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 3; c++ {
				p := min(max(v(x-1, y, c)+v(x, y-1, c)-v(x-1, y-1, c), 0), 255)
				out = append(out, byte(v(x, y, c)-p))
			}
		}
	}
	return out
}

type tightStep struct {
	name string
	w, h int
	data []byte
	want []color.RGBA
}

// copyStep makes a CopyFilter rectangle of w x h pixels for stream id.
func copyStep(t testing.TB, e *tightEncoder, name string, id int, ctl byte, w, h, seed int) tightStep {
	want := make([]color.RGBA, w*h)
	for i := range want {
		want[i] = testColor(seed + i)
	}

	data := []byte{ctl | byte(id)<<4}
	data = append(data, e.data(t, id, packTPixels(want...))...)

	return tightStep{name, w, h, data, want}
}

// paletteStep makes a PaletteFilter rectangle of w x h pixels with n colors
// in stream id. index gives the palette index of each pixel.
func paletteStep(t testing.TB, e *tightEncoder, name string, id, n, w, h int, index func(x, y int) int) tightStep {
	palette := make([]color.RGBA, n)
	for i := range palette {
		palette[i] = testColor(255 - i)
	}

	want := make([]color.RGBA, w*h)
	var raw []byte
	if n == 2 {
		row := (w + 7) / 8
		raw = make([]byte, row*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := index(x, y)
				want[y*w+x] = palette[i]
				raw[y*row+x/8] |= byte(i) << (7 - x%8)
			}
		}
	} else {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := index(x, y)
				want[y*w+x] = palette[i]
				raw = append(raw, byte(i))
			}
		}
	}

	data := []byte{(0x4 | byte(id)) << 4, 1, byte(n - 1)}
	data = append(data, packTPixels(palette...)...)
	data = append(data, e.data(t, id, raw)...)

	return tightStep{name, w, h, data, want}
}

func goldenSteps(t testing.TB) []tightStep {
	e := &tightEncoder{}

	var steps []tightStep

	// each of the four zlib streams, two times, so that each stream must keep
	// its state from one rectangle to the next
	for round := 0; round < 2; round++ {
		for id := 0; id < 4; id++ {
			steps = append(steps, copyStep(t, e, "copy stream", id, 0, 4, 3, 10*id+round))
		}
	}

	// reset stream 0 only: streams 1 and 3 keep their state
	e.reset(0)
	steps = append(steps,
		copyStep(t, e, "reset stream 0", 0, 0x01, 3, 4, 50),
		copyStep(t, e, "stream 1 after reset 0", 1, 0, 3, 4, 60),
		copyStep(t, e, "stream 3 after reset 0", 3, 0, 3, 4, 70),
	)

	// reset each stream with its own bit
	for id := 0; id < 4; id++ {
		e.reset(id)
		steps = append(steps, copyStep(t, e, "reset own stream", id, 1<<id, 5, 2, 80+id))
	}

	// reset all four streams in one rectangle
	for id := 0; id < 4; id++ {
		e.reset(id)
	}
	steps = append(steps, copyStep(t, e, "reset all", 2, 0x0f, 4, 4, 90))
	for _, id := range []int{0, 1, 3} {
		steps = append(steps, copyStep(t, e, "after reset all", id, 0, 4, 4, 100+id))
	}

	// small rectangles are not compressed: 3 pixels of 3 bytes is 9 bytes,
	// and 4 pixels is the 12-byte threshold
	steps = append(steps,
		copyStep(t, e, "uncompressed 1x3", 0, 0, 1, 3, 110),
		copyStep(t, e, "uncompressed 3x1", 0, 0, 3, 1, 115),
		copyStep(t, e, "threshold 2x2", 0, 0, 2, 2, 120),
	)
	// the explicit copy filter has a filter byte
	explicit := copyStep(t, e, "explicit copy filter", 1, 0, 2, 1, 125)
	explicit.data = append([]byte{explicit.data[0] | 0x40, 0}, explicit.data[1:]...)
	steps = append(steps, explicit)

	steps = append(steps,
		paletteStep(t, e, "256-color palette", 2, 256, 16, 16, func(x, y int) int { return (y*16 + x*7) % 256 }),
		paletteStep(t, e, "3-color palette uncompressed", 2, 3, 3, 3, func(x, y int) int { return (x + y) % 3 }),
		paletteStep(t, e, "2-color odd width compressed", 1, 2, 11, 8, func(x, y int) int { return (x*x + y) % 2 }),
		paletteStep(t, e, "2-color odd width uncompressed", 1, 2, 5, 2, func(x, y int) int { return (x + y) % 2 }),
		paletteStep(t, e, "2-color one row", 1, 2, 9, 1, func(x, y int) int { return x % 2 }),
	)

	// gradient filter in stream 3, as QEMU sends it
	gw, gh := 7, 5
	gwant := gradientImage(gw, gh)
	gdata := append([]byte{(0x4 | 3) << 4, 2}, e.data(t, 3, gradientFilter(gw, gh, gwant))...)
	steps = append(steps, tightStep{"gradient", gw, gh, gdata, gwant})

	// fill
	steps = append(steps, tightStep{"fill", 6, 2, append([]byte{0x80}, packTPixels(testColor(7))...), fill(12, testColor(7))})

	// incompressible data needs a 3-byte compact length
	rng := rand.New(rand.NewSource(1))
	raw := make([]byte, 100*100*3)
	rng.Read(raw)
	big := tightStep{name: "3-byte compact length", w: 100, h: 100, want: make([]color.RGBA, 100*100)}
	for i := range big.want {
		big.want[i] = color.RGBA{raw[3*i], raw[3*i+1], raw[3*i+2], 255}
	}
	big.data = append([]byte{0}, e.data(t, 0, raw)...)
	if big.data[1]&0x80 == 0 || big.data[2]&0x80 == 0 {
		t.Fatal("test data does not have a 3-byte compact length")
	}
	steps = append(steps, big)

	// stream 0 still works after the large rectangle
	steps = append(steps, copyStep(t, e, "stream 0 after large", 0, 0, 4, 4, 130))

	return steps
}

func fill(n int, c color.RGBA) []color.RGBA {
	cs := make([]color.RGBA, n)
	for i := range cs {
		cs[i] = c
	}
	return cs
}

func decodeTightStep(dec *tightDecoder, s tightStep) (*Rectangle, int, error) {
	rect := &Rectangle{
		RGBA:         image.NewRGBA(image.Rect(10, 20, 10+s.w, 20+s.h)),
		EncodingType: TightEncoding,
	}
	r := bytes.NewReader(s.data)
	err := dec.Decode(r, rect)
	return rect, r.Len(), err
}

func checkTightStep(t *testing.T, i int, dec *tightDecoder, s tightStep) {
	t.Helper()

	rect, left, err := decodeTightStep(dec, s)
	if err != nil {
		t.Fatalf("step %d (%s): %v", i, s.name, err)
	}
	if left != 0 {
		t.Errorf("step %d (%s): %d bytes not read", i, s.name, left)
	}

	bad := 0
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			got := rect.RGBAAt(10+x, 20+y)
			if want := s.want[y*s.w+x]; got != want && bad < 3 {
				t.Errorf("step %d (%s): pixel (%d,%d) = %v, want %v", i, s.name, x, y, got, want)
				bad++
			}
		}
	}
}

// T20: hand-built Tight rectangles decode to the expected pixels.
func TestTightGolden(t *testing.T) {
	dec := &tightDecoder{}
	for i, s := range goldenSteps(t) {
		checkTightStep(t, i, dec, s)
	}
}

// T20: two decoders do not share zlib state.
func TestTightDecodersIndependent(t *testing.T) {
	steps := goldenSteps(t)[:8]

	a, b := &tightDecoder{}, &tightDecoder{}
	for i, s := range steps {
		checkTightStep(t, i, a, s)
		checkTightStep(t, i, b, s)
	}
}

// T20: two requests for one Tight recording do not share zlib state.
func TestReadFileTightTwice(t *testing.T) {
	steps := goldenSteps(t)[:8]

	var rects [][]byte
	for _, s := range steps {
		rects = append(rects, append(rectHeader(0, 0, s.w, s.h, TightEncoding), s.data...))
	}
	path := filepath.Join(t.TempDir(), "tight.fb")
	writeRecording(t, path, chunk{0, fbUpdate(rects...)})

	for n := 0; n < 2; n++ {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		updates, err := readFile(context.Background(), f)
		if err != nil {
			t.Fatal(err)
		}

		var got []*FramebufferUpdate
		timeout := time.After(testTimeout)
	read:
		for {
			select {
			case u, ok := <-updates:
				if !ok {
					break read
				}
				got = append(got, u)
			case <-timeout:
				t.Fatal("timeout reading updates")
			}
		}
		f.Close()

		if len(got) != 1 || len(got[0].Rectangles) != len(steps) {
			t.Fatalf("request %d: got %d updates, want 1 with %d rectangles", n, len(got), len(steps))
		}
		for i, s := range steps {
			r := got[0].Rectangles[i]
			if c := r.RGBAAt(s.w-1, s.h-1); c != s.want[len(s.want)-1] {
				t.Errorf("request %d rectangle %d: last pixel = %v, want %v", n, i, c, s.want[len(s.want)-1])
			}
		}
	}
}

// T20: bad Tight rectangles give an error, not a panic.
func TestTightErrors(t *testing.T) {
	e := &tightEncoder{}
	tests := []tightStep{
		{name: "palette index out of range", w: 2, h: 1, data: append(append([]byte{0x40, 1, 2}, packTPixels(testColor(1), testColor(2), testColor(3))...), 0, 3)},
		{name: "1-color palette index out of range", w: 2, h: 1, data: append(append([]byte{0x40, 1, 0}, packTPixels(testColor(1))...), 0, 1)},
		{name: "unknown filter", w: 2, h: 2, data: []byte{0x40, 3}},
		{name: "unknown compression", w: 2, h: 2, data: []byte{0xa0}},
		{name: "jpeg", w: 2, h: 2, data: []byte{0x90, 1, 0}},
		{name: "bad zlib header", w: 4, h: 4, data: []byte{0x00, 4, 1, 2, 3, 4}},
		{name: "short compressed data", w: 4, h: 4, data: []byte{0x00, 100, 1, 2}},
		{name: "short zlib data", w: 4, h: 4, data: append([]byte{0x00, 4}, e.data(t, 0, make([]byte, 48))[1:5]...)},
		{name: "short fill", w: 4, h: 4, data: []byte{0x80, 1, 2}},
		{name: "empty", w: 4, h: 4},
	}

	for _, tt := range tests {
		if _, _, err := decodeTightStep(&tightDecoder{}, tt); err == nil {
			t.Errorf("%s: no error", tt.name)
		}
	}
}

// T25: the Tight decoder does not panic on any rectangle.
func FuzzTightRect(f *testing.F) {
	for _, s := range goldenSteps(f) {
		if len(s.data) < 4096 {
			f.Add(byte(s.w), byte(s.h), s.data)
		}
	}

	f.Fuzz(func(t *testing.T, w, h byte, data []byte) {
		dec := &tightDecoder{}
		rect := &Rectangle{
			RGBA:         image.NewRGBA(image.Rect(0, 0, int(w%64), int(h%64))),
			EncodingType: TightEncoding,
		}

		r := bytes.NewReader(data)
		for i := 0; i < 8 && r.Len() > 0; i++ {
			if err := dec.Decode(r, rect); err != nil {
				return
			}
		}
	})
}
