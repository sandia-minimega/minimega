// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	log "minilog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

var (
	vncRecording map[string]*vncVMRecord
	vncPlaying   map[string]*vncVMPlayback
)

type vncVMRecord struct {
	Host     string
	Name     string
	ID       int
	Filename string

	last   time.Time
	output *bufio.Writer
	file   *os.File
}

type vncVMPlayback struct {
	Host     string
	Name     string
	ID       int
	Filename string
	Rhost    string

	input *bufio.Reader
	file  *os.File
	conn  net.Conn

	done     chan bool
	finished bool
	lock     sync.Mutex
}

func init() {
	vncRecording = make(map[string]*vncVMRecord)
	vncPlaying = make(map[string]*vncVMPlayback)
}

func NewVMPlayback(filename string) (*vncVMPlayback, error) {
	log.Debug("NewVMPlayback: %v", filename)
	ret := &vncVMPlayback{}
	ret.done = make(chan bool)
	fi, err := os.Open(filename)
	if err != nil {
		return ret, err
	}
	ret.file = fi
	ret.input = bufio.NewReader(fi)
	return ret, nil
}

func (v *vncVMPlayback) Run() {
	scanner := bufio.NewScanner(v.input)
	defer v.conn.Close()
	defer v.Stop()
	for scanner.Scan() {
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
		b, err := base64.StdEncoding.DecodeString(s[1])
		if err != nil {
			log.Errorln(err)
			return
		}
		_, err = v.conn.Write(b)
		if err != nil {
			log.Errorln(err)
			return
		}
	}
}

// dial the vm in question, complete a handshake, and discard incoming
// messages.
func (v *vncVMPlayback) Dial() error {
	conn, err := net.Dial("tcp", v.Rhost)
	if err != nil {
		return err
	}

	v.conn = conn

	// handshake, receive 12 bytes from the server
	buf := make([]byte, 12)
	n, err := v.conn.Read(buf)
	if err != nil {
		return err
	}
	if n != 12 {
		return fmt.Errorf("invalid server version: %v", string(buf[:n]))
	}
	// respond with version 3.3
	buf = []byte("RFB 003.003\n")
	_, err = v.conn.Write(buf)
	if err != nil {
		return err
	}

	// the server sends a 4 byte security type
	buf = make([]byte, 4)
	n, err = v.conn.Read(buf)
	if err != nil {
		return err
	}
	if n != 4 {
		return fmt.Errorf("invalid server security message: %v", string(buf[:n]))
	}
	// the security type must be 1
	if buf[3] != 0x01 {
		return fmt.Errorf("invalid server security type: %v", string(buf[:n]))
	}

	// client sends an initialization message, non-zero here to indicate
	// we will allow a shared desktop.
	_, err = v.conn.Write([]byte{0x01})
	if err != nil {
		return err
	}

	// receive the server initialization
	buf = make([]byte, 32768)
	n, err = v.conn.Read(buf)
	if err != nil {
		return err
	}
	log.Debug("got server initialization length %v", n)

	// success!
	//	go func() {
	//		for {
	//			_, err := v.conn.Read(buf)
	//			if err != nil {
	//				if !strings.Contains(err.Error(), "closed network connection") && err != io.EOF {
	//					log.Errorln(err)
	//				}
	//				return
	//			}
	//		}
	//	}()
	buf = []byte{0, 0, 0, 0, 32, 24, 0, 1, 0, 255, 0, 255, 0, 255, 16, 8, 0, 0, 0, 0, 2, 0, 0, 11, 0, 0, 0, 1, 0, 0, 0, 7, 255, 255, 254, 252, 0, 0, 0, 5, 0, 0, 0, 2, 0, 0, 0, 0, 255, 255, 255, 33, 255, 255, 255, 17, 255, 255, 255, 230, 255, 255, 255, 9, 255, 255, 255, 32, 3, 0, 0, 0, 0, 0, 3, 32, 2, 88}
	_, err = v.conn.Write(buf)
	if err != nil {
		return err
	}

	return nil
}

func (v *vncVMPlayback) Stop() {
	v.lock.Lock()
	defer v.lock.Unlock()
	if !v.finished {
		v.file.Close()
		v.conn.Close()
		close(v.done) // this should cause the select in Run() to come back
		v.finished = true
		delete(vncPlaying, v.Rhost)
	}
}

func NewVMRecord(filename string) (*vncVMRecord, error) {
	log.Debug("NewVMRecord: %v", filename)
	ret := &vncVMRecord{}
	fi, err := os.Create(filename)
	if err != nil {
		return ret, err
	}
	ret.file = fi
	ret.output = bufio.NewWriter(fi)
	ret.last = time.Now()

	return ret, nil
}

