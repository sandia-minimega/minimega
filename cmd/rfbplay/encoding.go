// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"image/color"
	"io"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	RawEncoding   = 0
	TightEncoding = 7
	DesktopSize   = -223
)

// tightMinToCompress is the data size in bytes below which a Tight server
// sends the data with no compression.
const tightMinToCompress = 12

// The compression control byte that starts each Tight rectangle. See "Tight
// Encoding" in the RFB protocol community edition (rfbproto).
const (
	// tightStreams is the number of zlib streams. Bits 0 to 3 reset streams
	// 0 to 3.
	tightStreams = 4

	// tightBasic is clear for basic compression. Otherwise the high four bits
	// (tightTypeMask) give the compression type.
	tightBasic    = 0x80
	tightTypeMask = 0xf0
	tightFill     = 0x80
	tightJPEG     = 0x90

	// For basic compression, bits 4 and 5 give the zlib stream, and bit 6
	// says that a filter ID follows.
	tightStreamShift    = 4
	tightStreamMask     = 0x3
	tightExplicitFilter = 0x40
)

// A Tight compact length has one to three bytes. The first two give 7 bits
// each and set clenMore when another byte follows. The third gives 8 bits.
const (
	clenBits = 7
	clenMask = 0x7f
	clenMore = 0x80
)

// tightDecoder decodes Tight rectangles. A Tight server keeps one zlib stream
// for each of four stream IDs across the rectangles of a connection, so each
// recording that is read needs its own tightDecoder.
type tightDecoder struct {
	streams [tightStreams]zlibStream
}

// zlibStream is one zlib stream of a Tight connection. The server sends the
// stream in parts, one part for each rectangle.
type zlibStream struct {
	in bytes.Buffer
	zr io.ReadCloser
}

func (s *zlibStream) reset() {
	s.in.Reset()
	s.zr = nil
}

// inflate adds clen compressed bytes from r to the stream and returns the
// next size bytes of uncompressed data.
func (s *zlibStream) inflate(r io.Reader, clen, size int) ([]byte, error) {
	if _, err := io.CopyN(&s.in, r, int64(clen)); err != nil {
		return nil, fmt.Errorf("unable to read compressed data: %w", err)
	}

	if s.zr == nil {
		zr, err := zlib.NewReader(&s.in)
		if err != nil {
			return nil, fmt.Errorf("unable to start zlib stream: %w", err)
		}
		s.zr = zr
	}

	out := make([]byte, size)
	if _, err := io.ReadFull(s.zr, out); err != nil {
		return nil, fmt.Errorf("unable to decompress: %w", err)
	}

	return out, nil
}

func DecodeRawEncoding(buf io.Reader, rect *Rectangle) error {
	for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
		for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
			pixel, err := ReadPixel(buf)
			if err != nil {
				log.Error("error reading pixel %d, %d", x, y)
				return err
			}
			rect.Set(x, y, pixel)
		}
	}

	return nil
}

func DecodeDesktopSizeEncoding(buf io.Reader, rect *Rectangle) error {
	log.Debug("new desktop size: %d x %d", rect.Rect.Dx(), rect.Rect.Dy())

	return nil
}

// tpixelSize returns the size of a Tight TPIXEL. It is 3 bytes (red, green,
// and blue) for a 24-bit true color format in 32 bits, else the pixel size.
func tpixelSize() int {
	pf := pixelFormat
	if pf.BitsPerPixel == 32 && pf.Depth == 24 && pf.TrueColorFlag != 0 &&
		pf.RedMax == 255 && pf.GreenMax == 255 && pf.BlueMax == 255 {
		return 3
	}
	return int(pf.BitsPerPixel / 8)
}

// tpixels decodes the TPIXEL values in b.
func tpixels(b []byte) ([]color.RGBA, error) {
	size := tpixelSize()
	cs := make([]color.RGBA, len(b)/size)

	for i := range cs {
		p := b[i*size : (i+1)*size]
		if size == 3 {
			cs[i] = color.RGBA{p[0], p[1], p[2], 255}
			continue
		}

		c, err := ReadPixel(bytes.NewReader(p))
		if err != nil {
			return nil, err
		}
		cs[i] = c
	}

	return cs, nil
}

func readTPixels(r io.Reader, n int) ([]color.RGBA, error) {
	b := make([]byte, n*tpixelSize())
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, errors.New("unable to read pixel data")
	}
	return tpixels(b)
}

// setPixels sets the pixels of rect from cs in row order.
func setPixels(rect *Rectangle, cs []color.RGBA) {
	w := rect.Rect.Dx()
	for i, c := range cs {
		rect.SetRGBA(rect.Rect.Min.X+i%w, rect.Rect.Min.Y+i/w, c)
	}
}

