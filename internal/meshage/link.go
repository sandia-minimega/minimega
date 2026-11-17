// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package meshage

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Class is the type of traffic that a message carries. A node has one link
// for each class to each peer, so that one type of traffic does not wait for
// another.
type Class int

const (
	// Control carries the mesh state, commands, and small responses.
	Control Class = iota
	// Bulk carries files, VM launches, and large responses.
	Bulk
	// Log carries forwarded log lines.
	Log

	numClasses
)

func (c Class) String() string {
	switch c {
	case Control:
		return "control"
	case Bulk:
		return "bulk"
	case Log:
		return "log"
	}

	return fmt.Sprintf("Class(%d)", int(c))
}

func (c Class) valid() bool {
	return c >= 0 && c < numClasses
}

const (
	// WINDOW is the maximum number of messages on a link that wait for an
	// ACK.
	WINDOW = 64

	// PROTOCOL is the version of the link handshake. Peers must use the same
	// version.
	PROTOCOL = 2
)

// writeChunk is the size of one write. Each write gets its own deadline.
const writeChunk = 64 << 10

var (
	errTimeout    = errors.New("timeout")
	errLinkClosed = errors.New("link closed")
)

// hello is the first value that each side of a new connection sends.
type hello struct {
	Protocol  int
	Name      string
	Version   string
	Solicited bool   // sent by the side that accepts the connection
	Class     Class  // sent by the side that dials the connection
	Session   uint64 // sent by the side that dials, the same on all links of a peer
}

// readHello decodes the hello of the peer. A peer that runs an older version
// of meshage sends a string first, which gob cannot decode into a hello.
func readHello(dec *gob.Decoder) (hello, error) {
	var h hello
	if err := dec.Decode(&h); err != nil {
		if err != io.EOF && strings.Contains(err.Error(), "type mismatch") {
			err = fmt.Errorf("peer uses an older mesh protocol: %v", err)
		}
		return h, err
	}
	if h.Protocol != PROTOCOL {
		return h, fmt.Errorf("peer uses mesh protocol %d, want %d", h.Protocol, PROTOCOL)
	}

	return h, nil
}

// deadlineWriter writes in chunks and sets the write deadline before each
// chunk, so that a large message fails only when the connection stops.
type deadlineWriter struct {
	conn    net.Conn
	timeout time.Duration
}

func (w *deadlineWriter) Write(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		end := min(n+writeChunk, len(p))

		if err := w.conn.SetWriteDeadline(time.Now().Add(w.timeout)); err != nil {
			return n, err
		}

		m, err := w.conn.Write(p[n:end])
		n += m
		if err != nil {
			return n, err
		}
	}

	return n, nil
}

// progressReader sets the read deadline before each read, so that a large
// message fails only when the connection stops. It records the time of the
// last read that got data.
type progressReader struct {
	conn net.Conn

	// timeout returns the read deadline. Set it before the first read.
	timeout func() time.Duration

	last atomic.Int64 // UnixNano of the last read that got data
}

func (r *progressReader) Read(p []byte) (int, error) {
	if err := r.conn.SetReadDeadline(time.Now().Add(r.timeout())); err != nil {
		return 0, err
	}

	n, err := r.conn.Read(p)
	if n > 0 {
		r.last.Store(time.Now().UnixNano())
	}

	return n, err
}

// lastRead returns the time of the last read that got data, or the zero time.
func (r *progressReader) lastRead() time.Time {
	if v := r.last.Load(); v != 0 {
		return time.Unix(0, v)
	}

	return time.Time{}
}

// pending is a message that waits for its ACK.
type pending struct {
	m     *Message
	errc  chan error
	timer *time.Timer
}

// link is one connection to a peer. Senders queue their messages and a
// writer goroutine encodes them, so that up to WINDOW messages wait for an
// ACK at the same time. The reader of the link queues the ACKs for the
// writer, so it does not wait for a write to finish.
type link struct {
	class Class
	conn  net.Conn
	enc   *gob.Encoder
	dec   *gob.Decoder
	in    *progressReader // the reader of dec

	// timeout returns how long a message can wait for its ACK while the link
	// receives nothing.
	timeout func() time.Duration

	// keepalive returns how long the link can be idle before the writer
	// sends a keepalive.
	keepalive func() time.Duration

	slots chan struct{} // one for each message that a sender queued
	out   chan *pending
	acks  chan uint64
	done  chan struct{}

	mu      sync.Mutex
	seq     uint64
	waiting map[uint64]*pending
	err     error
	once    sync.Once
}

