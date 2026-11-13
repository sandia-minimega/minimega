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

// ErrMessageLimit is returned when a length field of a client message is
// outside the limits of the reader.
var ErrMessageLimit = errors.New("client message exceeds limit")

// UnknownMessageError is returned when the reader does not know a client
// message type. Raw holds the bytes that the reader consumed.
type UnknownMessageError struct {
	Type       uint8
	Subtype    uint8
	HasSubtype bool
	Raw        []byte
}

func (e *UnknownMessageError) Error() string {
	if e.HasSubtype {
		return fmt.Sprintf("unknown client-to-server message: %d/%d", e.Type, e.Subtype)
	}

	return fmt.Sprintf("unknown client-to-server message: %d", e.Type)
}

// ReadClientMessage reads the next client-to-server message. It returns
// io.EOF only when r ends before the first byte of a message.
func ReadClientMessage(r io.Reader) (ClientMessage, error) {
	_, msg, err := newMessageReader(r).next()
	return msg, err
}

// messageReader frames client-to-server messages. It reads each message into
// a buffer that it reuses, and it checks each length field before it reads
// the data that the field describes.
type messageReader struct {
	r   io.Reader
	buf []byte
}

func newMessageReader(r io.Reader) *messageReader {
	return &messageReader{r: r, buf: make([]byte, 0, 64)}
}

