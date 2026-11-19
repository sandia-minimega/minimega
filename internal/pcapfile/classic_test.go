// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package pcapfile

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcapgo"
)

// chunks records each Write call.
type chunks struct {
	bytes.Buffer
	writes [][]byte
}

func (c *chunks) Write(p []byte) (int, error) {
	c.writes = append(c.writes, append([]byte(nil), p...))
	return c.Buffer.Write(p)
}

func TestClassicRoundTrip(t *testing.T) {
	var out chunks

	w, err := NewClassic(&out, 1600)
	if err != nil {
		t.Fatal(err)
	}

	if out.Len() != 24 {
		t.Fatalf("file header not written at once: %d bytes", out.Len())
	}

	base := time.Unix(1762351353, 123456789)
	var frames [][]byte

	for i := 0; i < 3000; i++ {
		data := bytes.Repeat([]byte{byte(i)}, 60+i%1000)
		frames = append(frames, data)

		rec := AppendRecord(nil, base.Add(time.Duration(i)), len(data)+4, data)
		if err := w.WriteRecord(0, rec, RecordOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if got, want := w.Size(), int64(out.Len()); got != want {
		t.Errorf("Size() = %d, file has %d bytes", got, want)
	}

	// every write after the header ends on a record boundary
	for i, chunk := range out.writes[1:] {
		for len(chunk) > 0 {
			h, err := ParseRecordHeader(chunk)
			if err != nil || len(chunk) < RecordHeaderLen+int(h.InclLen) {
				t.Fatalf("write %d ends inside a record", i+1)
			}
			chunk = chunk[RecordHeaderLen+int(h.InclLen):]
		}
	}

	r, err := pcapgo.NewReader(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	if r.Resolution() != gopacket.TimestampResolutionNanosecond {
		t.Errorf("resolution %v, want nanoseconds", r.Resolution())
	}

	for i := 0; ; i++ {
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			if i != len(frames) {
				t.Fatalf("read %d frames, want %d", i, len(frames))
			}
			break
		} else if err != nil {
			t.Fatal(err)
		}

		if !ci.Timestamp.Equal(base.Add(time.Duration(i))) {
			t.Errorf("frame %d: time %v", i, ci.Timestamp)
		}
		if ci.Length != len(frames[i])+4 || !bytes.Equal(data, frames[i]) {
			t.Errorf("frame %d: wrong data or length", i)
		}
	}
}

func TestClassicBadRecord(t *testing.T) {
	w, err := NewClassic(io.Discard, 1600)
	if err != nil {
		t.Fatal(err)
	}

	rec := AppendRecord(nil, time.Now(), 10, []byte("0123456789"))

	for name, bad := range map[string][]byte{
		"short":     rec[:10],
		"truncated": rec[:len(rec)-1],
		"incl>orig": AppendRecord(nil, time.Now(), 2, []byte("abc")),
	} {
		if err := w.WriteRecord(0, bad, RecordOptions{}); !errors.Is(err, ErrBadRecord) {
			t.Errorf("%s: got %v, want ErrBadRecord", name, err)
		}
	}

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	if err := w.WriteRecord(0, rec, RecordOptions{}); !errors.Is(err, ErrClosed) {
		t.Errorf("write after close: got %v, want ErrClosed", err)
	}
}
