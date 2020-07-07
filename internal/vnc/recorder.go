// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Recorder keeps track of active recordings.
type Recorder struct {
	mu sync.RWMutex // guards below

	kb map[string]*kbRecorder
	fb map[string]*fbRecorder
}

type recorder struct {
	*Conn // embed

	file  *os.File
	err   error
	done  chan bool
	start time.Time
}

type kbRecorder struct {
	*recorder // embed

	last time.Time
}

type fbRecorder struct {
	*recorder // embed
}

func NewRecorder() *Recorder {
	return &Recorder{
		kb: make(map[string]*kbRecorder),
		fb: make(map[string]*fbRecorder),
	}
}

func newRecorder(rhost, filename string) (*recorder, error) {
	c, err := Dial(rhost)
	if err != nil {
		return nil, err
	}

	f, err := os.Create(filename)
	if err != nil {
		c.Close()
		return nil, err
	}

	return &recorder{
		Conn:  c,
		file:  f,
		start: time.Now(),
		done:  make(chan bool),
	}, nil
}

func (r *recorder) Stop() error {
	if r.file != nil {
		if err := r.file.Close(); err != nil {
			return err
		}
	}

	if r.Conn != nil {
		if err := r.Conn.Close(); err != nil {
			return err
		}
	}

	return nil
}

func (r *Recorder) RecordKB(id, rhost, filename string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// is this ID already being recorded?
	if _, ok := r.kb[id]; ok {
		return fmt.Errorf("kb recording for %v already running", id)
	}

	rc, err := newRecorder(rhost, filename)
	if err != nil {
		return err
	}

	kb := &kbRecorder{
		recorder: rc,
		last:     time.Now(),
	}
	r.kb[id] = kb

	return nil
}

func (r *Recorder) RecordFB(id, rhost, filename string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// is this vm already being recorded?
	if _, ok := r.fb[id]; ok {
		return fmt.Errorf("fb recording for %v already running", id)
	}

	rc, err := newRecorder(rhost, filename)
	if err != nil {
		return err
	}

	fb := &fbRecorder{
		recorder: rc,
	}
	r.fb[id] = fb

	go fb.Record()

	return nil
}

// Route records a message for the correct recording based on the VM
func (r *Recorder) Route(id string, msg interface{}) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r, ok := r.kb[id]; ok {
		r.Record(msg)
	}
}

func (r *Recorder) StopKB(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if kb, ok := r.kb[id]; ok {
		if err := kb.Stop(); err != nil {
			return err
		}

		delete(r.kb, id)
		return nil
	}

	return fmt.Errorf("kb recording %v not found", id)
}

func (r *Recorder) StopFB(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if fb, ok := r.fb[id]; ok {
		if err := fb.Stop(); err != nil {
			return err
		}

		delete(r.fb, id)
		return nil
	}

	return fmt.Errorf("fb recording %v not found", id)
}

// Clear stops all recordings
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, kb := range r.kb {
		log.Debug("stopping kb recording for %v", k)
		if err := kb.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(r.kb, k)
	}

	for k, fb := range r.fb {
		log.Debug("stopping fb recording for %v", k)
		if err := fb.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(r.fb, k)
	}
}

func (r *Recorder) Info() [][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	res := [][]string{}

	for id, kb := range r.kb {
		res = append(res, []string{
			id,
			"record kb",
			time.Since(kb.start).String(),
			kb.file.Name(),
		})
	}

	for id, fb := range r.fb {
		res = append(res, []string{
			id,
			"record fb",
			time.Since(fb.start).String(),
			fb.file.Name(),
		})
	}

	return res
}

// Record records a  client-to-server message in plaintext
func (kb *kbRecorder) Record(msg interface{}) {
	delta := time.Now().Sub(kb.last).Nanoseconds()

	switch msg := msg.(type) {
	case *SetPixelFormat:
	case *SetEncodings:
	case *FramebufferUpdateRequest:
	case *ClientCutText:
		// Don't record
	case *KeyEvent, *PointerEvent:
		fmt.Fprintf(kb.file, "%d:%s\n", delta, msg)
		kb.last = time.Now()
	default:
		log.Info("unexpected  client-to-server message: %#v\n", msg)
	}
}

func (fb *fbRecorder) Record() {
	go func() {
		prev := time.Now()
		buf := make([]byte, 4096)
		writer := gzip.NewWriter(fb.file)
		defer writer.Close()

		for {
			n, err := fb.Conn.Read(buf)
			if err != nil {
				log.Debug(" fb response read failed: %v", err)
				break
			}

			if n > 0 {
				offset := time.Now().Sub(prev).Nanoseconds()
				header := fmt.Sprintf("%d %d\r\n", offset, n)

				if _, err := io.WriteString(writer, header); err != nil {
					log.Debug(" fb write chunk header failed: %v", err)
					break
				}
				if _, err := writer.Write(buf[:n]); err != nil {
					log.Debug(" fb write chunk failed: %v", err)
					break
				}
				if _, err := io.WriteString(writer, "\r\n"); err != nil {
					log.Debug(" fb write chunk tailer failed: %v", err)
					break
				}

				prev = time.Now()

				log.Debug(" fb wrote %d bytes", n)
			}
		}
	}()

	req := &FramebufferUpdateRequest{}

	// fall into a loop issuing periodic fb update requests and dump to file
	// check if we need to quit
	for {
		select {
		case <-fb.done:
			break
		default:
		}

		if err := req.Write(fb.Conn); err != nil {
			fb.err = errors.New("unable to request framebuffer update")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}