// read appends n bytes from the stream to buf and returns them.
func (mr *messageReader) read(n int) ([]byte, error) {
	start := len(mr.buf)
	end := start + n

	if end > cap(mr.buf) {
		size := 2 * cap(mr.buf)
		if size < end {
			size = end
		}
		if size > maxMessageSize {
			size = maxMessageSize
		}

		buf := make([]byte, start, size)
		copy(buf, mr.buf)
		mr.buf = buf
	}

	mr.buf = mr.buf[:end]

	if _, err := io.ReadFull(mr.r, mr.buf[start:]); err != nil {
		mr.buf = mr.buf[:start]
		if err == io.EOF && start > 0 {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}

	return mr.buf[start:], nil
}

// next reads one message. The raw bytes are valid until the next call. The
// message does not share memory with them.
func (mr *messageReader) next() ([]byte, ClientMessage, error) {
	mr.buf = mr.buf[:0]

	b, err := mr.read(1)
	if err != nil {
		return nil, nil, err
	}
	msgType := b[0]

	msg, err := mr.readBody(msgType)
	if err != nil {
		return nil, nil, err
	}

	return mr.buf, msg, nil
}

func (mr *messageReader) unknown(msgType, subtype uint8, hasSubtype bool) error {
	return &UnknownMessageError{
		Type:       msgType,
		Subtype:    subtype,
		HasSubtype: hasSubtype,
		Raw:        append([]byte(nil), mr.buf...),
	}
}

func (mr *messageReader) readBody(msgType uint8) (ClientMessage, error) {
	be := binary.BigEndian

	switch msgType {
	case TypeSetPixelFormat:
		b, err := mr.read(19)
		if err != nil {
			return nil, err
		}

		return &SetPixelFormat{PixelFormat: PixelFormat{
			BitsPerPixel:  b[3],
			Depth:         b[4],
			BigEndianFlag: b[5],
			TrueColorFlag: b[6],
			RedMax:        be.Uint16(b[7:]),
			GreenMax:      be.Uint16(b[9:]),
			BlueMax:       be.Uint16(b[11:]),
			RedShift:      b[13],
			GreenShift:    b[14],
			BlueShift:     b[15],
		}}, nil
	case TypeSetEncodings:
		b, err := mr.read(3)
		if err != nil {
			return nil, err
		}
		n := be.Uint16(b[1:])

		b, err = mr.read(4 * int(n))
		if err != nil {
			return nil, err
		}

		m := &SetEncodings{Encodings: make([]int32, n)}
		m.NumberOfEncodings = n
		for i := range m.Encodings {
			m.Encodings[i] = int32(be.Uint32(b[4*i:]))
		}

		return m, nil
	case TypeFramebufferUpdateRequest:
		b, err := mr.read(9)
		if err != nil {
			return nil, err
		}

		return &FramebufferUpdateRequest{
			Incremental: b[0],
			X:           be.Uint16(b[1:]),
			Y:           be.Uint16(b[3:]),
			Width:       be.Uint16(b[5:]),
			Height:      be.Uint16(b[7:]),
		}, nil
	case TypeKeyEvent:
		b, err := mr.read(7)
		if err != nil {
			return nil, err
		}

		return &KeyEvent{DownFlag: b[0], Key: be.Uint32(b[3:])}, nil
	case TypePointerEvent:
		b, err := mr.read(5)
		if err != nil {
			return nil, err
		}

		mask, x, y := b[0], be.Uint16(b[1:]), be.Uint16(b[3:])
		if mask&0x80 == 0 {
			return &PointerEvent{ButtonMask: mask, XPosition: x, YPosition: y}, nil
		}

		b, err = mr.read(1)
		if err != nil {
			return nil, err
		}

		return &ExtendedPointerEvent{ButtonMask: mask, XPosition: x, YPosition: y, HighButtons: b[0]}, nil
	case TypeClientCutText:
		b, err := mr.read(7)
		if err != nil {
			return nil, err
		}

		length := int64(int32(be.Uint32(b[3:])))
		n := length
		if n < 0 {
			n = -n
		}
		if n > maxCutText {
			return nil, fmt.Errorf("%w: cut text length %d", ErrMessageLimit, length)
		}

		b, err = mr.read(int(n))
		if err != nil {
			return nil, err
		}

		if length < 0 {
			return &ExtendedClientCutText{Data: append([]byte{}, b...)}, nil
		}

		m := &ClientCutText{Text: append([]byte{}, b...)}
		m.Length = int32(length)

		return m, nil
	case TypeEnableContinuousUpdates:
		b, err := mr.read(9)
		if err != nil {
			return nil, err
		}

		return &EnableContinuousUpdates{
			Enable: b[0],
			X:      be.Uint16(b[1:]),
			Y:      be.Uint16(b[3:]),
			Width:  be.Uint16(b[5:]),
			Height: be.Uint16(b[7:]),
		}, nil
	case TypeClientFence:
		b, err := mr.read(8)
		if err != nil {
			return nil, err
		}

		flags, n := be.Uint32(b[3:]), b[7]
		if n > maxFencePayload {
			return nil, fmt.Errorf("%w: fence payload length %d", ErrMessageLimit, n)
		}

		b, err = mr.read(int(n))
		if err != nil {
			return nil, err
		}

		return &ClientFence{Flags: flags, Payload: append([]byte{}, b...)}, nil
	case TypeXVP:
		b, err := mr.read(3)
		if err != nil {
			return nil, err
		}

		return &XVPMessage{Version: b[1], Op: b[2]}, nil
	case TypeSetDesktopSize:
		b, err := mr.read(7)
		if err != nil {
			return nil, err
		}

		m := &SetDesktopSize{Width: be.Uint16(b[1:]), Height: be.Uint16(b[3:])}
		n := int(b[5])

		b, err = mr.read(16 * n)
		if err != nil {
			return nil, err
		}

		m.Screens = make([]Screen, n)
		for i := range m.Screens {
			s := b[16*i:]
			m.Screens[i] = Screen{
				ID:     be.Uint32(s),
				X:      be.Uint16(s[4:]),
				Y:      be.Uint16(s[6:]),
				Width:  be.Uint16(s[8:]),
				Height: be.Uint16(s[10:]),
				Flags:  be.Uint32(s[12:]),
			}
		}

		return m, nil
	case TypeQEMU:
		return mr.readQEMU()
	}

	return nil, mr.unknown(msgType, 0, false)
}

func (mr *messageReader) readQEMU() (ClientMessage, error) {
	be := binary.BigEndian

	b, err := mr.read(1)
	if err != nil {
		return nil, err
	}
	subtype := b[0]

	switch subtype {
	case QEMUExtendedKey:
		b, err := mr.read(10)
		if err != nil {
			return nil, err
		}

		return &QEMUExtendedKeyEvent{
			DownFlag: be.Uint16(b),
			Keysym:   be.Uint32(b[2:]),
			Keycode:  be.Uint32(b[6:]),
		}, nil
	case QEMUAudio:
		b, err := mr.read(2)
		if err != nil {
			return nil, err
		}

		m := &QEMUAudioMessage{Operation: be.Uint16(b)}
		switch m.Operation {
		case QEMUAudioEnable, QEMUAudioDisable:
			return m, nil
		case QEMUAudioSetFormat:
			b, err := mr.read(6)
			if err != nil {
				return nil, err
			}

			m.Format, m.Channels, m.Frequency = b[0], b[1], be.Uint32(b[2:])
			return m, nil
		}
	}

	return nil, mr.unknown(TypeQEMU, subtype, true)
}
