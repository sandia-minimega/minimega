// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"io"
	"net"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	MODE_TAG = iota
	MODE_PIPE
)

func commandSocketStart(path string) {
	l, err := net.Listen("unix", path)
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

	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)

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
	var k string
	var v string

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
	err := dec.Decode(&pipe)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debug("got pipe: %v", pipe)

	reader, err := NewPlumberReader(pipe)
	if err != nil {
		log.Errorln(err)
		return
	}
	writer, err := NewPlumberWriter(pipe)
	if err != nil {
		log.Errorln(err)
		return
	}

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
