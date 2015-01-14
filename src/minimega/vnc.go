// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	vncRecording   map[string]*vncVMRecord
	vncFBRecording map[string]*vncFBRecord
	vncPlaying     map[string]*vncVMPlayback
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

type vncFBRecord struct {
	vncVMRecord

	Rhost string
	conn  net.Conn
	sInit *vncServerInit
}

type vncServerInit struct {
	Width, Height uint16
	PixelFormat   vncPixelFormat
}

type vncPixelFormat struct {
	BitsPerPixel, Depth, BigEndianFlag, TrueColorFlag uint8
	RedMax, GreenMax, BlueMax                         uint16
	RedShift, GreenShift, BlueShift                   uint8
	Padding                                           [3]byte
}

var vncCLIHandlers = []minicli.Handler{
	{ // vnc
		HelpShort: "record or playback VNC kbd/mouse input",
		HelpLong: `
Record or playback keyboard and mouse events sent via the web interface to the
selected VM.

With no arguments, vnc will list currently recording or playing VNC sessions.

If record is selected, a file will be created containing a record of mouse and
keyboard actions by the user.

If playback is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.`,
		Patterns: []string{
			"vnc",
			"vnc <kb,fb> <record,> <host> <vm id or name> <filename>",
			"vnc <kb,fb> <norecord,> <host> <vm id or name>",
			"vnc <playback,> <host> <vm id or name> <filename>",
			"vnc <noplayback,> <host> <vm id or name>",
			"clear vnc",
		},
		Record: false,
		Call:   cliVNC,
	},
}

