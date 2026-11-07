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

// fbRequestInterval is the time between two FramebufferUpdateRequests of an
// fb recording.
const fbRequestInterval = 100 * time.Millisecond

// Recorder keeps track of active recordings.
type Recorder struct {
	mu sync.RWMutex // guards below

	kb map[string]*kbRecorder
	fb map[string]*fbRecorder
}

type recorder struct {
	*Conn // embed

	file  *os.File
	start time.Time
}

type kbRecorder struct {
	*recorder // embed

	mu   sync.Mutex // guards last and the writes to file
	last time.Time
}

type fbRecorder struct {
	*recorder // embed

	done     chan struct{} // closed by Stop, ends the request loop
	readDone chan struct{} // closed when the reader goroutine ends
	readErr  error         // error that the reader gives, valid after readDone

	stopOnce sync.Once
	stopErr  error
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
	}, nil
}

// close closes the connection and then the file. It closes both, even if
// the first Close fails.
func (r *recorder) close() error {
	var errs []error

	if r.Conn != nil {
		if err := r.Conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close vnc connection: %v", err))
		}
	}

	if r.file != nil {
		if err := r.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %v: %v", r.file.Name(), err))
		}
	}

	return errors.Join(errs...)
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
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	r.fb[id] = fb

	go fb.read()
	go fb.request()

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

// StopKB stops the kb recording of id. It always removes the recording, and
// returns the errors from the Close calls.
func (r *Recorder) StopKB(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	kb, ok := r.kb[id]
	if !ok {
		return fmt.Errorf("kb recording %v not found", id)
	}

	delete(r.kb, id)
	return kb.Stop()
}

// StopFB stops the fb recording of id. It always removes the recording, and
// returns the errors from the Close calls.
func (r *Recorder) StopFB(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fb, ok := r.fb[id]
	if !ok {
		return fmt.Errorf("fb recording %v not found", id)
	}

	delete(r.fb, id)
	return fb.Stop()
}

// StopVM stops the kb and fb recordings of id, if there are any. Use it when
// the VM is killed or flushed.
func (r *Recorder) StopVM(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var errs []error

	if kb, ok := r.kb[id]; ok {
		log.Debug("stopping kb recording for %v", id)
		delete(r.kb, id)
		errs = append(errs, kb.Stop())
	}

	if fb, ok := r.fb[id]; ok {
		log.Debug("stopping fb recording for %v", id)
		delete(r.fb, id)
		errs = append(errs, fb.Stop())
	}

	return errors.Join(errs...)
}

// Clear stops all recordings
func (r *Recorder) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for k, kb := range r.kb {
		log.Debug("stopping kb recording for %v", k)
		delete(r.kb, k)
		if err := kb.Stop(); err != nil {
			log.Error("%v", err)
		}
	}

	for k, fb := range r.fb {
		log.Debug("stopping fb recording for %v", k)
		delete(r.fb, k)
		if err := fb.Stop(); err != nil {
			log.Error("%v", err)
		}
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

// Stop closes the connection and the file.
func (kb *kbRecorder) Stop() error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	return kb.close()
}

// Record records a client-to-server message in plaintext
func (kb *kbRecorder) Record(msg interface{}) {
	var e fmt.Stringer

	switch msg := msg.(type) {
	case *SetPixelFormat, *SetEncodings, *FramebufferUpdateRequest,
		*ClientCutText, *ExtendedClientCutText, *EnableContinuousUpdates,
		*ClientFence, *XVPMessage, *SetDesktopSize, *QEMUAudioMessage:
		// Not recorded
		return
	case *KeyEvent:
		e = msg
	case *QEMUExtendedKeyEvent:
		e = msg.KeyEvent()
	case *PointerEvent:
		e = &kbPointerEvent{Mask: uint16(msg.ButtonMask), XPosition: msg.XPosition, YPosition: msg.YPosition}
	case *ExtendedPointerEvent:
		e = &kbPointerEvent{Mask: msg.Mask(), XPosition: msg.XPosition, YPosition: msg.YPosition}
	default:
		log.Info("unexpected client-to-server message: %#v", msg)
		return
	}

	if ke, ok := e.(*KeyEvent); ok && ke.Key == 0 {
		log.Debug("skip KeyEvent with keysym 0")
		return
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	now := time.Now()
	if _, err := fmt.Fprintf(kb.file, "%d:%s\n", now.Sub(kb.last).Nanoseconds(), e); err != nil {
		log.Error("unable to write kb recording %v: %v", kb.file.Name(), err)
	}
	kb.last = now
}

// Stop ends the request loop, closes the connection, and waits for the
// reader goroutine to close the file. It returns the errors from the Close
// calls and from the writes to the file.
func (fb *fbRecorder) Stop() error {
	fb.stopOnce.Do(func() {
		close(fb.done)

		var errs []error
		if err := fb.Conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close vnc connection: %v", err))
		}

		<-fb.readDone
		fb.stopErr = errors.Join(append(errs, fb.readErr)...)
	})

	return fb.stopErr
}

// read copies the server bytes to the gzip file in chunks until the
// connection ends. Then it closes the gzip writer and the file.
func (fb *fbRecorder) read() {
	defer close(fb.readDone)

	writer := gzip.NewWriter(fb.file)
	prev := time.Now()
	buf := make([]byte, 4096)

	var werr error
	for werr == nil {
		n, err := fb.Conn.Read(buf)
		if n > 0 {
			offset := time.Since(prev).Nanoseconds()
			prev = time.Now()

			werr = fb.writeChunk(writer, offset, buf[:n])
			log.Debug("fb wrote %d bytes", n)
		}
		if err != nil {
			log.Debug("fb response read failed: %v", err)
			break
		}
	}

	var errs []error
	if werr != nil {
		errs = append(errs, fmt.Errorf("write %v: %v", fb.file.Name(), werr))
	}
	if err := writer.Close(); err != nil {
		errs = append(errs, fmt.Errorf("write %v: %v", fb.file.Name(), err))
	}
	if err := fb.file.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close %v: %v", fb.file.Name(), err))
	}

	fb.readErr = errors.Join(errs...)
}

func (fb *fbRecorder) writeChunk(w io.Writer, offset int64, b []byte) error {
	if _, err := fmt.Fprintf(w, "%d %d\r\n", offset, len(b)); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\r\n")
	return err
}

// request asks for the whole screen every fbRequestInterval until Stop or the
// end of the reader. QEMU clamps the rectangle to the current screen size, so
// the frames follow a guest resize.
func (fb *fbRecorder) request() {
	req := &FramebufferUpdateRequest{Width: 0xffff, Height: 0xffff}

	ticker := time.NewTicker(fbRequestInterval)
	defer ticker.Stop()

	for {
		if err := req.Write(fb.Conn); err != nil {
			log.Debug("fb update request failed: %v", err)
			return
		}

		select {
		case <-fb.done:
			return
		case <-fb.readDone:
			return
		case <-ticker.C:
		}
	}
}
