// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

func cutTextHeader(length int32) []byte {
	b := []byte{TypeClientCutText, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[4:], uint32(length))
	return b
}

// TestReadClientMessageLimits is the regression test for B1: a length outside
// the limits gives an error before the reader allocates or reads the body.
func TestReadClientMessageLimits(t *testing.T) {
	fence := []byte{TypeClientFence, 0, 0, 0, 0, 0, 0, 0, maxFencePayload + 1}

	cases := []struct {
		name string
		in   []byte
	}{
		{"cut text MinInt32", cutTextHeader(math.MinInt32)},
		{"cut text above 1 MiB", cutTextHeader(maxCutText + 1)},
		{"cut text below -1 MiB", cutTextHeader(-maxCutText - 1)},
		{"fence payload above 64", fence},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := &readSizes{r: bytes.NewReader(c.in)}
			mr := newMessageReader(src)

			_, _, err := mr.next()

			if err == nil {
				t.Fatal("got no error")
			}
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("reader tried to read the body: %v", err)
			}
			if !errors.Is(err, ErrMessageLimit) {
				t.Fatalf("got %v, want ErrMessageLimit", err)
			}
			if n := cap(mr.buf); n > 64<<10 {
				t.Fatalf("reader buffer has a capacity of %d bytes", n)
			}
			if src.max > 64<<10 {
				t.Fatalf("reader asked for %d bytes in one read", src.max)
			}
		})
	}
}

// readSizes records the largest buffer given to Read.
type readSizes struct {
	r   io.Reader
	max int
}

func (r *readSizes) Read(p []byte) (int, error) {
	r.max = max(r.max, len(p))
	return r.r.Read(p)
}

func TestReadClientMessageLimitBoundaries(t *testing.T) {
	for _, length := range []int32{maxCutText, -maxCutText} {
		n := int(length)
		if n < 0 {
			n = -n
		}
		in := append(cutTextHeader(length), make([]byte, n)...)

		msg, err := ReadClientMessage(bytes.NewReader(in))
		if err != nil {
			t.Fatalf("length %d: %v", length, err)
		}

		var w bytes.Buffer
		if err := msg.Write(&w); err != nil {
			t.Fatalf("length %d: write: %v", length, err)
		}
		if !bytes.Equal(w.Bytes(), in) {
			t.Fatalf("length %d: re-encoded bytes differ", length)
		}
	}

	in := []byte{TypeClientFence, 0, 0, 0, 0, 0, 0, 1, maxFencePayload}
	in = append(in, bytes.Repeat([]byte{'x'}, maxFencePayload)...)
	if _, err := ReadClientMessage(bytes.NewReader(in)); err != nil {
		t.Fatalf("fence with 64-byte payload: %v", err)
	}
}

func TestReadClientMessageShortInput(t *testing.T) {
	full := []byte{TypeKeyEvent, 1, 0, 0, 0, 0, 0, 0x41}
	for i := 1; i < len(full); i++ {
		_, err := ReadClientMessage(bytes.NewReader(full[:i]))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("%d of %d bytes: got %v, want io.ErrUnexpectedEOF", i, len(full), err)
		}
	}

	if _, err := ReadClientMessage(bytes.NewReader(nil)); err != io.EOF {
		t.Fatalf("no input: got %v, want io.EOF", err)
	}
}

func TestReadClientMessageUnknown(t *testing.T) {
	cases := []struct {
		in            []byte
		typ           uint8
		subtype       uint8
		subtypeIsRead bool
	}{
		{[]byte{1, 2, 3}, 1, 0, false},
		{[]byte{7}, 7, 0, false},
		{[]byte{TypeQEMU, 9, 0, 0}, TypeQEMU, 9, true},
		{[]byte{TypeQEMU, QEMUAudio, 0, 7}, TypeQEMU, QEMUAudio, true},
	}

	for _, c := range cases {
		_, err := ReadClientMessage(bytes.NewReader(c.in))

		var unknown *UnknownMessageError
		if !errors.As(err, &unknown) {
			t.Fatalf("% x: got %v, want UnknownMessageError", c.in, err)
		}
		if unknown.Type != c.typ || unknown.Subtype != c.subtype || unknown.HasSubtype != c.subtypeIsRead {
			t.Fatalf("% x: got %+v", c.in, unknown)
		}
		if !bytes.HasPrefix(c.in, unknown.Raw) || len(unknown.Raw) == 0 {
			t.Fatalf("% x: raw prefix % x is wrong", c.in, unknown.Raw)
		}
	}
}

// FuzzReadClientMessage is T22: the reader does not panic, and it never keeps
// a buffer larger than the largest valid message.
func FuzzReadClientMessage(f *testing.F) {
	for _, m := range sampleMessages() {
		var b bytes.Buffer
		if err := m.Write(&b); err != nil {
			f.Fatal(err)
		}
		f.Add(b.Bytes())
	}
	f.Add(cutTextHeader(math.MinInt32))
	f.Add(cutTextHeader(maxCutText + 1))
	f.Add([]byte{TypeClientFence, 0, 0, 0, 0, 0, 0, 0, 0xff})
	f.Add([]byte{TypeSetEncodings, 0, 0xff, 0xff})
	f.Add([]byte{TypeSetDesktopSize, 0, 0, 1, 0, 1, 0xff, 0})

	session, msgs := noVNCSession(f)
	f.Add(session)
	for _, m := range msgs {
		f.Add(m)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		r := newMessageReader(bytes.NewReader(in))
		for {
			raw, msg, err := r.next()
			if cap(r.buf) > maxMessageSize {
				t.Fatalf("buffer grew to %d bytes", cap(r.buf))
			}
			if err != nil {
				return
			}

			var w bytes.Buffer
			if err := msg.Write(&w); err != nil {
				t.Fatalf("write %T: %v", msg, err)
			}
			if w.Len() != len(raw) {
				t.Fatalf("%T: re-encoded %d bytes, read %d", msg, w.Len(), len(raw))
			}
		}
	})
}
