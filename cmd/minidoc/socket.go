// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package socket implements an WebSocket-based playground backend.
// Clients connect to a websocket handler and send run/kill commands, and
// the server sends the output and exit status of the running processes.
// Multiple clients running multiple processes may be served concurrently.
// The wire format is JSON and is described by the Message type.
//
// This will not run on App Engine as WebSockets are not supported there.
package main

import (
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"golang.org/x/net/websocket"
)

const (
	// The maximum number of messages to send per session (avoid flooding).
	msgLimit = 1000

	// Batch messages sent in this interval and send as a single message.
	msgDelay = 10 * time.Millisecond
)

// Message is the wire format for the websocket connection to the browser.
// It is used for both sending output messages and receiving commands, as
// distinguished by the Kind field.
type Message struct {
	Id   string // client-provided unique id for the process
	Kind string // in: "run", "kill" out: "stdout", "stderr", "end"
	Body string
}

// NewHandler returns a websocket server which checks the origin of requests.
func NewSocketHandler() websocket.Server {
	return websocket.Server{
		Handler: websocket.Handler(socketHandler),
	}
}

func socketHandler(c *websocket.Conn) {
	in, out := make(chan *Message), make(chan *Message)
	errc := make(chan error, 1)

	// Decode messages from client and send to the in channel.
	go func() {
		dec := json.NewDecoder(c)
		for {
			var m Message
			if err := dec.Decode(&m); err != nil {
				errc <- err
				return
			}
			in <- &m
		}
	}()

	// Receive messages from the out channel and encode to the client.
	go func() {
		enc := json.NewEncoder(c)
		for m := range out {
			if err := enc.Encode(m); err != nil {
				errc <- err
				return
			}
		}
	}()

	// open a connection to minimega and handle the request
	megaconns := make(map[string]*megaconn)
	for {
		select {
		case m := <-in:
			log.Debugln("running snippet from:", c.Request().RemoteAddr)
			lOut := limiter(in, out)
			megaconns[m.Id] = runMega(m.Id, m.Body, lOut)
		case err := <-errc:
			if err != io.EOF {
				// A encode or decode has failed; bail.
				log.Errorln(err)
			}
			return
		}
	}
}

// process represents a running process.
type megaconn struct {
	id   string
	out  chan<- *Message
	body string
}

func runMega(id, body string, out chan<- *Message) *megaconn {
	m := &megaconn{
		id:   id,
		out:  out,
		body: body,
	}

	go m.start()
	return m
}

func (m *megaconn) start() {
	log.Debug("got body: %v", m.body)
	lines := strings.Split(m.body, "\n")
	for _, v := range lines {
		resp := sendCommand(v)
		mess := &Message{
			Id: m.id,
		}
		mess.Kind = "stdout"
		mess.Body = resp
		log.Debug("generated message: %v", mess)
		m.out <- mess
	}

	mess := &Message{
		Id:   m.id,
		Kind: "end",
	}
	time.AfterFunc(msgDelay, func() { m.out <- mess })
}

// messageWriter is an io.Writer that converts all writes to Message sends on
// the out channel with the specified id and kind.
type messageWriter struct {
	id, kind string
	out      chan<- *Message

	mu   sync.Mutex
	buf  []byte
	send *time.Timer
}

// limiter returns a channel that wraps dest. Messages sent to the channel are
// sent to dest. After msgLimit Messages have been passed on, a "kill" Message
// is sent to the kill channel, and only "end" messages are passed.
func limiter(kill chan<- *Message, dest chan<- *Message) chan<- *Message {
	ch := make(chan *Message)
	go func() {
		n := 0
		for m := range ch {
			switch {
			case n < msgLimit || m.Kind == "end":
				dest <- m
				if m.Kind == "end" {
					return
				}
			case n == msgLimit:
				// process produced too much output. Kill it.
				kill <- &Message{Id: m.Id, Kind: "kill"}
			}
			n++
		}
	}()
	return ch
}
