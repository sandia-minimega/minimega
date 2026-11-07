// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// ReadMessage reads the next server-to-client message
func (c *Conn) ReadMessage() (interface{}, error) {
	var msgType uint8
	if err := binary.Read(c, binary.BigEndian, &msgType); err != nil {
		return nil, fmt.Errorf("unable to read message type: %v", err)
	}

	switch msgType {
	case TypeFramebufferUpdate:
		msg := FramebufferUpdate{}

		// Skip padding
		if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
			return nil, fmt.Errorf("unable to skip padding: %v", err)
		}

		// Decode up to rectangles
		if err := binary.Read(c, binary.BigEndian, &msg.NumRectangles); err != nil {
			return nil, fmt.Errorf("unable to decode num rectangles: %v", err)
		}

		log.Debugln("number of rectangles:", msg.NumRectangles)

		// Read all the rectangles
		for len(msg.Rectangles) < int(msg.NumRectangles) {
			rect, err := c.readRectangle()
			if err != nil {
				return nil, err
			}

			msg.Rectangles = append(msg.Rectangles, rect)
		}

		return &msg, nil
	case TypeSetColorMapEntries:
		msg := SetColorMapEntries{}

		// Skip padding
		if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
			return nil, fmt.Errorf("unable to skip padding: %v", err)
		}

		// Decode first color
		if err := binary.Read(c, binary.BigEndian, &msg.FirstColor); err != nil {
			return nil, fmt.Errorf("unable to decode first color: %v", err)
		}

		// Decode num colors
		if err := binary.Read(c, binary.BigEndian, &msg.NumColors); err != nil {
			return nil, fmt.Errorf("unable to decode num colors: %v", err)
		}

		msg.Colors = make([]Color, msg.NumColors)

		// Decode the colors
		if err := binary.Read(c, binary.BigEndian, &msg.Colors); err != nil {
			return nil, fmt.Errorf("unable to read colors: %v", err)
		}

		return &msg, nil
	case TypeServerCutText:
		msg := ServerCutText{}

		// Skip padding
		if _, err := io.ReadFull(c, make([]byte, 3)); err != nil {
			return nil, fmt.Errorf("unable to skip padding: %v", err)
		}

		// Decode cut text length
		if err := binary.Read(c, binary.BigEndian, &msg.Length); err != nil {
			return nil, fmt.Errorf("unable to decode cut text length: %v", err)
		}
		if msg.Length > maxCutText {
			return nil, fmt.Errorf("server cut text has %d bytes, limit is %d", msg.Length, maxCutText)
		}

		msg.Text = make([]uint8, msg.Length)

		// Decode the text
		if err := binary.Read(c, binary.BigEndian, &msg.Text); err != nil {
			return nil, fmt.Errorf("unable to decode cut text: %v", err)
		}

		return &msg, nil
	case TypeBell:
		return &Bell{}, nil
	}

	return nil, fmt.Errorf("unhandled message type: %v", msgType)
}

func (c *Conn) readRectangle() (*Rectangle, error) {
	r := &Rectangle{}

	if err := binary.Read(c, binary.BigEndian, &r.X); err != nil {
		return nil, fmt.Errorf("unable to decode rect X pos: %v", err)
	} else if err := binary.Read(c, binary.BigEndian, &r.Y); err != nil {
		return nil, fmt.Errorf("unable to decode rect Y pos: %v", err)
	} else if err := binary.Read(c, binary.BigEndian, &r.Width); err != nil {
		return nil, fmt.Errorf("unable to decode rect width: %v", err)
	} else if err := binary.Read(c, binary.BigEndian, &r.Height); err != nil {
		return nil, fmt.Errorf("unable to decode rect height: %v", err)
	} else if err := binary.Read(c, binary.BigEndian, &r.EncodingType); err != nil {
		return nil, fmt.Errorf("unable to decode rect encoding type: %v", err)
	}

	log.Debug("rectangle: %d x %d at (%d, %d)", r.Width, r.Height, r.X, r.Y)

	var err error

	// Only Raw rectangles get pixel data. The others leave r.RGBA nil.
	switch r.EncodingType {
	case RawEncoding:
		r.RGBA = image.NewRGBA(image.Rect(int(r.X), int(r.Y), int(r.X)+int(r.Width), int(r.Y)+int(r.Height)))
		err = c.decodeRawEncoding(r)
	case DesktopSizePseudoEncoding:
		err = c.decodeDesktopSizeEncoding(r)
	case CursorPseudoEncoding:
		err = c.skipCursorEncoding(r)
	default:
		err = fmt.Errorf("unaccepted encoding: %d", r.EncodingType)
	}

	if err != nil {
		return nil, err
	}

	return r, nil
}

func (c *Conn) decodeRawEncoding(r *Rectangle) error {
	for y := r.Rect.Min.Y; y < r.Rect.Max.Y; y++ {
		for x := r.Rect.Min.X; x < r.Rect.Max.X; x++ {
			pixel, err := c.readPixel()
			if err != nil {
				return fmt.Errorf("error reading pixel (%v, %v): %v", x, y, err)
			}
			r.RGBA.SetRGBA(x, y, pixel)
		}
	}

	return nil
}

func (c *Conn) decodeDesktopSizeEncoding(r *Rectangle) error {
	oldW, oldH := c.size()
	log.Info("new desktop size: %v x %v -> %v x %v", oldW, oldH, r.Width, r.Height)
	c.setSize(r.Width, r.Height)

	return nil
}

// skipCursorEncoding reads and drops the pixels and the bitmask of a Cursor
// pseudo-encoding rectangle.
func (c *Conn) skipCursorEncoding(r *Rectangle) error {
	w, h := int64(r.Width), int64(r.Height)
	n := w*h*int64(c.pf.BitsPerPixel/8) + (w+7)/8*h

	if _, err := io.CopyN(io.Discard, c, n); err != nil {
		return fmt.Errorf("unable to skip cursor: %v", err)
	}

	return nil
}

func (c *Conn) readPixel() (color.RGBA, error) {
	var rgb color.RGBA

	pf := c.pf
	if pf.TrueColorFlag == 0 {
		// TODO
		return rgb, errors.New("unable to decode untrue colors")
	}

	buf := make([]byte, pf.BitsPerPixel/8)
	if _, err := io.ReadFull(c, buf); err != nil {
		return rgb, err
	}

	var raw uint32
	switch len(buf) {
	case 1:
		raw = uint32(buf[0])
	case 2:
		if pf.BigEndianFlag != 0 {
			raw = uint32(binary.BigEndian.Uint16(buf))
		} else {
			raw = uint32(binary.LittleEndian.Uint16(buf))
		}
	case 4:
		if pf.BigEndianFlag != 0 {
			raw = binary.BigEndian.Uint32(buf)
		} else {
			raw = binary.LittleEndian.Uint32(buf)
		}
	default:
		return rgb, fmt.Errorf("unable to decode %d bits per pixel", pf.BitsPerPixel)
	}

	rgb.R = uint8((raw >> uint32(pf.RedShift)) & uint32(pf.RedMax))
	rgb.G = uint8((raw >> uint32(pf.GreenShift)) & uint32(pf.GreenMax))
	rgb.B = uint8((raw >> uint32(pf.BlueShift)) & uint32(pf.BlueMax))
	rgb.A = 255

	return rgb, nil
}
