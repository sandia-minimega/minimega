// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"net"
	"path/filepath"
)

const (
	MODE_TAG = iota
	MODE_PIPE
)

func commandSocketStart() {
	l, err := net.Listen("unix", filepath.Join(*f_path, "miniccc"))
	if err != nil {
		log.Fatalln(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("command socket: %v", err)
		}
		log.Debugln("client connected")
		go commandSocketHandle(conn)
	}
}

func commandSocketHandle(conn net.Conn) {
	var err error
	var k string
	var v string

	defer conn.Close()

	dec := gob.NewDecoder(conn)

	var mode int
	err = dec.Decode(&mode)
	if err != nil {
		log.Errorln(err)
		return
	}

	switch mode {
	case MODE_TAG:
		tagConn(dec)
	case MODE_PIPE:
		pipeConn(enc, dec)
	default:
		log.Error("unknown mode: %v", mode)
	}
}

func tagConn(dec *gob.Decoder) {
	err := dec.Decode(&k)
	if err != nil {
		log.Errorln(err)
		return
	}
	err = dec.Decode(&v)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debug("adding key/value: %v : %v", k, v)
	addTag(k, v)
}

func pipeConn(enc *gob.Encoder, dec *gob.Decoder) {
	// finish the handshake by decoding the pipe name, then fall into the
	// i/o loop
	var pipe string
	err := dec.Deocde(&pipe)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debug("got pipe: %v", pipe)

	reader := NewPlumberReader(r.PlumbPipe)
	writer := NewplumberWriter(r.PlumbPipe)

	go func() {
		for {
			select {
			case <-reader.Done:
				return
			case line := <-reader.C:
				if err := enc.Encode(line); err != nil {
					log.Errorln(err)
					break
				}
			}
		}

	}()

	for err == nil {
		var line string
		err = dec.Decode(&line)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			break
		}

		if line != "" {
			writer <- line
		}
	}

	// stop the reader
	reader.Close()
	close(writer)
}
