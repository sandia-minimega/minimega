// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"compress/gzip"
	"encoding/base64"
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
	vncKBPlaying   map[string]*vncKBPlayback
)

type Foo interface {
	Matches(string, string) bool
	Stop() error
}

type vncClient struct {
	Host  string
	Name  string
	ID    int
	Rhost string

	done chan bool
	file *os.File

	err error

	Conn *vnc.Conn
}

type vncKBRecord struct {
	*vncClient

	last time.Time
}

type vncFBRecord struct {
	*vncClient
}

type vncKBPlayback struct {
	*vncClient
}

func init() {
	vncKBRecording = make(map[string]*vncKBRecord)
	vncFBRecording = make(map[string]*vncFBRecord)
	vncKBPlaying = make(map[string]*vncKBPlayback)
}

func NewVNCClient(host, vm string) (*vncClient, error) {
	vmID, vmName, err := findRemoteVM(host, vm)
	if err != nil {
		return nil, err
	}

	rhost := fmt.Sprintf("%v:%v", host, 5900+vmID)

	c := &vncClient{
		Rhost: rhost,
		Host:  host,
		Name:  vmName,
		ID:    vmID,
		done:  make(chan bool),
	}

	return c, nil
}

func (v *vncClient) Matches(host, vm string) bool {
	return v.Host == host && (v.Name == vm || strconv.Itoa(v.ID) == vm)
}

func (v *vncClient) Stop() error {
	<-v.done

	if v.file != nil {
		v.file.Close()
	}

	if v.Conn != nil {
		v.Conn.Close()
	}

	return v.err
}

// Input ought to be a base64-encoded string as read from the websocket
// connected to NoVNC. If not, well, oops.
func (r *vncKBRecord) AddAction(s string) {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		log.Errorln(err)
		return
	}

	if d[0] == 4 || d[0] == 5 {
		record := fmt.Sprintf("%d %s\n", (time.Now().Sub(r.last)).Nanoseconds(), s)
		r.file.WriteString(record)
		r.last = time.Now()
	}
}

func (r *vncKBRecord) Run() {
	<-r.done
	r.file.Close()
}

func (v *vncFBRecord) Run() {
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
		Encodings: []int32{vnc.RawEncoding, vnc.DesktopSize},
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

func (v *vncKBPlayback) Run() {
	var buf []byte

	scanner := bufio.NewScanner(v.file)

	for scanner.Scan() && v.err == nil {
		s := strings.Split(scanner.Text(), " ")
		if len(s) != 2 {
			continue
		}

		ns := s[0] + "ns"
		duration, err := time.ParseDuration(ns)
		if err != nil {
			log.Errorln(err)
			continue
		}

		wait := time.After(duration)
		select {
		case <-wait:
		case <-v.done:
			return
		}

		buf, v.err = base64.StdEncoding.DecodeString(s[1])
		if v.err != nil {
			return
		}

		_, v.err = v.Conn.Write(buf)
	}
}

func vncRecordKB(host, vm, filename string) error {
	c, err := NewVNCClient(host, vm)
	if err != nil {
		return err
	}

	// is this rhost already being recorded?
	if _, ok := vncKBRecording[c.Rhost]; ok {
		return fmt.Errorf("kb recording for %v %v already running", host, vm)
	}

	c.file, err = os.Create(filename)
	if err != nil {
		return err
	}

	r := &vncKBRecord{vncClient: c, last: time.Now()}
	vncKBRecording[c.Rhost] = r

	go r.Run()

	return nil
}

func vncRecordFB(host, vm, filename string) error {
	c, err := NewVNCClient(host, vm)
	if err != nil {
		return err
	}

	// is this rhost already being recorded?
	if _, ok := vncFBRecording[c.Rhost]; ok {
		return fmt.Errorf("fb recording for %v %v already running", host, vm)
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
	vncFBRecording[c.Rhost] = r

	go r.Run()

	return nil
}

func vncPlaybackKB(host, vm, filename string) error {
	c, err := NewVNCClient(host, vm)
	if err != nil {
		return err
	}

	// is this rhost already being recorded?
	if _, ok := vncKBPlaying[c.Rhost]; ok {
		return fmt.Errorf("fb playback for %v %v already running", host, vm)
	}

	c.file, err = os.Open(filename)
	if err != nil {
		return err
	}

	c.Conn, err = vnc.Dial(c.Rhost)
	if err != nil {
		return err
	}

	r := &vncKBPlayback{c}
	vncKBPlaying[c.Rhost] = r

	go r.Run()

	return nil
}

func vncClear() error {
	for k, v := range vncKBRecording {
		log.Debug("stopping kb recording for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncKBRecording, k)
	}

	for k, v := range vncFBRecording {
		log.Debug("stopping fb recording for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncFBRecording, k)
	}

	for k, v := range vncKBPlaying {
		log.Debug("stopping kb playing for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncKBPlaying, k)
	}

	return nil
}
