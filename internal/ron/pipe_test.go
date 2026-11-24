// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"net"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
)

const pipeTestTimeout = 10 * time.Second

// newPipeClient returns the server's side of a client in namespace "ns", and
// the messages that the server sends to it.
func newPipeClient(t *testing.T) (*client, <-chan *Message) {
	t.Helper()

	serverConn, guestConn := net.Pipe()

	c := &client{
		Client:      &Client{UUID: "client"},
		Namespace:   "ns",
		conn:        serverConn,
		enc:         gob.NewEncoder(serverConn),
		pipeReaders: make(map[string]*miniplumber.Reader),
		pipeWriters: make(map[string]chan<- string),
	}

	sent := make(chan *Message, 100)
	done := make(chan struct{})

	go func() {
		defer close(done)

		dec := gob.NewDecoder(guestConn)
		for {
			m := new(Message)
			if err := dec.Decode(m); err != nil {
				return
			}
			sent <- m
		}
	}()

	t.Cleanup(func() {
		c.pipeLock.Lock()
		for _, r := range c.pipeReaders {
			r.Close()
		}
		c.pipeLock.Unlock()

		serverConn.Close()
		guestConn.Close()
		<-done
	})

	return c, sent
}

// pipeMsg is a pipe message from the guest, which names a pipe without its
// namespace.
func pipeMsg(mode int) *Message {
	return &Message{Type: MESSAGE_PIPE, Pipe: "foo", PipeMode: mode}
}

// expectPipeMsg waits for the next message to the guest, which must have the
// given mode and data.
func expectPipeMsg(t *testing.T, sent <-chan *Message, mode int, data string) {
	t.Helper()

	select {
	case m := <-sent:
		if m.Type != MESSAGE_PIPE || m.Pipe != "foo" || m.PipeMode != mode || m.PipeData != data {
			t.Fatalf("guest was sent pipe %q, mode %v, data %q; want pipe \"foo\", mode %v, data %q",
				m.Pipe, m.PipeMode, m.PipeData, mode, data)
		}
	case <-time.After(pipeTestTimeout):
		t.Fatalf("guest was sent nothing, want mode %v, data %q", mode, data)
	}
}

// expectReaders waits until the pipe has n readers. A closed reader leaves
// its pipe a moment later.
func expectReaders(t *testing.T, plumber *miniplumber.Plumber, n int) {
	t.Helper()

	var got int
	for deadline := time.Now().Add(pipeTestTimeout); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		got = 0
		for _, p := range plumber.Pipes() {
			if p.Name() == "ns//foo" {
				got = p.NumReaders()
			}
		}
		if got == n {
			return
		}
	}
	t.Fatalf("pipe has %d readers, want %d", got, n)
}

// A guest that closes its reader on a pipe and makes a new one right away, as
// two runs of `miniccc -pipe` in a row do, has a reader afterwards, and is not
// told to close it.
func TestPipeReaderAttachAgain(t *testing.T) {
	plumber := miniplumber.New(nil)
	c, sent := newPipeClient(t)

	for i := 0; i < 20; i++ {
		c.pipeHandler(plumber, pipeMsg(PIPE_NEW_READER))
		c.pipeHandler(plumber, pipeMsg(PIPE_CLOSE_READER))
	}
	c.pipeHandler(plumber, pipeMsg(PIPE_NEW_READER))
	expectReaders(t, plumber, 1)

	// the first message to the guest is the data, not a close for one of the
	// readers that the guest closed itself
	plumber.Write("ns//foo", "hello")
	expectPipeMsg(t, sent, PIPE_DATA, "hello\n")

	c.pipeHandler(plumber, pipeMsg(PIPE_CLOSE_READER))
	expectReaders(t, plumber, 0)

	select {
	case m := <-sent:
		t.Errorf("guest was sent mode %v, data %q after it closed its reader, want nothing", m.PipeMode, m.PipeData)
	case <-time.After(50 * time.Millisecond):
	}
}

// When minimega deletes a pipe, the guest is told that its reader is closed,
// and can attach a new one.
func TestPipeReaderClosedByPlumber(t *testing.T) {
	plumber := miniplumber.New(nil)
	c, sent := newPipeClient(t)

	c.pipeHandler(plumber, pipeMsg(PIPE_NEW_READER))
	expectReaders(t, plumber, 1)

	if err := plumber.PipeDelete("ns//foo"); err != nil {
		t.Fatal(err)
	}
	expectPipeMsg(t, sent, PIPE_CLOSE_READER, "")

	// the guest answers with a close of its own, for a reader that is gone
	c.pipeHandler(plumber, pipeMsg(PIPE_CLOSE_READER))

	c.pipeHandler(plumber, pipeMsg(PIPE_NEW_READER))
	expectReaders(t, plumber, 1)

	plumber.Write("ns//foo", "again")
	expectPipeMsg(t, sent, PIPE_DATA, "again\n")
}
