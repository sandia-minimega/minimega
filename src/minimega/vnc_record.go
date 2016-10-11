// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"os"
	"strconv"
	"strings"
	"time"
	"vnc"
)

var (
	vncKBRecording map[string]*vncKBRecord
	vncFBRecording map[string]*vncFBRecord
)

func init() {
	vncKBRecording = make(map[string]*vncKBRecord)
	vncFBRecording = make(map[string]*vncFBRecord)
}

type vncKBRecord struct {
	*vncClient
	last time.Time
}

type vncFBRecord struct {
	*vncClient
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

func vncRecordKB(vm *KvmVM, filename string) error {
	c, err := NewVNCClient(vm)
	if err != nil {
		return err
	}

	// is this namespace:vm already being recorded?
	if _, ok := vncKBRecording[c.ID]; ok {
		return fmt.Errorf("kb recording for %v already running", vm.Name)
	}

	c.file, err = os.Create(filename)
	if err != nil {
		return err
	}

	r := &vncKBRecord{vncClient: c, last: time.Now()}
	vncKBRecording[c.ID] = r

	go r.Record()

	return nil
}

func vncRecordFB(vm *KvmVM, filename string) error {
	c, err := NewVNCClient(vm)
	if err != nil {
		return err
	}

	// is this namespace:vm already being recorded?
	if _, ok := vncFBRecording[c.ID]; ok {
		return fmt.Errorf("fb recording for %v already running", vm.Name)
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
	vncFBRecording[c.ID] = r

	go r.Record()

	return nil
}

// Returns the duration of a given kbrecording file
func getDuration(filename string) time.Duration {
	d := 0

	f, _ := os.Open(filename)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		s := strings.SplitN(scanner.Text(), ":", 2)
		i, err := strconv.Atoi(s[0])
		if err != nil {
			log.Errorln(err)
			return 0
		}
		d += i
	}

	duration, err := time.ParseDuration(strconv.Itoa(d) + "ns")
	if err != nil {
		log.Errorln(err)
		return 0
	}

	return duration
}
