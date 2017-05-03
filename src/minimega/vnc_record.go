// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"os"
	"sync"
	"time"
	"vnc"
)

type vncRecorder struct {
	kb map[string]*vncKBRecord
	fb map[string]*vncFBRecord

	sync.RWMutex // embed
}

type vncKBRecord struct {
	*vncClient
	last time.Time
}

type vncFBRecord struct {
	*vncClient
}

func (v *vncRecorder) RecordKB(vm *KvmVM, filename string) error {
	v.Lock()
	defer v.Unlock()

	// is this vm already being recorded?
	if _, ok := v.kb[vm.Name]; ok {
		return fmt.Errorf("kb recording for %v already running", vm.Name)
	}

	c, err := NewVNCClient(vm)
	if err != nil {
		return err
	}

	c.file, err = os.Create(filename)
	if err != nil {
		return err
	}

	r := &vncKBRecord{vncClient: c, last: time.Now()}
	v.kb[c.ID] = r

	go r.Record()

	return nil
}

func (v *vncRecorder) RecordFB(vm *KvmVM, filename string) error {
	v.Lock()
	defer v.Unlock()

	// is this vm already being recorded?
	if _, ok := v.fb[vm.Name]; ok {
		return fmt.Errorf("fb recording for %v already running", vm.Name)
	}

	c, err := NewVNCClient(vm)
	if err != nil {
		return err
	}

	c.file, err = os.Create(filename)
	if err != nil {
		return err
	}

	c.Conn, err = vnc.Dial(c.Rhost)
	if err != nil {
		return err
	}

	r := &vncFBRecord{c}
	v.fb[c.ID] = r

	go r.Record()

	return nil
}

// Route records a message for the correct recording based on the VM
func (v *vncRecorder) Route(vm VM, msg interface{}) {
	v.RLock()
	defer v.RUnlock()

	if r, ok := v.kb[vm.GetName()]; ok {
		r.RecordMessage(msg)
	}
}

// Clear stops all recordings
func (v *vncRecorder) Clear() {
	v.Lock()
	defer v.Unlock()

	for k, r := range v.kb {
		log.Debug("stopping kb recording for %v", k)
		if err := r.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(v.kb, k)
	}

	for k, r := range v.fb {
		log.Debug("stopping fb recording for %v", k)
		if err := r.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(v.fb, k)
	}
}

// RecordMessage records a VNC client-to-server message in plaintext
func (r *vncKBRecord) RecordMessage(msg interface{}) {
	delta := time.Now().Sub(r.last).Nanoseconds()

	switch msg := msg.(type) {
	case *vnc.SetPixelFormat:
	case *vnc.SetEncodings:
	case *vnc.FramebufferUpdateRequest:
	case *vnc.ClientCutText:
		// Don't record
	case *vnc.KeyEvent, *vnc.PointerEvent:
		fmt.Fprintf(r.file, "%d:%s\n", delta, msg)
		r.last = time.Now()
	default:
		log.Info("unexpected VNC client-to-server message: %#v\n", msg)
	}
}

func (r *vncKBRecord) Record() {
	<-r.done
}

func (v *vncFBRecord) Record() {
	err := (&vnc.SetPixelFormat{
		PixelFormat: vnc.PixelFormat{
			BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1,
			RedMax: 255, GreenMax: 255, BlueMax: 255,
			RedShift: 16, GreenShift: 8, BlueShift: 0,
		},
	}).Write(v.Conn)

	if err != nil {
		v.err = fmt.Errorf("unable to set pixel format: %v", err)
		return
	}

	err = (&vnc.SetEncodings{
		Encodings: []int32{vnc.RawEncoding, vnc.DesktopSizePseudoEncoding},
	}).Write(v.Conn)

	if err != nil {
		v.err = fmt.Errorf("unable to set encodings: %v", err)
		return
	}

	go func() {
		prev := time.Now()
		buf := make([]byte, 4096)
		writer := gzip.NewWriter(v.file)
		defer writer.Close()

		for {
			n, err := v.Conn.Read(buf)
			if err != nil {
				log.Debug("vnc fb response read failed: %v", err)
				break
			}

			if n > 0 {
				offset := time.Now().Sub(prev).Nanoseconds()
				header := fmt.Sprintf("%d %d\r\n", offset, n)

				if _, err := io.WriteString(writer, header); err != nil {
					log.Debug("vnc fb write chunk header failed: %v", err)
					break
				}
				if _, err := writer.Write(buf[:n]); err != nil {
					log.Debug("vnc fb write chunk failed: %v", err)
					break
				}
				if _, err := io.WriteString(writer, "\r\n"); err != nil {
					log.Debug("vnc fb write chunk tailer failed: %v", err)
					break
				}

				prev = time.Now()

				log.Debug("vnc fb wrote %d bytes", n)
			}
		}
	}()

	req := &vnc.FramebufferUpdateRequest{}

	// fall into a loop issuing periodic fb update requests and dump to file
	// check if we need to quit
	for {
		select {
		case <-v.done:
			break
		default:
		}

		if err := req.Write(v.Conn); err != nil {
			v.err = errors.New("unable to request framebuffer update")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}
