// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWriteRead(t *testing.T) {
	want := []Event{
		&SetPixelFormat{
			PixelFormat: PixelFormat{
				BitsPerPixel:  1,
				Depth:         2,
				BigEndianFlag: 3,
				TrueColorFlag: 4,
				RedMax:        5,
				GreenMax:      6,
				BlueMax:       7,
				RedShift:      8,
				GreenShift:    9,
				BlueShift:     10,
			},
		},
		&SetEncodings{
			Encodings: []int32{0, 1, 2, 3, 4},
		},
		&FramebufferUpdateRequest{
			Incremental: 1,
			X:           2,
			Y:           3,
			Width:       4,
			Height:      5,
		},
		&KeyEvent{
			DownFlag: 1,
			Key:      2,
		},
		&PointerEvent{
			ButtonMask: 1,
			XPosition:  2,
			YPosition:  3,
		},
		&ClientCutText{
			Text: []byte("hello world"),
		},
	}

	for _, want := range want {
		var buf bytes.Buffer

		if err := want.Write(&buf); err != nil {
			t.Fatalf("write failed -- %s", err)
		}

		got, err := ReadClientMessage(&buf)
		if err != nil {
			t.Fatalf("read failed -- %s", err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("struct aren't equal -- got: %v, want: %v", got, want)
		}
	}
}

func TestReadBuffer(t *testing.T) {
	b := []byte{0, 0, 0, 0, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0,
		0, 0, 2, 0, 0, 11, 0, 0, 0, 1, 0, 0, 0, 7, 255, 255, 254, 252, 0, 0,
		0, 5, 0, 0, 0, 2, 0, 0, 0, 0, 255, 255, 255, 33, 255, 255, 255, 17,
		255, 255, 255, 230, 255, 255, 255, 9, 255, 255, 255, 32, 3, 0, 0, 0, 0,
		0, 3, 32, 2, 88, 2, 0, 0, 2, 0, 0, 0, 0, 255, 255, 255, 33}
	buf := bytes.NewBuffer(b)

	for buf.Len() > 0 {
		msg, err := ReadClientMessage(buf)
		if err != nil {
			t.Fatalf("read message failed -- %v", err)
		}
		t.Logf("%#v\n", msg)
	}
}
