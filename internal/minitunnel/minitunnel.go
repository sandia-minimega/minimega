// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const BufferSize = 32768

// tunnel message types
const (
	HANDSHAKE = iota
	CONNECT
	CLOSED
	DATA
	FORWARD
)

type Tunnel struct {
	transport io.ReadWriteCloser // underlying transport

	enc   *gob.Encoder
	dec   *gob.Decoder
	quit  chan bool // tell the message pump to quit
	chans chans

	forwardIDs chan int
	forwards   map[int]*forward

	sendLock sync.Mutex
}

type tunnelMessage struct {
	Type   int
	Ack    bool
	TID    int
	Source int
	Host   string
	Port   int
	Error  string
	Data   []byte
}

func init() {
	gob.Register(tunnelMessage{})
}

// Listen for an incoming Tunnel connection. Only one tunnel connection is
// permitted. ListenAndServe will block indefinitely until an error occurs.
func ListenAndServe(transport io.ReadWriteCloser) error {
	enc := gob.NewEncoder(transport)
	dec := gob.NewDecoder(transport)

	// wait for an incoming handshake
	var handshake tunnelMessage
	err := dec.Decode(&handshake)
	if err != nil {
		return err
	}
	if handshake.Type != HANDSHAKE {
		return fmt.Errorf("did not receive handshake: %v", handshake)
	}

	// ack the handshake
	resp := &tunnelMessage{
		Type: HANDSHAKE,
		Ack:  true,
	}
	err = enc.Encode(&resp)
	if err != nil {
		return err
	}

	t := &Tunnel{
		transport: transport,

		enc:   enc,
		dec:   dec,
		quit:  make(chan bool),
		chans: makeChans(),

		forwardIDs: make(chan int),
		forwards:   make(map[int]*forward),
	}

	// start a goroutine to generate forward IDs for us
	go func() {
		for id := 1; ; id++ {
			t.forwardIDs <- id
		}
	}()

	go t.mux()
	return nil
}

// Dial a listening minitunnel. Only one tunnel connection is permitted per
// transport.
func Dial(transport io.ReadWriteCloser) (*Tunnel, error) {
	t := &Tunnel{
		transport: transport,

		enc:   gob.NewEncoder(transport),
		dec:   gob.NewDecoder(transport),
		quit:  make(chan bool),
		chans: makeChans(),

		forwardIDs: make(chan int),
		forwards:   make(map[int]*forward),
	}

	handshake := &tunnelMessage{
		Type: HANDSHAKE,
	}

	err := t.enc.Encode(handshake)
	if err != nil {
		return nil, err
	}

	err = t.dec.Decode(handshake)
	if err != nil {
		return nil, err
	}

	if !handshake.Ack {
		return nil, fmt.Errorf("did not receive handshake ack: %v", handshake)
	}

	// start a goroutine to generate forward IDs for us
	go func() {
		for id := 1; ; id++ {
			t.forwardIDs <- id
		}
	}()

	// start the message mux
	go t.mux()

	return t, nil
}

// Forward a local port to a remote host and destination port
func (t *Tunnel) Forward(source int, host string, dest int) error {
	log.Info("forward %v -> %v:%v", source, host, dest)

	// start a goroutine that listens on the source port, and on every
	// accept, opens a new tunnel over the transport.
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", source))
	if err != nil {
		return err
	}

	go t.forward(ln, source, host, dest)
	return nil
}

// Create a reverse forwarded port from a source port on the remote end,
// destination host, and destination port on the local end.
func (t *Tunnel) Reverse(source int, host string, dest int) error {
	// create a temporary TID registration in order to get an ACK back
	TID := rand.Int()
	in := t.chans.add(TID)
	defer t.chans.remove(TID)

	// send a message to invoke Forward() on the remote side
	m := &tunnelMessage{
		Type:   FORWARD,
		TID:    TID,
		Source: source,
		Host:   host,
		Port:   dest,
	}
	if err := t.sendMessage(m); err != nil {
		return err
	}

	m = <-in
	if m == nil {
		return errors.New("tunnel terminating")
	} else if m.Error != "" {
		return errors.New(m.Error)
	}

	return nil
}

// listen on source port and start new remote connections for every Accept()
func (t *Tunnel) forward(ln net.Listener, source int, host string, dest int) {
	f := t.newForward(ln, source, host, dest)

	t.sendLock.Lock()
	t.forwards[f.fid] = f
	t.sendLock.Unlock()

	go func() {
		<-t.quit
		f.close()

		t.sendLock.Lock()
		delete(t.forwards, f.fid)
		t.sendLock.Unlock()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorln(err)
			return
		}

		f.addConnection(conn)
		go t.createTunnel(conn, host, dest)
	}
}

func (t *Tunnel) createTunnel(conn net.Conn, host string, dest int) {
	TID := rand.Int()
	in := t.chans.add(TID)

	log.Debug("create tunnel for %v:%v: %v", host, dest, TID)

	m := &tunnelMessage{
		Type: CONNECT,
		Host: host,
		Port: dest,
		TID:  TID,
	}

	if err := t.sendMessage(m); err != nil {
		log.Errorln(err)
		return
	}

	t.transfer(in, conn, TID)

	log.Debug("tunnel quit for %v:%v: %v", host, dest, TID)
}

func (t *Tunnel) transfer(in chan *tunnelMessage, conn net.Conn, TID int) {
	defer t.chans.remove(TID)

	// begin forwarding until an error occurs
	go func() {
		defer conn.Close()

		for m := range in {
			if m.Type == CLOSED {
				if m.Error != "" {
					log.Errorln(m.Error)
					break
				}
			}

			if _, err := conn.Write(m.Data); err != nil {
				log.Errorln(err)
				conn.Close()

				m := &tunnelMessage{
					Type:  CLOSED,
					TID:   TID,
					Error: err.Error(),
				}
				if err := t.sendMessage(m); err != nil {
					log.Errorln(err)
				}
				return
			}
		}
	}()

	var n int
	var err error

	buf := make([]byte, BufferSize)

	for err == nil {
		n, err = conn.Read(buf)

		if err == nil {
			m := &tunnelMessage{
				Type: DATA,
				TID:  TID,
				Data: buf[:n],
			}

			err = t.sendMessage(m)
		}
	}

	if err != io.EOF {
		log.Errorln(err)

		m := &tunnelMessage{
			Type:  CLOSED,
			TID:   TID,
			Error: err.Error(),
		}

		if err := t.sendMessage(m); err != nil {
			log.Errorln(err)
		}
	}
}
