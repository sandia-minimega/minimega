package main

import (
	"encoding/gob"
	"net"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

// After a command with stdin=<pipe> exits, miniccc must tell the server it is
// no longer reading the pipe; otherwise the server keeps routing data to a
// reader nobody drains.
func TestRunCommandClosesStdinReaderOnExit(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client.enc = gob.NewEncoder(clientConn)
	defer func() { client.enc = nil }()

	msgs := make(chan *ron.Message, 16)
	go func() {
		dec := gob.NewDecoder(serverConn)
		for {
			var m ron.Message
			if err := dec.Decode(&m); err != nil {
				return
			}
			msgs <- &m
		}
	}()

	runCommand(&ron.Command{Command: []string{"true"}, Stdin: "foo"})

	timeout := time.After(2 * time.Second)
	for {
		select {
		case m := <-msgs:
			if m.Pipe == "foo" && m.PipeMode == ron.PIPE_CLOSE_READER {
				return
			}
		case <-timeout:
			t.Fatal("no PIPE_CLOSE_READER sent after command exited")
		}
	}
}
