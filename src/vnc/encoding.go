// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

import (
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"io"
	"log"
)

const (
	RawEncoding = 0
	DesktopSize = -223
)

func (s *Server) readPixel(reader io.Reader) (c color.RGBA64, err error) {
	var n int

	bytesPerPixel := s.BitsPerPixel / uint8(8)
	buf := make([]byte, bytesPerPixel)

	n, err = io.ReadFull(reader, buf)
	if err != nil {
		return c, err
	} else if uint8(n) != bytesPerPixel {
		return c, errors.New("unable to read full pixel")
	}

	raw := binary.LittleEndian.Uint32(buf)
	if s.BigEndianFlag != 0 {
		raw = binary.BigEndian.Uint32(buf)
	}

	if s.TrueColorFlag != 0 {
		c.R = uint16((raw >> uint32(s.RedShift)) & uint32(s.RedMax))
		c.G = uint16((raw >> uint32(s.GreenShift)) & uint32(s.GreenMax))
		c.B = uint16((raw >> uint32(s.BlueShift)) & uint32(s.BlueMax))
		c.A = 65535
	} else {
		// TODO
		return c, errors.New("unable to decode untrue colors")
	}

	return c, nil
}

func (s *Server) decodeRawEncoding(r io.Reader, rect *image.RGBA64) error {
	for y := rect.Rect.Min.Y; y < rect.Rect.Max.Y; y++ {
		for x := rect.Rect.Min.X; x < rect.Rect.Max.X; x++ {
			pixel, err := s.readPixel(r)
			if err != nil {
				log.Println("error reading pixel %d, %d", x, y)
				return err
			}
			rect.Set(x, y, pixel)
		}
	}

	return nil
}
