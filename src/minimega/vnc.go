// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	log "minilog"
	"os"
	"strconv"
	"strings"
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

	input *bufio.Reader
	file  *os.File

	nextEvent chan []byte
	done      chan bool
}

func init() {
	vncRecording = make(map[string]*vncVMRecord)
	vncPlaying = make(map[string]*vncVMPlayback)
}

func NewVMPlayback(filename string) (*vncVMPlayback, error) {
	ret := &vncVMPlayback{}
	ret.nextEvent = make(chan []byte)
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
		v.nextEvent <- []byte(s[1])
	}
	v.Stop()
}

func (v *vncVMPlayback) Stop() {
	v.file.Close()
	close(v.done) // this should cause the select in Run() to come back
	close(v.nextEvent)
}

func NewVMRecord(filename string) (*vncVMRecord, error) {
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
	record := fmt.Sprintf("%d %s\n", (time.Now().Sub(v.last)).Nanoseconds(), s)
	v.output.WriteString(record)
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
				// will be deleted elsewhere
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
