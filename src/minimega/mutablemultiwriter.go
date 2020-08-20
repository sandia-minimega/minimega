package main

import (
	"io"
	"sync"
)

type mutableMultiWriter struct {
	writers []io.Writer
	sync.Mutex
}

func (t *mutableMultiWriter) Write(p []byte) (n int, err error) {
	t.Lock()
	defer t.Unlock()
	for i, w := range t.writers {
		if w == nil {
			continue
		}
		n, err = w.Write(p)
		if err != nil {
			// if a write fails, get rid of the writer
			t.writers[i] = nil
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}

func (t *mutableMultiWriter) AddWriter(writer io.Writer) {
	t.Lock()
	defer t.Unlock()
	t.writers = append(t.writers, writer)
}

func (t *mutableMultiWriter) DelWriter(writer io.Writer) {
	t.Lock()
	defer t.Unlock()
	for i, _ := range t.writers {
		if t.writers[i] == writer {
			t.writers = append(t.writers[:i], t.writers[i+1:]...)
			return
		}
	}
}

// MultiWriter creates a writer that duplicates its writes to all the
// provided writers, similar to the Unix tee(1) command.
func NewMutableMultiWriter(writers ...io.Writer) *mutableMultiWriter {
	w := make([]io.Writer, len(writers))
	copy(w, writers)
	return &mutableMultiWriter{writers: w}
}
