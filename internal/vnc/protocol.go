// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
)

// Client to server messages. See RFC 6143 Section 7.5
const (
	TypeSetPixelFormat uint8 = iota
	_                        // Not used
	TypeSetEncodings
	TypeFramebufferUpdateRequest
	TypeKeyEvent
	TypePointerEvent
	TypeClientCutText
)

// Server to client messages. See RFC 6143 Section 7.6
const (
	TypeFramebufferUpdate uint8 = iota
	TypeSetColorMapEntries
	TypeBell
	TypeServerCutText
)

const (
	RawEncoding               = 0
	TightEncoding             = 7
	DesktopSizePseudoEncoding = -223
	CursorPseudoEncoding      = -239
)

// See RFC 6143 Section 7.4
type PixelFormat struct {
	BitsPerPixel, Depth, BigEndianFlag, TrueColorFlag uint8
	RedMax, GreenMax, BlueMax                         uint16
	RedShift, GreenShift, BlueShift                   uint8
	_                                                 [3]byte // Padding
}

// See RFC 6143 Section 7.5.1
type SetPixelFormat struct {
	_ [3]byte // Padding
	PixelFormat
}

type _SetEncodings struct {
	_                 [1]byte // Padding
	NumberOfEncodings uint16  // Length of Encodings
}

// See RFC 6143 Section 7.5.2
type SetEncodings struct {
	_SetEncodings
	Encodings []int32
}

// See RFC 6143 Section 7.5.3
type FramebufferUpdateRequest struct {
	Incremental uint8
	X           uint16
	Y           uint16
	Width       uint16
	Height      uint16
}

// See RFC 6143 Section 7.5.4
type KeyEvent struct {
	DownFlag uint8
	_        [2]byte // Padding
	Key      uint32
}

// See RFC 6143 Section 7.5.5
type PointerEvent struct {
	ButtonMask uint8
	XPosition  uint16
	YPosition  uint16
}

type _ClientCutText struct {
	_      [3]byte // Padding
	Length int32   // Length of Text. Signed for extended pseudo-encoding
}

// See RFC 6143 Section 7.5.6
type ClientCutText struct {
	_ClientCutText
	Text []uint8
}

// See RFC 6143 Section 7.6.1
type Rectangle struct {
	X            uint16
	Y            uint16
	Width        uint16
	Height       uint16
	EncodingType int32

	// rgba is the pixel data for this Rectangle
	*image.RGBA
}

// See RFC 6143 Section 7.6.1
type FramebufferUpdate struct {
	_             [1]byte // Padding
	NumRectangles uint16
	Rectangles    []*Rectangle
}

// See RFC 6143 Section 7.6.2
type Color struct {
	R, G, B uint16
}

// See RFC 6143 Section 7.6.2
type SetColorMapEntries struct {
	_          [1]byte // Padding
	FirstColor uint16
	NumColors  uint16
	Colors     []Color
}

// See RFC 6143 Section 7.6.3
type Bell struct {
}

// See RFC 6143 Section 7.6.4
type ServerCutText struct {
	_      [3]byte // Padding
	Length uint32  // Length of Text
	Text   []uint8
}

func (m *SetPixelFormat) Write(w io.Writer) error {
	return writeMessage(w, TypeSetPixelFormat, m)
}

func (m *SetEncodings) Write(w io.Writer) error {
	// Ensure length is set correctly
	m.NumberOfEncodings = uint16(len(m.Encodings))

	if err := writeMessage(w, TypeSetEncodings, m._SetEncodings); err != nil {
		return err
	}

	// Write variable length encodings
	if err := binary.Write(w, binary.BigEndian, &m.Encodings); err != nil {
		return fmt.Errorf("unable to write encodings -- %v", err)
	}

	return nil
}

func (m *FramebufferUpdateRequest) Write(w io.Writer) error {
	return writeMessage(w, TypeFramebufferUpdateRequest, m)
}

func (m *KeyEvent) String() string {
	key, err := xKeysymToString(m.Key)
	if err != nil {
		key = fmt.Sprintf("%U", m.Key)
	}

	return fmt.Sprintf(keyEventFmt, m.DownFlag != 0, key)
}

func (m *KeyEvent) Write(w io.Writer) error {
	return writeMessage(w, TypeKeyEvent, m)
}

func (m *PointerEvent) String() string {
	return fmt.Sprintf(pointerEventFmt, m.ButtonMask, m.XPosition, m.YPosition)
}

func (m *PointerEvent) Write(w io.Writer) error {
	return writeMessage(w, TypePointerEvent, m)
}

func (m *ClientCutText) Write(w io.Writer) error {
	// Ensure length is set correctly
	m.Length = int32(len(m.Text))

	if err := writeMessage(w, TypeClientCutText, m._ClientCutText); err != nil {
		return err
	}

	// Write variable length encodings
	if err := binary.Write(w, binary.BigEndian, &m.Text); err != nil {
		return fmt.Errorf("unable to write text -- %s", err.Error())
	}

	return nil
}