func init() {
	registerHandlers("vnc", vncCLIHandlers)

	vncRecording = make(map[string]*vncVMRecord)
	vncFBRecording = make(map[string]*vncFBRecord)
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
func vncDial(rhost string) (conn net.Conn, serverInit *vncServerInit, err error) {
	var n int

	serverInit = &vncServerInit{}

	conn, err = net.Dial("tcp", rhost)
	if err != nil {
		return nil, nil, err
	}

	defer func() {
		if err != nil && conn != nil {
			conn.Close()
		}
	}()

	// handshake, receive 12 bytes from the server
	buf := make([]byte, 12)
	n, err = conn.Read(buf)
	if err != nil && n != 12 {
		err = fmt.Errorf("invalid server version: %v", string(buf[:n]))
	}

	if err != nil {
		return
	}

	// respond with version 3.3
	buf = []byte("RFB 003.003\n")
	_, err = conn.Write(buf)
	if err != nil {
		return
	}

	// the server sends a 4 byte security type
	buf = make([]byte, 4)
	n, err = conn.Read(buf)
	if err != nil && n != 4 {
		err = fmt.Errorf("invalid server security message: %v", string(buf[:n]))
	} else if err != nil && buf[3] != 0x01 { // the security type must be 1
		err = fmt.Errorf("invalid server security type: %v", string(buf[:n]))
	}

	if err != nil {
		return
	}

	// client sends an initialization message, non-zero here to indicate
	// we will allow a shared desktop.
	_, err = conn.Write([]byte{0x01})
	if err != nil {
		return
	}

	// receive the server initialization
	buf = make([]byte, 32768)
	n, err = conn.Read(buf)
	if err != nil {
		return
	}
	log.Debug("got server initialization length %v", n)

	reader := bytes.NewReader(buf)
	if err = binary.Read(reader, binary.BigEndian, &serverInit.Width); err != nil {
		err = errors.New("unable to decode width")
		return
	}
	if err = binary.Read(reader, binary.BigEndian, &serverInit.Height); err != nil {
		err = errors.New("unable to decode height")
		return
	}
	if err = binary.Read(reader, binary.BigEndian, &serverInit.PixelFormat); err != nil {
		err = errors.New("unable to decode pixel format")
		return
	}

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
	_, err = conn.Write(buf)

	return
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

func NewFBRecord(filename string) (*vncFBRecord, error) {
	log.Debug("NewFBRecord: %v", filename)
	ret := &vncFBRecord{}
	fi, err := os.Create(filename)
	if err != nil {
		return ret, err
	}
	ret.file = fi
	ret.last = time.Now()

	return ret, nil
}

func (v *vncFBRecord) Dial() error {
	var err error
	v.conn, v.sInit, err = vncDial(v.Rhost)
	if err == nil {
		log.Debug("got server init: %#v", v.sInit)
	}

	return err
}

func (v *vncFBRecord) Run() {
	defer v.file.Close()

	// Only accept raw encoding
	n, err := v.conn.Write([]byte{0x02, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21})
	if err != nil {
		log.Debug("vnc handshake failed: %v", err)
		return
	} else if n != 12 {
		log.Debug("vnc handshake failed: couldn't write bytes")
		return
	}

	go func() {
		prev := time.Now()
		buf := make([]byte, 4096)
		writer := gzip.NewWriter(v.file)
		defer writer.Close()

		for {
			n, err := v.conn.Read(buf)
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

	// fall into a loop issuing periodic fb update requests and dump to file
	// check if we need to quit
	for {
		buf := []byte{0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

		// Send fb update request
		n, err := v.conn.Write(buf)
		if err != nil {
			log.Debug("vnc fb request failed: %v", err)
			return
		} else if n != len(buf) {
			log.Debug("vnc fb request failed: couldn't write bytes")
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
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
		v.last = time.Now()
	}
}

func (v *vncVMRecord) Close() {
	v.output.Flush()
	v.file.Close()
}

func vncRecordKB(host, vm, filename string) error {
	vmID, vmName, err := findRemoteVM(host, vm)
	if err != nil {
		return err
	}

	rhost := fmt.Sprintf("%v:%v", host, 5900+vmID)

	// is this rhost already being recorded?
	if _, ok := vncRecording[rhost]; ok {
		return fmt.Errorf("kb recording for %v %v already running", host, vm)
	}

	vmr, err := NewVMRecord(filename)
	if err != nil {
		log.Errorln(err)
		return err
	}

	vmr.Filename = filename
	vmr.Host = host
	vmr.Name = vmName
	vmr.ID = vmID

	vncRecording[rhost] = vmr

	return nil
}

func vncRecordFB(host, vm, filename string) error {
	vmID, vmName, err := findRemoteVM(host, vm)
	if err != nil {
		return err
	}

	rhost := fmt.Sprintf("%v:%v", host, 5900+vmID)

	// is this rhost already being recorded?
	if _, ok := vncFBRecording[rhost]; ok {
		return fmt.Errorf("fb recording for %v %v already running", host, vm)
	}

	// fb recording
	fbr, err := NewFBRecord(filename)
	if err != nil {
		log.Errorln(err)
		return err
	}

	fbr.Filename = filename
	fbr.Host = host
	fbr.Name = vmName
	fbr.ID = vmID
	fbr.Rhost = rhost

	// attempt to connect
	err = fbr.Dial()
	if err != nil {
		return err
	}

	go fbr.Run()

	vncFBRecording[rhost] = fbr

	return nil
}

func vncPlayback(host, vm, filename string) error {
	vmID, vmName, err := findRemoteVM(host, vm)
	if err != nil {
		return err
	}

	rhost := fmt.Sprintf("%v:%v", host, 5900+vmID)

	// is this rhost already being recorded?
	if _, ok := vncPlaying[rhost]; ok {
		return fmt.Errorf("fb playback for %v %v already running", host, vm)
	}

	vmp, err := NewVMPlayback(filename)
	if err != nil {
		log.Errorln(err)
		return err
	}

	vmp.Filename = filename
	vmp.Host = host
	vmp.Name = vmName
	vmp.ID = vmID
	vmp.Rhost = rhost

	vmp.conn, _, err = vncDial(vmp.Rhost)
	if err != nil {
		return fmt.Errorf("vnc handshake: %v", err)
	}

	vncPlaying[rhost] = vmp
	go vmp.Run()

	return nil
}

func cliVNC(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}
	var err error

	host := c.StringArgs["host"]
	vm := c.StringArgs["vm"]
	fname := c.StringArgs["filename"]

	if isClearCommand(c) {
		err = vncClear()
	} else if c.BoolArgs["record"] && c.BoolArgs["kb"] {
		// Starting keyboard recording
		err = vncRecordKB(host, vm, fname)
	} else if c.BoolArgs["record"] && c.BoolArgs["fb"] {
		// Starting framebuffer recording
		err = vncRecordFB(host, vm, fname)
	} else if c.BoolArgs["norecord"] && c.BoolArgs["kb"] {
		// Stopping keyboard recording
		var found bool
		for k, vmr := range vncRecording {
			if vmr.Host == host && (vmr.Name == vm || strconv.Itoa(vmr.ID) == vm) {
				vmr.Close()
				delete(vncRecording, k)
				found = true
				break
			}
		}

		if !found {
			err = fmt.Errorf("kb recording %v %v not found", host, vm)
		}
	} else if c.BoolArgs["norecord"] && c.BoolArgs["fb"] {
		// Stopping framebuffer recording
		var found bool
		for k, vmr := range vncFBRecording {
			if vmr.Host == host && (vmr.Name == vm || strconv.Itoa(vmr.ID) == vm) {
				vmr.conn.Close()
				delete(vncFBRecording, k)
				found = true
				break
			}
		}

		if !found {
			err = fmt.Errorf("fb recording %v %v not found", host, vm)
		}
	} else if c.BoolArgs["playback"] {
		// Start keyboard playback
		err = vncPlayback(host, vm, fname)
	} else if c.BoolArgs["noplayback"] {
		// Stop keyboard playback
		var found bool
		for k, vmp := range vncPlaying {
			if vmp.Host == host && (vmp.Name == vm || strconv.Itoa(vmp.ID) == vm) {
				vmp.Stop()
				delete(vncPlaying, k)
				found = true
				break
			}
		}

		if !found {
			err = fmt.Errorf("recording %v %v not found", host, vm)
		}
	} else {
		// List all active recordings and playbacks
		resp.Header = []string{"Host", "VM name", "VM id", "Type", "Filename"}
		resp.Tabular = [][]string{}

		for _, v := range vncRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record kb",
				v.Filename,
			})
		}

		for _, v := range vncFBRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record fb",
				v.Filename,
			})
		}

		for _, v := range vncPlaying {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"playback kb",
				v.Filename,
			})
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}
	return minicli.Responses{resp}
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
