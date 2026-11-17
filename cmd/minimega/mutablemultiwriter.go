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
	// a failed or short writer (e.g. a dead console connection) is dropped
	// so that it cannot stop the copy from the pty to the other writers
	live := t.writers[:0]
	for _, w := range t.writers {
		if w == nil {
			continue
		}
		if n, err := w.Write(p); err != nil || n != len(p) {
			continue
		}
		live = append(live, w)
	}
	for i := len(live); i < len(t.writers); i++ {
		t.writers[i] = nil
	}
	t.writers = live
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
