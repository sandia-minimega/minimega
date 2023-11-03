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

var clientMessages = map[uint8]func() interface{}{
	TypeSetPixelFormat:           func() interface{} { return new(SetPixelFormat) },
	TypeSetEncodings:             func() interface{} { return new(_SetEncodings) },
	TypeFramebufferUpdateRequest: func() interface{} { return new(FramebufferUpdateRequest) },
	TypeKeyEvent:                 func() interface{} { return new(KeyEvent) },
	TypePointerEvent:             func() interface{} { return new(PointerEvent) },
	TypeClientCutText:            func() interface{} { return new(_ClientCutText) },
}

// ReadClientMessage reads the next client-to-server message
func ReadClientMessage(r io.Reader) (interface{}, error) {
	var msgType uint8
	if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
		return nil, err
	}

	if _, ok := clientMessages[msgType]; !ok {
		return nil, fmt.Errorf("unknown client-to-server message: %d", msgType)
	}

	// Copy the struct
	msg := clientMessages[msgType]()

	if err := binary.Read(r, binary.BigEndian, msg); err != nil {
		return nil, err
	}

	var err error

	// Do extra processing on messages that have variable length fields
	switch msgType {
	case TypeSetEncodings:
		msg2 := &SetEncodings{_SetEncodings: *msg.(*_SetEncodings)}
		msg2.Encodings = make([]int32, msg2.NumberOfEncodings)

		err = binary.Read(r, binary.BigEndian, &msg2.Encodings)
		msg = msg2
	case TypeClientCutText:
		msg2 := &ClientCutText{_ClientCutText: *msg.(*_ClientCutText)}
		if msg2.Length < 0 {
			msg2.Length = -msg2.Length
		}
		msg2.Text = make([]uint8, msg2.Length)

		err = binary.Read(r, binary.BigEndian, &msg2.Text)
		msg = msg2
	}

	if err != nil {
		return nil, err
	}

	return msg, nil
}

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
		if _, err := c.Read(make([]byte, 1)); err != nil {
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
		if _, err := c.Read(make([]byte, 1)); err != nil {
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

		// Decode cut text length
		if err := binary.Read(c, binary.BigEndian, &msg.Length); err != nil {
			return nil, fmt.Errorf("unable to decode cut text length: %v", err)
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

	r.RGBA = image.NewRGBA(image.Rectangle{
		image.Point{int(r.X), int(r.Y)},
		image.Point{int(r.X) + int(r.Width), int(r.Y) + int(r.Height)}})

	var err error

	switch r.EncodingType {
	case RawEncoding:
		err = c.decodeRawEncoding(r)
	case TightEncoding:
		// TODO
		//err = c.decodeTightEncoding(reader, r)
	case DesktopSizePseudoEncoding:
		err = c.decodeDesktopSizeEncoding(r)
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
			r.RGBA.Set(x, y, pixel)
		}
	}

	return nil
}

func (c *Conn) decodeDesktopSizeEncoding(r *Rectangle) error {
	width, height := uint16(r.Rect.Dx()), uint16(r.Rect.Dy())
	log.Info("new desktop size: %v x %v -> %v x %v", c.s.Width, c.s.Height, width, height)
	c.s.Width, c.s.Height = width, height

	return nil
}

func (c *Conn) readPixel() (color.RGBA, error) {
	var rgb color.RGBA

	bytesPerPixel := c.s.BitsPerPixel / 8
	buf := make([]byte, bytesPerPixel)

	n, err := io.ReadFull(c, buf)
	if err != nil {
		return rgb, err
	} else if uint8(n) != bytesPerPixel {
		return rgb, errors.New("unable to read full pixel")
	}

	raw := binary.LittleEndian.Uint32(buf)
	if c.s.BigEndianFlag != 0 {
		raw = binary.BigEndian.Uint32(buf)
	}

	if c.s.TrueColorFlag != 0 {
		rgb.R = uint8((raw >> uint32(c.s.RedShift)) & uint32(c.s.RedMax))
		rgb.G = uint8((raw >> uint32(c.s.GreenShift)) & uint32(c.s.GreenMax))
		rgb.B = uint8((raw >> uint32(c.s.BlueShift)) & uint32(c.s.BlueMax))
		rgb.A = 255
	} else {
		// TODO
		return rgb, errors.New("unable to decode untrue colors")
	}

	return rgb, nil
}
