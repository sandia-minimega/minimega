package vnc

import (
	"bytes"
	"reflect"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	want := []Writable{
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
			XPosition:   2,
			YPosition:   3,
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
