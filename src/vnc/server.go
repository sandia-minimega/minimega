// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package vnc

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
)

// See RFC 6143 Section 7.6
const (
	TypeFramebufferUpdate uint8 = iota
	TypeSetColorMapEntries
	TypeBell
	TypeServerCutText
)

var serverMessages = map[uint8]func() interface{}{
	TypeFramebufferUpdate:  func() interface{} { return new(_FramebufferUpdate) },
	TypeSetColorMapEntries: func() interface{} { return new(_SetColorMapEntries) },
	TypeBell:               func() interface{} { return new(Bell) },
	TypeServerCutText:      func() interface{} { return new(_ServerCutText) },
}

// See RFC 6143 Section 7.3.2
type Server struct {
	Width  uint16
	Height uint16
	PixelFormat
	NameLength uint32
	Name       []uint8
}

// See RFC 6143 Section 7.6.1
type Rectangle struct {
	XPosition    uint16
	YPosition    uint16
	Width        uint16
	Height       uint16
	EncodingType int32
}

type _FramebufferUpdate struct {
	_                  [1]byte // Padding
	NumberOfRectangles uint16
}

// See RFC 6143 Section 7.6.1
type FramebufferUpdate struct {
	_FramebufferUpdate
	Updates []*image.RGBA64
}

type _SetColorMapEntries struct {
	_              [1]byte // Padding
	FirstColor     uint16
	NumberOfColors uint16
}

// See RFC 6143 Section 7.6.2
type Color struct {
	R, G, B uint16
}

// See RFC 6143 Section 7.6.2
type SetColorMapEntries struct {
	_SetColorMapEntries
	Colors []Color
}

// See RFC 6143 Section 7.6.3
type Bell struct {
}

// See RFC 6143 Section 7.6.4
type _ServerCutText struct {
	_      [3]byte // Padding
	Length uint32  // Length of Text
}

// See RFC 6143 Section 7.6.4
type ServerCutText struct {
	_ServerCutText
	Text []uint8
}

// ReadMessage reads the next server-to-client message
func (s *Server) ReadMessage(r io.Reader) (interface{}, error) {
	var msgType uint8
	if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
		return nil, fmt.Errorf("unable to read message type -- %s", err.Error())
	}

	if _, ok := serverMessages[msgType]; !ok {
		return nil, fmt.Errorf("unknown server-to-client message: %d", msgType)
	}

	// Copy the struct
	msg := serverMessages[msgType]()

	if err := binary.Read(r, binary.BigEndian, msg); err != nil {
		return nil, fmt.Errorf("unable to read message -- %s", err.Error())
	}

	switch msgType {
	case TypeFramebufferUpdate:
		newMsg := &FramebufferUpdate{_FramebufferUpdate: *msg.(*_FramebufferUpdate)}
		newMsg.Updates = make([]*image.RGBA64, newMsg.NumberOfRectangles)

		for i := uint16(0); i < newMsg.NumberOfRectangles; i++ {
			var rect Rectangle
			if err := binary.Read(r, binary.BigEndian, &rect); err != nil {
				return nil, fmt.Errorf("unable to read rectangle -- %s", err.Error())
			}

			update := image.NewRGBA64(image.Rect(
				int(rect.XPosition),
				int(rect.YPosition),
				int(rect.XPosition+rect.Width),
				int(rect.YPosition+rect.Height),
			))

			if err := s.readPixelData(r, rect.EncodingType, update); err != nil {
				return nil, fmt.Errorf("unable to read pixel data %d -- %s", i, err.Error())
			}
			newMsg.Updates[i] = update
		}

		msg = newMsg
	case TypeSetColorMapEntries:
		newMsg := &SetColorMapEntries{_SetColorMapEntries: *msg.(*_SetColorMapEntries)}
		newMsg.Colors = make([]Color, newMsg.NumberOfColors)

		if err := binary.Read(r, binary.BigEndian, &newMsg.Colors); err != nil {
			return nil, fmt.Errorf("unable to read colors -- %s", err.Error())
		}

		msg = newMsg
	case TypeServerCutText:
		newMsg := &ServerCutText{_ServerCutText: *msg.(*_ServerCutText)}
		newMsg.Text = make([]uint8, newMsg.Length)

		if err := binary.Read(r, binary.BigEndian, &newMsg.Text); err != nil {
			return nil, fmt.Errorf("unable to read text -- %s", err.Error())
		}

		msg = newMsg
	}

	return msg, nil
}

func (s *Server) readPixelData(r io.Reader, encType int32, rect *image.RGBA64) error {
	switch encType {
	case RawEncoding:
		return s.decodeRawEncoding(r, rect)
	case DesktopSizePseudoEncoding:
		// No pixel data to read
		return nil
	}

	return fmt.Errorf("unaccepted encoding: %d", encType)
}