// Decode decodes one Tight rectangle from r into rect.
func (d *tightDecoder) Decode(r io.Reader, rect *Rectangle) error {
	var ctl [1]byte
	if _, err := io.ReadFull(r, ctl[:]); err != nil {
		return errors.New("unable to decode compression control")
	}

	// bits 0 to 3 tell the decoder to reset zlib streams 0 to 3
	for i := range d.streams {
		if ctl[0]&(1<<i) != 0 {
			log.Debugln("reset stream", i)
			d.streams[i].reset()
		}
	}

	switch {
	case ctl[0]&tightBasic == 0:
		return d.decodeBasic(r, ctl[0], rect)
	case ctl[0]&tightTypeMask == tightFill:
		return decodeFill(r, rect)
	case ctl[0]&tightTypeMask == tightJPEG:
		return errors.New("unimplemented: jpeg compression")
	default:
		return fmt.Errorf("unknown tight compression control %#x", ctl[0])
	}
}

func decodeFill(r io.Reader, rect *Rectangle) error {
	cs, err := readTPixels(r, 1)
	if err != nil {
		return err
	}

	for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
		for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
			rect.SetRGBA(x, y, cs[0])
		}
	}

	return nil
}

// readData reads size bytes of filtered data from r. The data is compressed
// in stream s when size is 12 bytes or more.
func readData(r io.Reader, s *zlibStream, size int) ([]byte, error) {
	if size < tightMinToCompress {
		b := make([]byte, size)
		if _, err := io.ReadFull(r, b); err != nil {
			return nil, errors.New("unable to read uncompressed data")
		}
		return b, nil
	}

	clen, err := DecodeCLength(r)
	if err != nil {
		return nil, err
	}

	return s.inflate(r, int(clen), size)
}

func (d *tightDecoder) decodeBasic(r io.Reader, ctl byte, rect *Rectangle) error {
	s := &d.streams[(ctl>>tightStreamShift)&tightStreamMask]

	filter := CopyFilter
	if ctl&tightExplicitFilter != 0 {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return errors.New("unable to decode filter ID")
		}
		filter = PixelFilter(b[0])
	}

	w, h := rect.Rect.Dx(), rect.Rect.Dy()

	switch filter {
	case CopyFilter:
		data, err := readData(r, s, w*h*tpixelSize())
		if err != nil {
			return err
		}
		cs, err := tpixels(data)
		if err != nil {
			return err
		}
		setPixels(rect, cs)
	case PaletteFilter:
		return decodePalette(r, s, rect)
	case GradientFilter:
		return decodeGradient(r, s, rect)
	default:
		return fmt.Errorf("unknown tight filter %d", filter)
	}

	return nil
}

// decodePalette decodes a rectangle with the palette filter. A palette of two
// colors uses one bit for each pixel, and each row starts on a byte boundary.
// Other palettes use one byte for each pixel.
func decodePalette(r io.Reader, s *zlibStream, rect *Rectangle) error {
	var n [1]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return errors.New("unable to decode palette size")
	}

	palette, err := readTPixels(r, int(n[0])+1)
	if err != nil {
		return err
	}

	w, h := rect.Rect.Dx(), rect.Rect.Dy()

	if len(palette) == 2 {
		row := (w + 7) / 8
		data, err := readData(r, s, row*h)
		if err != nil {
			return err
		}

		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := data[y*row+x/8] >> (7 - x%8) & 1
				rect.SetRGBA(rect.Rect.Min.X+x, rect.Rect.Min.Y+y, palette[i])
			}
		}
		return nil
	}

	data, err := readData(r, s, w*h)
	if err != nil {
		return err
	}

	cs := make([]color.RGBA, len(data))
	for i, k := range data {
		if int(k) >= len(palette) {
			return fmt.Errorf("palette index %d out of range for %d colors", k, len(palette))
		}
		cs[i] = palette[k]
	}
	setPixels(rect, cs)

	return nil
}

// decodeGradient decodes a rectangle with the gradient filter. Each value is
// the difference from a prediction made from the pixels to the left, above,
// and above left.
func decodeGradient(r io.Reader, s *zlibStream, rect *Rectangle) error {
	if tpixelSize() != 3 {
		return errors.New("unimplemented: gradient filter for this pixel format")
	}

	w, h := rect.Rect.Dx(), rect.Rect.Dy()
	data, err := readData(r, s, w*h*3)
	if err != nil {
		return err
	}

	at := func(x, y, c int) int {
		if x < 0 || y < 0 {
			return 0
		}
		return int(data[(y*w+x)*3+c])
	}

	// decode in place: each value only uses values that come before it
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			for c := 0; c < 3; c++ {
				p := min(max(at(x-1, y, c)+at(x, y-1, c)-at(x-1, y-1, c), 0), 255)
				data[(y*w+x)*3+c] += byte(p)
			}
		}
	}

	cs, err := tpixels(data)
	if err != nil {
		return err
	}
	setPixels(rect, cs)

	return nil
}

// DecodeCLength decodes a Tight compact length of one to three bytes. The
// first two bytes give 7 bits each, and the third byte gives 8 bits.
func DecodeCLength(r io.Reader) (uint, error) {
	var b [1]byte
	var res uint

	for i := 0; i < 3; i++ {
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, errors.New("unable to decode compressed data length")
		}

		if i == 2 {
			res |= uint(b[0]) << (2 * clenBits)
			break
		}

		res |= uint(b[0]&clenMask) << (clenBits * i)
		if b[0]&clenMore == 0 {
			break
		}
	}

	return res, nil
}
