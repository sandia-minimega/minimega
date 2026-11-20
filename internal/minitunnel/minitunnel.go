// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
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
	"strings"
	"sync"
	"sync/atomic"

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

	lastFID  atomic.Int64 // last forward ID, IDs start at 1
	forwards map[int]*forward

	// reverses that the remote end acknowledged, protected by reverseLock
	reverses    []ReverseTunnel
	reverseLock sync.Mutex

	sendLock sync.Mutex

	policy Policy
	limit  *limitReader
}

// Policy limits what the remote end of a Tunnel may ask this end to do. The
// zero Policy allows every request.
type Policy struct {
	// AllowConnect, when set, reports whether a CONNECT from the remote end
	// may dial host:port from this end.
	AllowConnect func(host string, port int) bool

	// DenyForward refuses FORWARD requests, which make this end listen.
	DenyForward bool

	// MaxStreams caps the number of open streams. Zero means no cap.
	MaxStreams int

	// MaxMessage caps the encoded size of one message from the remote end.
	// Zero means no cap.
	MaxMessage int64

	// OnRefuse, when set, receives the reason for each refused request
	// instead of the log.
	OnRefuse func(reason string)
}

var errMessageTooLarge = errors.New("tunnel message too large")

// limitReader returns an error when more than limit bytes are read between
// two calls to reset.
type limitReader struct {
	r     io.Reader
	limit int64
	n     int64
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, errMessageTooLarge
	}
	if int64(len(p)) > l.n {
		p = p[:l.n]
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func (l *limitReader) reset() {
	l.n = l.limit
}

func (t *Tunnel) decode(m *tunnelMessage) error {
	if t.limit != nil {
		t.limit.reset()
	}

	return t.dec.Decode(m)
}

func (t *Tunnel) refused(reason string) {
	if t.policy.OnRefuse != nil {
		t.policy.OnRefuse(reason)
		return
	}

	log.Warn("minitunnel: %v", reason)
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

		forwards: make(map[int]*forward),
	}

	go t.mux()
	return nil
}

// Dial a listening minitunnel. Only one tunnel connection is permitted per
// transport.
func Dial(transport io.ReadWriteCloser) (*Tunnel, error) {
	return DialWithPolicy(transport, Policy{})
}

// DialWithPolicy is like Dial, but applies p to the requests that the remote
// end sends.
func DialWithPolicy(transport io.ReadWriteCloser, p Policy) (*Tunnel, error) {
	t := &Tunnel{
		transport: transport,

		enc:   gob.NewEncoder(transport),
		quit:  make(chan bool),
		chans: makeChans(),

		forwards: make(map[int]*forward),

		policy: p,
	}

	t.chans.max = p.MaxStreams

	if p.MaxMessage > 0 {
		t.limit = &limitReader{r: transport, limit: p.MaxMessage}
		t.dec = gob.NewDecoder(t.limit)
	} else {
		t.dec = gob.NewDecoder(transport)
	}

	handshake := &tunnelMessage{
		Type: HANDSHAKE,
	}

	err := t.enc.Encode(handshake)
	if err != nil {
		return nil, err
	}

	err = t.decode(handshake)
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
	in, err := t.chans.add(TID)
	if err != nil {
		return err
	}
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

	t.addReverse(ReverseTunnel{Source: source, Host: host, Dest: dest})

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
			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Errorln(err)
			}

			return
		}

		if !f.addConnection(conn) {
			conn.Close()
			return
		}

		go t.createTunnel(conn, host, dest)
	}
}

func (t *Tunnel) createTunnel(conn net.Conn, host string, dest int) {
	TID := rand.Int()
	in, err := t.chans.add(TID)
	if err != nil {
		t.refused(fmt.Sprintf("tunnel to %v:%v: %v", host, dest, err))
		conn.Close()
		return
	}

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
		if !strings.Contains(err.Error(), "use of closed network connection") {
			log.Errorln(err)
		}

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