func newLink(class Class, conn net.Conn, enc *gob.Encoder, dec *gob.Decoder, in *progressReader, timeout, keepalive func() time.Duration) *link {
	l := &link{
		class:     class,
		conn:      conn,
		enc:       enc,
		dec:       dec,
		in:        in,
		timeout:   timeout,
		keepalive: keepalive,
		slots:     make(chan struct{}, WINDOW),
		out:       make(chan *pending),
		acks:      make(chan uint64, 2*WINDOW),
		done:      make(chan struct{}),
		waiting:   make(map[uint64]*pending),
	}

	go l.writer()

	return l
}

// send queues m and waits for the ACK of the peer.
func (l *link) send(m *Message) error {
	select {
	case l.slots <- struct{}{}:
	case <-l.done:
		return l.closeErr()
	}
	defer func() { <-l.slots }()

	p := &pending{m: m, errc: make(chan error, 1)}

	select {
	case l.out <- p:
	case <-l.done:
		return l.closeErr()
	}

	select {
	case err := <-p.errc:
		return err
	case <-l.done:
		// the ACK can arrive at the same time as the close
		select {
		case err := <-p.errc:
			return err
		default:
			return l.closeErr()
		}
	}
}

// writer encodes the queued ACKs and messages. ACKs go first, so that the
// peer does not wait for them behind a large message. When the link is idle,
// the writer sends an ACK with sequence number 0 as a keepalive, so that the
// peer can use a read deadline on each link.
func (l *link) writer() {
	idle := time.NewTimer(l.keepalive())
	defer idle.Stop()

	for {
		var ack *Message
		var p *pending

		select {
		case seq := <-l.acks:
			ack = &Message{Command: ACK, Seq: seq}
		case <-l.done:
			return
		default:
			select {
			case seq := <-l.acks:
				ack = &Message{Command: ACK, Seq: seq}
			case p = <-l.out:
			case <-idle.C:
				ack = &Message{Command: ACK}
			case <-l.done:
				return
			}
		}

		idle.Reset(l.keepalive())

		if ack != nil {
			if err := l.write(ack); err != nil {
				l.close(fmt.Errorf("write ACK: %w", err))
				return
			}
			continue
		}

		// Copy the message, because a flood sends the same message on more
		// than one link.
		m := *p.m

		l.mu.Lock()
		l.seq++
		m.Seq = l.seq
		l.waiting[m.Seq] = p
		l.mu.Unlock()

		if err := l.write(&m); err != nil {
			l.close(err)
			return
		}

		seq := m.Seq
		l.mu.Lock()
		if _, ok := l.waiting[seq]; ok {
			p.timer = time.AfterFunc(l.timeout(), func() { l.ackTimeout(seq) })
		}
		l.mu.Unlock()
	}
}

// ackTimeout closes the link when the message seq has no ACK and the link
// received nothing for l.timeout(). The peer sends its ACKs between messages, so
// a link that receives a large message waits longer for an ACK.
func (l *link) ackTimeout(seq uint64) {
	l.mu.Lock()
	// the ACK can arrive at the same time as the timeout
	p, ok := l.waiting[seq]
	if ok {
		if wait := l.timeout() - time.Since(l.in.lastRead()); wait > 0 {
			p.timer.Reset(wait)
			l.mu.Unlock()
			return
		}
	}
	l.mu.Unlock()

	if ok {
		l.close(fmt.Errorf("%w: no ACK for message %d", errTimeout, seq))
	}
}

// write encodes m. The encoder of a link writes through a deadlineWriter.
func (l *link) write(m *Message) error {
	return l.enc.Encode(m)
}

// acked completes the message with the sequence number seq.
func (l *link) acked(seq uint64) {
	l.mu.Lock()
	p, ok := l.waiting[seq]
	if ok {
		delete(l.waiting, seq)
		if p.timer != nil {
			p.timer.Stop()
		}
	}
	l.mu.Unlock()

	if ok {
		p.errc <- nil
	} else {
		log.Debug("%v link: ACK for unknown message %d", l.class, seq)
	}
}

// ack queues an ACK for the message with the sequence number seq. The peer
// has at most WINDOW messages that wait for an ACK, so the queue has space.
func (l *link) ack(seq uint64) error {
	select {
	case l.acks <- seq:
		return nil
	case <-l.done:
		return l.closeErr()
	}
}

// close closes the link and completes all the messages that wait for an ACK
// with err.
func (l *link) close(err error) {
	l.once.Do(func() {
		l.mu.Lock()
		l.err = err
		waiting := l.waiting
		l.waiting = map[uint64]*pending{}
		l.mu.Unlock()

		close(l.done)
		l.conn.Close()

		for _, p := range waiting {
			if p.timer != nil {
				p.timer.Stop()
			}
			p.errc <- err
		}
	})
}

func (l *link) closed() bool {
	select {
	case <-l.done:
		return true
	default:
		return false
	}
}

func (l *link) closeErr() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.err == nil {
		return errLinkClosed
	}
	return l.err
}
