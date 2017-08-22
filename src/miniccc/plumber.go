// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"encoding/gob"
	"io"
	log "minilog"
	"miniplumber"
	"net"
	"os"
	"path/filepath"
	"ron"
	"sync"
)

var (
	plumber     *miniplumber.Plumber
	readerCount map[string]int
	writerCount map[string]int
	plumberLock sync.Mutex
)

func init() {
	plumber = miniplumber.New(nil)
	readerCount = make(map[string]int)
	writerCount = make(map[string]int)
}

func pipeMessage(m *ron.Message) {
	plumberLock.Lock()
	defer plumberLock.Unlock()

	// incoming messages can be data writes or read closers
	switch m.PipeMode {
	case ron.PIPE_DATA:
		plumber.Write(m.Pipe, m.PipeData)
	case ron.PIPE_CLOSE_READER:
		err := plumber.PipeDelete(m.Pipe)
		if err != nil {
			log.Errorln(err)
		}
	default:
		log.Error("invalid message type: %v", m.PipeMode)
	}
}

func NewPlumberReader(pipe string) (*miniplumber.Reader, error) {
	plumberLock.Lock()
	defer plumberLock.Unlock()

	r := plumber.NewReader(pipe)

	m := &ron.Message{
		Type:     ron.MESSAGE_PIPE,
		Pipe:     pipe,
		PipeMode: ron.PIPE_NEW_READER,
	}

	if err := sendMessage(m); err != nil {
		r.Close()
		return nil, err
	}

	readerCount[pipe]++
	go func() {
		<-r.Done
		plumberLock.Lock()
		defer plumberLock.Unlock()
		readerCount[pipe]--
		if readerCount[pipe] == 0 {
			delete(readerCount, pipe)
			closeUpstreamReader(pipe)
		}
	}()

	return r, nil
}

func NewPlumberWriter(pipe string) (chan<- string, error) {
	plumberLock.Lock()
	defer plumberLock.Unlock()

	w := plumber.NewWriter(pipe)

	m := &ron.Message{
		Type:     ron.MESSAGE_PIPE,
		Pipe:     pipe,
		PipeMode: ron.PIPE_NEW_WRITER,
	}

	if err := sendMessage(m); err != nil {
		close(w)
		return nil, err
	}

	writerCount[pipe]++

	// we have to intercept writes in order to forward them up to minimega,
	// so we do an indirect just like miniplumber itself does internally.
	ww := make(chan string)
	go func() {
		for v := range ww {
			err := pipeForward(pipe, v)
			if err != nil {
				log.Errorln(err)
			}
			w <- v
		}
		close(w)

		plumberLock.Lock()
		defer plumberLock.Unlock()
		writerCount[pipe]--
		if writerCount[pipe] == 0 {
			delete(writerCount, pipe)
			closeUpstreamWriter(pipe)
		}
	}()

	return ww, nil
}

func closeUpstreamReader(pipe string) {
	m := &ron.Message{
		Type:     ron.MESSAGE_PIPE,
		Pipe:     pipe,
		PipeMode: ron.PIPE_CLOSE_READER,
	}

	if err := sendMessage(m); err != nil {
		log.Errorln(err)
	}
}

func closeUpstreamWriter(pipe string) {
	m := &ron.Message{
		Type:     ron.MESSAGE_PIPE,
		Pipe:     pipe,
		PipeMode: ron.PIPE_CLOSE_WRITER,
	}

	if err := sendMessage(m); err != nil {
		log.Errorln(err)
	}
}

func pipeForward(pipe, data string) error {
	m := &ron.Message{
		Type:     ron.MESSAGE_PIPE,
		Pipe:     pipe,
		PipeMode: ron.PIPE_DATA,
		PipeData: data,
	}

	if err := sendMessage(m); err != nil {
		return err
	}

	return nil
}

func pipeHandler(pipe string) {
	host := filepath.Join(*f_path, "miniccc")

	conn, err := net.Dial("unix", host)
	if err != nil {
		log.Errorln(err)
		return
	}

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	err = enc.Encode(MODE_PIPE)
	if err != nil {
		log.Fatalln(err)
	}

	// encode the pipe name
	err = enc.Encode(pipe)
	if err != nil {
		log.Fatalln(err)
	}

	// from here we just encode/decode on the pipe

	go func() {
		var buf string
		for {
			err := dec.Decode(&buf)
			if err != nil {
				if err != io.EOF {
					log.Fatal("local command gob decode: %v", err)
				}
				os.Exit(0)
			}

			_, err = os.Stdout.WriteString(buf)
			if err != nil {
				log.Fatal("write: %v", err)
			}
		}
	}()

	for {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			err := enc.Encode(scanner.Text() + "\n")
			if err != nil {
				log.Fatal("local command gob encode: %v", err)
			}
		}

		// scanners don't return EOF errors
		if err := scanner.Err(); err != nil {
			log.Fatal("read: %v", err)
		}

		log.Debugln("client closed stdin")
		os.Exit(0)
	}
}
