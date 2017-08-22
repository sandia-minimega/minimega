package main

import (
	"sync"
	"io"
)

type mutableMultiWriter struct {
	writers []io.Writer
	sync.Mutex
}

func (t *mutableMultiWriter) Write(p []byte) (n int, err error) {
	t.Lock()
	defer t.Unlock()
	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
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

// MultiWriter creates a writer that duplicates its writes to all the
// provided writers, similar to the Unix tee(1) command.
func NewMutableMultiWriter(writers ...io.Writer) *mutableMultiWriter {
	w := make([]io.Writer, len(writers))
	copy(w, writers)
	return &mutableMultiWriter{writers: w}
}
