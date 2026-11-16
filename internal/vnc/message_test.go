// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type messageCase struct {
	name string
	msg  ClientMessage
	raw  string // hex, spaces are ignored
}

func messageCases() []messageCase {
	return []messageCase{
		{
			"SetPixelFormat",
			&SetPixelFormat{PixelFormat: PixelFormat{
				BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1,
				RedMax: 255, GreenMax: 255, BlueMax: 255,
				RedShift: 0, GreenShift: 8, BlueShift: 16,
			}},
			"00 000000 20 18 00 01 00ff 00ff 00ff 00 08 10 000000",
		},
		{
			"SetEncodings",
			&SetEncodings{Encodings: []int32{7, 0, -258, ExtendedClipboardPseudoEncoding}},
			"02 00 0004 00000007 00000000 fffffefe c0a1e5ce",
		},
		{
			"SetEncodings empty",
			&SetEncodings{Encodings: []int32{}},
			"02 00 0000",
		},
		{
			"FramebufferUpdateRequest",
			&FramebufferUpdateRequest{Incremental: 1, X: 1, Y: 2, Width: 1024, Height: 768},
			"03 01 0001 0002 0400 0300",
		},
		{
			"KeyEvent",
			&KeyEvent{DownFlag: 1, Key: 0xff0d},
			"04 01 0000 0000ff0d",
		},
		{
			"PointerEvent",
			&PointerEvent{ButtonMask: 0x7f, XPosition: 640, YPosition: 480},
			"05 7f 0280 01e0",
		},
		{
			"ExtendedPointerEvent",
			&ExtendedPointerEvent{ButtonMask: 0x81, XPosition: 10, YPosition: 20, HighButtons: 0x03},
			"05 81 000a 0014 03",
		},
		{
			"ClientCutText",
			&ClientCutText{Text: []byte("hi\xe9")},
			"06 000000 00000003 6869e9",
		},
		{
			"ClientCutText empty",
			&ClientCutText{Text: []byte{}},
			"06 000000 00000000",
		},
		{
			"ExtendedClientCutText",
			&ExtendedClientCutText{Data: []byte{0x08, 0, 0, 0x01}},
			"06 000000 fffffffc 08000001",
		},
		{
			"EnableContinuousUpdates",
			&EnableContinuousUpdates{Enable: 1, X: 0, Y: 0, Width: 800, Height: 600},
			"96 01 0000 0000 0320 0258",
		},
		{
			"ClientFence",
			&ClientFence{Flags: 0x80000003, Payload: []byte("ab")},
			"f8 000000 80000003 02 6162",
		},
		{
			"ClientFence empty",
			&ClientFence{Flags: 1, Payload: []byte{}},
			"f8 000000 00000001 00",
		},
		{
			"XVPMessage",
			&XVPMessage{Version: 1, Op: 4},
			"fa 00 01 04",
		},
		{
			"SetDesktopSize",
			&SetDesktopSize{Width: 1280, Height: 720, Screens: []Screen{
				{ID: 1, X: 0, Y: 0, Width: 1280, Height: 720, Flags: 0},
			}},
			"fb 00 0500 02d0 01 00 00000001 0000 0000 0500 02d0 00000000",
		},
		{
			"SetDesktopSize no screens",
			&SetDesktopSize{Width: 1, Height: 2, Screens: []Screen{}},
			"fb 00 0001 0002 00 00",
		},
		{
			"QEMUExtendedKeyEvent",
			&QEMUExtendedKeyEvent{DownFlag: 1, Keysym: 0x61, Keycode: 0x1e},
			"ff 00 0001 00000061 0000001e",
		},
		{
			"QEMUAudioMessage enable",
			&QEMUAudioMessage{Operation: QEMUAudioEnable},
			"ff 01 0000",
		},
		{
			"QEMUAudioMessage disable",
			&QEMUAudioMessage{Operation: QEMUAudioDisable},
			"ff 01 0001",
		},
		{
			"QEMUAudioMessage set format",
			&QEMUAudioMessage{Operation: QEMUAudioSetFormat, Format: 3, Channels: 2, Frequency: 44100},
			"ff 01 0002 03 02 0000ac44",
		},
	}
}

