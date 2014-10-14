// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// novnctun supports creating a websocket based tunnel to vnc ports on other
// hosts and serving a novnc client to the machine requesting the tunnel. This
// is used to automate connecting to virtual machines on a cluster when the
// user does not have direct access to cluster nodes. novnctun runs on the
// routable head node of the cluster, the user connects to it, and tunnels are
// created to connect to virtual machines.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	log "minilog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"websocket"
)

// Global variables rulez
var (
	recording map[string]*vmrecord
	playing   map[string]*vmplayback
)

type vmrecord struct {
	last   time.Time
	output *bufio.Writer
	file   *os.File
}

type vmplayback struct {
	input *bufio.Reader
	file  *os.File

	nextevent chan []byte
	done      chan bool
}

func init() {
	recording = make(map[string]*vmrecord)
	playing = make(map[string]*vmplayback)
}

func NewVMPlayback(filename string) (*vmplayback, error) {
	ret := &vmplayback{}
	ret.nextevent = make(chan []byte)
	ret.done = make(chan bool)
	fi, err := os.Open(filename)
	if err != nil {
		return ret, err
	}
	ret.file = fi
	ret.input = bufio.NewReader(fi)
	return ret, nil
}

func (v *vmplayback) Run() {
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
		v.nextevent <- []byte(s[1])
	}
	v.Stop()
}

func (v *vmplayback) Stop() {
	v.file.Close()
	close(v.done) // this should cause the select in Run() to come back
	close(v.nextevent)
}

func NewVMRecord(filename string) (*vmrecord, error) {
	ret := &vmrecord{}
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
func (v *vmrecord) AddAction(s string) {
	record := fmt.Sprintf("%d %s\n", (time.Now().Sub(v.last)).Nanoseconds(), s)
	v.output.WriteString(record)
	v.last = time.Now()
}

func (v *vmrecord) Close() {
	v.output.Flush()
	v.file.Close()
}

const BUF = 32768

func WsHandler(w http.ResponseWriter, r *http.Request) {
	// we assume that if we got here, then the url must be sane and of
	// the format /<host>/<port>
	url := r.URL.String()
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	fields := strings.Split(url, "/")
	if len(fields) != 6 {
		http.NotFound(w, r)
		return
	}

	rhost := fmt.Sprintf("%v:%v", fields[3], fields[4])

	// connect to the remote host
	remote, err := net.Dial("tcp", rhost)
	if err != nil {
		http.StatusText(500)
		return
	}

	websocket.Handler(func(ws *websocket.Conn) {
		go func() {
			var ok bool
			var n int
			var sbuf []byte
			//sbuf := make([]byte, BUF)
			dbuf := make([]byte, BUF)
			gotevent := make(chan []byte)
			go func() {
				buf := make([]byte, BUF)
				for {
					n, err = ws.Read(buf)
					if err != nil {
						break
					}
					gotevent <- buf[0:n]
				}
			}()
			for {
				var pbchan chan []byte
				if pb, ok := playing[rhost]; ok {
					pbchan = pb.nextevent
				}
				select {
				case sbuf, ok = <-pbchan:
					if !ok {
						// channel closed, stop playback
						delete(playing, rhost)
					}
				case sbuf = <-gotevent:
					if r, ok := recording[rhost]; ok {
						r.AddAction(string(sbuf))
					}
				}
				n = len(sbuf)
				n, err = base64.StdEncoding.Decode(dbuf, sbuf[0:n])
				if err != nil {
					break
				}
				_, err = remote.Write(dbuf[0:n])
				if err != nil {
					break
				}
			}
			remote.Close()
		}()
		func() {
			sbuf := make([]byte, BUF)
			dbuf := make([]byte, 2*BUF)
			for {
				n, err := remote.Read(sbuf)
				if err != nil {
					break
				}
				base64.StdEncoding.Encode(dbuf, sbuf[0:n])
				n = base64.StdEncoding.EncodedLen(n)
				_, err = ws.Write(dbuf[0:n])
				if err != nil {
					break
				}
			}
			ws.Close()
		}()
	}).ServeHTTP(w, r)
}
