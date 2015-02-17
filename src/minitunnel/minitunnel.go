// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minitunnel

import (
	"encoding/gob"
	"fmt"
	"io"
	"math/rand"
	log "minilog"
	"net"
)

const (
	BUFFER_SIZE = 32768
)

const (
	HANDSHAKE = iota
	CONNECT
	CLOSED
	DATA
	FORWARD
)

type Tunnel struct {
	transport io.ReadWriter
	enc       *gob.Encoder
	dec       *gob.Decoder
	out       chan *tunnelMessage
	tids      map[int32]chan *tunnelMessage
}

type tunnelMessage struct {
	Type   int
	Ack    bool
	TID    int32
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
func ListenAndServe(transport io.ReadWriter) error {
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
		enc:       enc,
		dec:       dec,
		out:       make(chan *tunnelMessage, 1024),
		tids:      make(map[int32]chan *tunnelMessage, 1024),
	}

	go t.mux()

	return nil
}

// Dial a listening minitunnel. Only one tunnel connection is permitted per
// transport.
func Dial(transport io.ReadWriter) (*Tunnel, error) {
	t := &Tunnel{
		transport: transport,
		enc:       gob.NewEncoder(transport),
		dec:       gob.NewDecoder(transport),
		out:       make(chan *tunnelMessage, 1024),
		tids:      make(map[int32]chan *tunnelMessage, 1024),
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

	// start the message mux
	go t.mux()

	return t, nil
}

func (t *Tunnel) mux() {
	go func() {
		for {
			m := <-t.out
			err := t.enc.Encode(m)
			if err != nil {
				log.Errorln(err)
			}
		}
	}()

	for {
		var m tunnelMessage
		err := t.dec.Decode(&m)
		if err != nil {
			log.Errorln(err)
		}

		// create new sessions if necessary
		if m.Type == CONNECT {
			go t.handleRemote(&m)
		} else if m.Type == FORWARD {
			go t.handleReverse(&m)
		} else if c, ok := t.tids[m.TID]; ok {
			// route the message to the handler by TID
			c <- &m
		} else {
			log.Info("invalid TID: %v", m.TID)
		}
	}
}

func (t *Tunnel) handleReverse(m *tunnelMessage) {
	resp := &tunnelMessage{
		Type: DATA,
		TID:  m.TID,
		Ack:  true,
	}
	err := t.Forward(m.Source, m.Host, m.Port)
	if err != nil {
		resp.Error = err.Error()
	}
	t.out <- resp
}

// Forward a local port to a remote host and destination port
func (t *Tunnel) Forward(source int, host string, dest int) error {
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
	TID := rand.Int31()
	in := t.registerTID(TID)
	defer t.unregisterTID(TID)

	// send a message to invoke Forward() on the remote side
	t.out <- &tunnelMessage{
		Type:   FORWARD,
		TID:    TID,
		Source: source,
		Host:   host,
		Port:   dest,
	}

	m := <-in

	if m.Error != "" {
		return fmt.Errorf("%v", m.Error)
	}

	return nil
}

// listen on source port and start new remote connections for every Accept()
func (t *Tunnel) forward(ln net.Listener, source int, host string, dest int) {
	defer ln.Close()
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorln(err)
			continue
		}

		go t.handleTunnel(conn, host, dest)
	}
}

// register a transaction ID, adding a return channel to the mux
func (t *Tunnel) registerTID(TID int32) chan *tunnelMessage {
	if _, ok := t.tids[TID]; ok {
		log.Fatalln(fmt.Sprintf("tid %v already exists!", TID))
	}
	c := make(chan *tunnelMessage, 1024)
	t.tids[TID] = c
	return c
}

func (t *Tunnel) unregisterTID(TID int32) {
	if _, ok := t.tids[TID]; ok {
		delete(t.tids, TID)
	}
}

func (t *Tunnel) handleRemote(m *tunnelMessage) {
	host := m.Host
	port := m.Port
	TID := m.TID

	in := t.registerTID(TID)

	// attempt to connect to the host/port
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:%v", host, port))
	if err != nil {
		log.Errorln(err)
		t.out <- &tunnelMessage{
			Type:  CLOSED,
			TID:   TID,
			Error: err.Error(),
		}
		t.unregisterTID(TID)
		return
	}

	t.handle(in, conn, TID)
}

func (t *Tunnel) handleTunnel(conn net.Conn, host string, dest int) {
	TID := rand.Int31()
	in := t.registerTID(TID)

	m := &tunnelMessage{
		Type: CONNECT,
		Host: host,
		Port: dest,
		TID:  TID,
	}

	t.out <- m

	t.handle(in, conn, TID)
}

func (t *Tunnel) handle(in chan *tunnelMessage, conn net.Conn, TID int32) {
	// begin forwarding until an error occurs
	go func() {
		for {
			m := <-in
			if m.Type == CLOSED {
				if m.Error != "" {
					log.Errorln(m.Error)
					conn.Close()
					break
				}
			}
			_, err := conn.Write(m.Data)
			if err != nil {
				log.Errorln(err)
				conn.Close()
				t.out <- &tunnelMessage{
					Type:  CLOSED,
					TID:   TID,
					Error: err.Error(),
				}
				break
			}
		}
	}()

	for {
		var buf = make([]byte, BUFFER_SIZE)
		n, err := conn.Read(buf)
		if err != nil {
			conn.Close()
			closeMessage := &tunnelMessage{
				Type: CLOSED,
				TID:  TID,
			}
			if err != io.EOF {
				log.Errorln(err)
				closeMessage.Error = err.Error()
			}
			t.out <- closeMessage
			t.unregisterTID(TID)
			break
		}
		m := &tunnelMessage{
			Type: DATA,
			TID:  TID,
			Data: buf[:n],
		}
		t.out <- m
	}
}
