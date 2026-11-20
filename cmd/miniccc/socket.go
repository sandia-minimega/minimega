// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
	"syscall"

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
		reader.Close()
		log.Errorln(err)
		return
	}

	// Send lines written to the pipe to the client, until the reader is
	// closed or the client is gone.
	sent := make(chan struct{})
	go func() {
		defer close(sent)
		// stop the plumber from delivering to a client that is gone
		defer reader.Close()

		for {
			select {
			case <-reader.Done:
				return
			case line, ok := <-reader.C:
				if !ok {
					return
				}

				if err := enc.Encode(line); err != nil {
					if !isClientGone(err) {
						log.Errorln(err)
					}
					return
				}
			}
		}
	}()

	for {
		var line string
		err = dec.Decode(&line)
		if err != nil {
			if err != io.EOF && !isClientGone(err) {
				log.Errorln(err)
			}
			break
		}

		if line != "" {
			writer <- line
		}
	}

	// stop the reader, and wait for it so that nothing is sent to the client
	// after the caller closes the connection
	reader.Close()
	<-sent

	close(writer)
}

// isClientGone reports whether err is how a write to, or a read from, the
// command socket fails when the client disconnected early: EPIPE on a write
// and, if the client left data it was sent unread, ECONNRESET on a read.
func isClientGone(err error) bool {
	return errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET)
}
