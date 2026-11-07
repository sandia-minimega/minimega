// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Client-to-server message types from RFC 6143 extensions. See the RFB
// community wiki (rfbproto) for each one.
const (
	TypeEnableContinuousUpdates uint8 = 150
	TypeClientFence             uint8 = 248
	TypeXVP                     uint8 = 250
	TypeSetDesktopSize          uint8 = 251
	TypeQEMU                    uint8 = 255
)

// Subtypes of TypeQEMU.
const (
	QEMUExtendedKey uint8 = 0
	QEMUAudio       uint8 = 1
)

// Operations of a QEMU audio message.
const (
	QEMUAudioEnable    uint16 = 0
	QEMUAudioDisable   uint16 = 1
	QEMUAudioSetFormat uint16 = 2
)

// ExtendedClipboardPseudoEncoding is the encoding 0xc0a1e5ce as a signed
// value.
const ExtendedClipboardPseudoEncoding int32 = -0x3f5e1a32

const (
	// maxCutText is the largest cut text length, the same limit as QEMU 6.2.
	maxCutText = 1 << 20

	// maxFencePayload is the largest ClientFence payload that rfbproto allows.
	maxFencePayload = 64

	// maxMessageSize is the size of the largest client message that the
	// reader accepts: a ClientCutText with maxCutText bytes.
	maxMessageSize = 8 + maxCutText
)

// ClientMessage is a client-to-server message that the shim can frame.
type ClientMessage interface {
	Write(w io.Writer) error
}

// ExtendedPointerEvent is the 7-byte PointerEvent form. A client sends it
// only after the server sends the ExtendedMouseButtons pseudo-encoding (-316).
type ExtendedPointerEvent struct {
	ButtonMask  uint8 // mask bits 0-6, with the marker bit 0x80 set
	XPosition   uint16
	YPosition   uint16
	HighButtons uint8 // mask bits 7 and 8 (Back and Forward)
}

// Mask returns the 9-bit button mask.
func (m *ExtendedPointerEvent) Mask() uint16 {
	return uint16(m.ButtonMask&0x7f) | uint16(m.HighButtons)<<7
}

func (m *ExtendedPointerEvent) Write(w io.Writer) error {
	e := *m
	e.ButtonMask |= 0x80

	return writeMessage(w, TypePointerEvent, &e)
}

// ExtendedClientCutText is a ClientCutText with a negative length, which the
// extended clipboard pseudo-encoding uses. Data holds the flags and the
// payload. An empty Data encodes as a plain, empty ClientCutText.
type ExtendedClientCutText struct {
	Data []byte
}

func (m *ExtendedClientCutText) Write(w io.Writer) error {
	if len(m.Data) > maxCutText {
		return fmt.Errorf("extended cut text has %d bytes, limit is %d", len(m.Data), maxCutText)
	}

	hdr := _ClientCutText{Length: -int32(len(m.Data))}
	if err := writeMessage(w, TypeClientCutText, &hdr); err != nil {
		return err
	}

	return writeBytes(w, m.Data)
}

// EnableContinuousUpdates is client message 150.
type EnableContinuousUpdates struct {
	Enable uint8
	X      uint16
	Y      uint16
	Width  uint16
	Height uint16
}

func (m *EnableContinuousUpdates) Write(w io.Writer) error {
	return writeMessage(w, TypeEnableContinuousUpdates, m)
}

type _ClientFence struct {
	_      [3]byte // Padding
	Flags  uint32
	Length uint8 // Length of Payload
}

// ClientFence is client message 248.
type ClientFence struct {
	Flags   uint32
	Payload []byte
}

func (m *ClientFence) Write(w io.Writer) error {
	if len(m.Payload) > maxFencePayload {
		return fmt.Errorf("fence payload has %d bytes, limit is %d", len(m.Payload), maxFencePayload)
	}

	hdr := _ClientFence{Flags: m.Flags, Length: uint8(len(m.Payload))}
	if err := writeMessage(w, TypeClientFence, &hdr); err != nil {
		return err
	}

	return writeBytes(w, m.Payload)
}

// XVPMessage is client message 250.
type XVPMessage struct {
	_       [1]byte // Padding
	Version uint8
	Op      uint8
}

func (m *XVPMessage) Write(w io.Writer) error {
	return writeMessage(w, TypeXVP, m)
}

// Screen is one screen of a SetDesktopSize message.
type Screen struct {
	ID     uint32
	X      uint16
	Y      uint16
	Width  uint16
	Height uint16
	Flags  uint32
}

type _SetDesktopSize struct {
	_          [1]byte // Padding
	Width      uint16
	Height     uint16
	NumScreens uint8
	_          [1]byte // Padding
}

// SetDesktopSize is client message 251.
type SetDesktopSize struct {
	Width   uint16
	Height  uint16
	Screens []Screen
}

func (m *SetDesktopSize) Write(w io.Writer) error {
	if len(m.Screens) > 255 {
		return fmt.Errorf("desktop size has %d screens, limit is 255", len(m.Screens))
	}

	hdr := _SetDesktopSize{Width: m.Width, Height: m.Height, NumScreens: uint8(len(m.Screens))}
	if err := writeMessage(w, TypeSetDesktopSize, &hdr); err != nil {
		return err
	}

	if err := binary.Write(w, binary.BigEndian, m.Screens); err != nil {
		return fmt.Errorf("unable to write screens -- %v", err)
	}

	return nil
}

// QEMUExtendedKeyEvent is QEMU client message 255/0. It carries both the
// keysym and the XT scancode of a key.
type QEMUExtendedKeyEvent struct {
	DownFlag uint16
	Keysym   uint32
	Keycode  uint32
}

// KeyEvent returns the plain KeyEvent with the same keysym and down flag.
func (m *QEMUExtendedKeyEvent) KeyEvent() *KeyEvent {
	e := &KeyEvent{Key: m.Keysym}
	if m.DownFlag != 0 {
		e.DownFlag = 1
	}

	return e
}

func (m *QEMUExtendedKeyEvent) Write(w io.Writer) error {
	body := struct {
		Subtype uint8
		QEMUExtendedKeyEvent
	}{QEMUExtendedKey, *m}

	return writeMessage(w, TypeQEMU, &body)
}

// QEMUAudioMessage is QEMU client message 255/1. Format, Channels, and
// Frequency are used only by the QEMUAudioSetFormat operation.
type QEMUAudioMessage struct {
	Operation uint16
	Format    uint8
	Channels  uint8
	Frequency uint32
}

func (m *QEMUAudioMessage) Write(w io.Writer) error {
	switch m.Operation {
	case QEMUAudioEnable, QEMUAudioDisable:
		body := struct {
			Subtype   uint8
			Operation uint16
		}{QEMUAudio, m.Operation}

		return writeMessage(w, TypeQEMU, &body)
	case QEMUAudioSetFormat:
		body := struct {
			Subtype uint8
			QEMUAudioMessage
		}{QEMUAudio, *m}

		return writeMessage(w, TypeQEMU, &body)
	}

	return errors.New("unknown QEMU audio operation")
}

func writeBytes(w io.Writer, b []byte) error {
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("unable to write message data -- %v", err)
	}

	return nil
}