// Input ought to be a base64-encoded string as read from the websocket
// connected to NoVNC. If not, well, oops.
func (v *vncVMRecord) AddAction(s string) {
	d, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		log.Errorln(err)
		return
	}
	if d[0] == 4 || d[0] == 5 {
		record := fmt.Sprintf("%d %s\n", (time.Now().Sub(v.last)).Nanoseconds(), s)
		v.output.WriteString(record)
	}
	v.last = time.Now()
}

func (v *vncVMRecord) Close() {
	v.output.Flush()
	v.file.Close()
}

func cliVNC(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0: // show current recordings/playbacks
		var recordings string
		var playbacks string
		var o bytes.Buffer

		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintln(&o, "Recordings:")
		fmt.Fprintf(w, "Host\tVM name\tVM id\tFile\n")
		for _, v := range vncRecording {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", v.Host, v.Name, v.ID, v.Filename)
		}
		w.Flush()
		recordings = o.String()

		o.Reset()

		w = new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintln(&o, "Playbacks:")
		fmt.Fprintf(w, "Host\tVM name\tVM id\tFile\n")
		for _, v := range vncPlaying {
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", v.Host, v.Name, v.ID, v.Filename)
		}
		w.Flush()
		playbacks = o.String()

		return cliResponse{
			Response: fmt.Sprintf("%v\n%v", recordings, playbacks),
		}
	case 3: // [norecord|noplayback] <host> <vm>
		if c.Args[0] != "norecord" && c.Args[0] != "noplayback" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		host := c.Args[1]
		vm := c.Args[2]
		vmID, err := strconv.Atoi(vm)
		if err != nil {
			vmID = -1
		}

		var rhost string
		id := -1

		// attempt to find a match
		for _, v := range vncPlaying {
			if v.Host == host {
				if v.Name == vm {
					id = v.ID
					break
				}
				if vmID != -1 && v.ID == vmID {
					id = vmID
					break
				}
			}
		}
		if id == -1 { // check in recordings
			for _, v := range vncRecording {
				if v.Host == host {
					if v.Name == vm {
						id = v.ID
						break
					}
					if vmID != -1 && v.ID == vmID {
						id = vmID
						break
					}
				}
			}
		}

		if id == -1 {
			return cliResponse{
				Error: fmt.Sprintf("recording/playback %v %v not found", host, vm),
			}
		}

		rhost = fmt.Sprintf("%v:%v", host, 5900+id)
		switch {
		case c.Args[0] == "norecord":
			if _, ok := vncRecording[rhost]; ok {
				vncRecording[rhost].Close()
				delete(vncRecording, rhost)
			}
		case c.Args[0] == "noplayback":
			if _, ok := vncPlaying[rhost]; ok {
				vncPlaying[rhost].Stop()
			}
		}
	case 4: // [record|playback] <host> <vm> <file>
		if c.Args[0] != "record" && c.Args[0] != "playback" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		host := c.Args[1]
		vm := c.Args[2]

		vmID, vmName, err := findRemoteVM(host, vm)
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		filename := c.Args[3]
		rhost := fmt.Sprintf("%v:%v", host, 5900+vmID)

		// is this rhost already being recorded?
		if _, ok := vncRecording[rhost]; ok {
			return cliResponse{
				Error: fmt.Sprintf("recording for %v %v already running", host, vm),
			}
		}

		switch {
		case c.Args[0] == "record":
			vmr, err := NewVMRecord(filename)
			if err != nil {
				log.Errorln(err)
				return cliResponse{
					Error: err.Error(),
				}
			}
			vmr.Filename = filename
			vmr.Host = host
			vmr.Name = vmName
			vmr.ID = vmID
			vncRecording[rhost] = vmr
		case c.Args[0] == "playback":
			vmp, err := NewVMPlayback(filename)
			if err != nil {
				log.Errorln(err)
				return cliResponse{
					Error: err.Error(),
				}
			}
			vmp.Filename = filename
			vmp.Host = host
			vmp.Name = vmName
			vmp.ID = vmID
			vmp.Rhost = fmt.Sprintf("%v:%v", host, 5900+vmID)
			err = vmp.Dial()
			if err != nil {
				vmp.conn.Close()
				return cliResponse{
					Error: fmt.Sprintf("vnc handshake: %v", err),
				}
			}
			vncPlaying[rhost] = vmp
			go vmp.Run()
		}
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}
	return cliResponse{}
}

func vncClear() error {
	log.Debugln("vncClear")
	for k, v := range vncRecording {
		log.Debug("stopping recording for %v", k)
		v.Close()
		delete(vncRecording, k)
	}
	for k, v := range vncPlaying {
		log.Debug("stopping playback for %v", k)
		v.Stop()
	}
	return nil
}
