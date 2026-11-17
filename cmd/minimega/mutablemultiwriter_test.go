package main

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type failWriter struct{}

func (failWriter) Write(p []byte) (int, error) { return 0, errors.New("conn reset") }

// A dead console client must not stop io.Copy from draining the pty, and
// the writers after it must still receive data.
func TestMutableMultiWriterSurvivesFailedWriter(t *testing.T) {
	var a, b bytes.Buffer
	mw := NewMutableMultiWriter(&a, failWriter{}, &b)

	n, err := io.Copy(mw, bytes.NewBufferString("hello"))
	if err != nil || n != 5 {
		t.Fatalf("copy = %d, %v; want 5, nil", n, err)
	}
	if _, err := io.Copy(mw, bytes.NewBufferString(" world")); err != nil {
		t.Fatalf("second copy: %v", err)
	}
	if a.String() != "hello world" || b.String() != "hello world" {
		t.Fatalf("a=%q b=%q", a.String(), b.String())
	}
}
