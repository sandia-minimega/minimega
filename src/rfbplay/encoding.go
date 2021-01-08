// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"image/color"
	"io"
	log "minilog"
)

const (
	RawEncoding   = 0
	TightEncoding = 7
	DesktopSize   = -223
)

var Streams = make([]io.ReadCloser, 4)
var Buffers = make([]*bytes.Buffer, 4)

func resetStream(i int) {
	Buffers[i] = nil
	Streams[i] = nil
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

func DecodeTightEncoding(buf io.Reader, rect *Rectangle) (err error) {
	// Read the reset control
	var control byte
	if err = binary.Read(buf, binary.BigEndian, &control); err != nil {
		err = errors.New("unable to decode reset control")
		return
	}

	// Figure out whether we need to reset any streams or not
	for i := 0; i < 4; i++ {
		if control&byte(i) != 0 {
			log.Debugln("reset stream", i)
			resetStream(i)
		}
	}

	if control&0x80 == 0 {
		err = DecodeBasicCompression(buf, control, rect)
	} else if control&0xf0 == 0x80 {
		err = DecodeFillCompression(buf, rect)
	} else if control&0xf0 == 0x90 {
		// TODO: Implement
		err = errors.New("unimplemented: jpeg compression")
	} else {
		err = errors.New("unknown pixel compression")
	}

	return
}

func DecodeFillCompression(buf io.Reader, rect *Rectangle) error {
	log.Debugln("decoding fill compression")

	pixel, err := ReadPixel(buf)
	if err != nil {
		return err
	}

	log.Debug("pixel: %q", pixel)

	for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
		for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
			rect.Set(x, y, pixel)
		}
	}

	return nil
}

func DecodeBasicCompression(buf io.Reader, control byte, rect *Rectangle) error {
	log.Debugln("decoding basic compression")

	var filter PixelFilter
	var palette []color.RGBA

	// Figure out what stream we should write to
	stream := control >> 4 & 0x30
	log.Debugln("write stream", stream)

	if control&0x40 != 0 {
		log.Debugln("filter-id set")

		var filterID byte
		if err := binary.Read(buf, binary.BigEndian, &filterID); err != nil {
			log.Errorln(err)
			return errors.New("unable to decode filter ID")
		}

		log.Debug("filter-id: %d", filterID)

		if filterID == 1 {
			filter = PaletteFilter

			// The palette begins with an unsigned byte which value is the number of
			// colors in the palette minus 1 (i.e. 1 means 2 colors, 255 means 256
			// colors in the palette).
			var numColors byte
			if err := binary.Read(buf, binary.BigEndian, &numColors); err != nil {
				return errors.New("unable to decode palette filter number of colors")
			}
			numColors += 1

			log.Debugln("color palette size:", numColors)

			palette = make([]color.RGBA, numColors)

			for i := 0; i < int(numColors); i++ {
				pixel, err := ReadPixel(buf)
				if err != nil {
					return err
				}
				palette[i] = pixel
			}
		} else if filterID == 2 {
			// TODO: Implement
			filter = GradientFilter
			return errors.New("unimplemented: gradient filter")
		} else if filterID != 0 {
			return errors.New("unknown filter")
		}
	}

	var pixelReader io.Reader

	if rect.Rect.Dx()*rect.Rect.Dy()*int(pixelFormat.BitsPerPixel) < 12 {
		pixelReader = buf
	} else {
		clen, err := DecodeCLength(buf)
		if err != nil {
			return err
		}

		if Buffers[stream] == nil {
			Buffers[stream] = &bytes.Buffer{}
		}

		// Shove data into buffer
		n, err := io.CopyN(Buffers[stream], buf, int64(clen))
		if err != nil {
			return errors.New("unable to decompress")
		}

		log.Debug("wrote %d/%d compressed bytes to buffer", n, clen)

		if Streams[stream] == nil {
			reader, err := zlib.NewReader(Buffers[stream])
			if err != nil {
				panic(err)
			}
			Streams[stream] = reader
		}

		pixelReader = Streams[stream]
	}

	switch filter {
	case CopyFilter:
		log.Debugln("reading copy filtered pixels")
		for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
			for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
				pixel, err := ReadPixel(pixelReader)
				if err != nil {
					return err
				}
				rect.Set(x, y, pixel)
			}
		}
	case PaletteFilter:
		log.Debugln("reading palette filtered pixels")
		if len(palette) == 2 {
			var k byte
			var count int
			for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
				for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
					// Read a byte every 8 pixels
					if count%8 == 0 {
						if err := binary.Read(pixelReader, binary.BigEndian, &k); err != nil {
							return errors.New("unable to decode color index")
						}
					}

					index := (k >> uint(7-(count%8))) & 0x01
					rect.Set(x, y, palette[index])
					count += 1
				}
			}
		} else {
			for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
				for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
					var k byte
					if err := binary.Read(pixelReader, binary.BigEndian, &k); err != nil {
						return errors.New("unable to decode color index")
					}

					rect.Set(x, y, palette[k])
				}
			}
		}
	case GradientFilter:
		// TODO: Implement
		log.Debugln("unimplemented: gradient filter")
	}

	return nil
}

func DecodeCLength(buf io.Reader) (res uint, err error) {
	var b byte

	for i := 0; i < 3; i++ {
		if err2 := binary.Read(buf, binary.BigEndian, &b); err2 != nil {
			err = errors.New("unable to decode compressed data length")
			return
		}

		res += uint(b&0x7f) << uint(7*i)
		if b&0x80 == 0 {
			break
		}
	}

	return
}