func sampleMessages() []ClientMessage {
	var msgs []ClientMessage
	for _, c := range messageCases() {
		msgs = append(msgs, c.msg)
	}
	return msgs
}

func mustHex(t testing.TB, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("bad hex %q: %v", s, err)
	}
	return b
}

// Each client message encodes to the bytes
// that the protocol defines, and decodes back to the same message.
func TestClientMessageRoundTrip(t *testing.T) {
	for _, c := range messageCases() {
		t.Run(c.name, func(t *testing.T) {
			want := mustHex(t, c.raw)

			var w bytes.Buffer
			if err := c.msg.Write(&w); err != nil {
				t.Fatalf("write: %v", err)
			}
			if !bytes.Equal(w.Bytes(), want) {
				t.Fatalf("encode:\n got % x\nwant % x", w.Bytes(), want)
			}

			// A trailing byte shows that the reader stops at the message end.
			in := bytes.NewReader(append(append([]byte{}, want...), 0xaa))
			r := newMessageReader(in)

			raw, got, err := r.next()
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(raw, want) {
				t.Fatalf("raw:\n got % x\nwant % x", raw, want)
			}
			if in.Len() != 1 {
				t.Fatalf("reader left %d bytes, want 1", in.Len())
			}
			if !reflect.DeepEqual(got, c.msg) {
				t.Fatalf("decode:\n got %#v\nwant %#v", got, c.msg)
			}

			w.Reset()
			if err := got.Write(&w); err != nil {
				t.Fatalf("write decoded: %v", err)
			}
			if !bytes.Equal(w.Bytes(), want) {
				t.Fatalf("re-encode:\n got % x\nwant % x", w.Bytes(), want)
			}
		})
	}
}

// writeCounter counts the Write calls. A Write after limit calls fails.
type writeCounter struct {
	writes int
	limit  int
}

var errWriteLimit = errors.New("write limit")

func (w *writeCounter) Write(b []byte) (int, error) {
	w.writes++
	if w.writes > w.limit {
		return 0, errWriteLimit
	}
	return len(b), nil
}

// Each client message is one Write, so that the messages of two writers
// on one connection do not mix. A write error is wrapped.
func TestClientMessageOneWrite(t *testing.T) {
	for _, c := range messageCases() {
		w := &writeCounter{limit: 1}
		if err := c.msg.Write(w); err != nil {
			t.Errorf("%v: write: %v", c.name, err)
		}
		if w.writes != 1 {
			t.Errorf("%v: %d Write calls, want 1", c.name, w.writes)
		}

		w = &writeCounter{}
		if err := c.msg.Write(w); !errors.Is(err, errWriteLimit) {
			t.Errorf("%v: write error = %v, want errWriteLimit", c.name, err)
		}
	}
}

func TestReaderReusesBuffer(t *testing.T) {
	want := sampleMessages()

	var in bytes.Buffer
	for _, m := range want {
		if err := m.Write(&in); err != nil {
			t.Fatal(err)
		}
	}

	r := newMessageReader(&in)
	var got []ClientMessage
	for {
		_, msg, err := r.next()
		if err != nil {
			break
		}
		got = append(got, msg)
	}

	// Decoded messages must not alias the reused buffer.
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("decoded messages changed after later reads")
	}
}

// An ExtendedClientCutText above maxCutText gives an error and writes
// nothing. One at the limit is written.
func TestExtendedClientCutTextLimit(t *testing.T) {
	w := &writeCounter{limit: 1}
	err := (&ExtendedClientCutText{Data: make([]byte, maxCutText+1)}).Write(w)
	if err == nil || !strings.Contains(err.Error(), "limit") {
		t.Errorf("write above the limit = %v, want a limit error", err)
	}
	if w.writes != 0 {
		t.Errorf("write above the limit made %d Write calls, want 0", w.writes)
	}

	if err := (&ExtendedClientCutText{Data: make([]byte, maxCutText)}).Write(&writeCounter{limit: 1}); err != nil {
		t.Errorf("write at the limit: %v", err)
	}
}
