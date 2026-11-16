package main

import (
	"errors"
	"net"
	"testing"
	"time"
)

// An unparsable line must not wedge the handler: it should keep reading and
// close the connection once the peer hangs up.
func TestCommandSocketHandleInvalidLine(t *testing.T) {
	server, client := net.Pipe()
	done := make(chan struct{})
	go func() {
		commandSocketHandle(server)
		close(done)
	}()

	client.Write([]byte("this is not a minirouter command\n"))
	client.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler blocked after invalid line")
	}
}

type fakeListener struct {
	conns chan net.Conn
	errs  chan error
}

func (f *fakeListener) Accept() (net.Conn, error) {
	select {
	case e := <-f.errs:
		return nil, e
	case c := <-f.conns:
		return c, nil
	}
}
func (f *fakeListener) Close() error   { return nil }
func (f *fakeListener) Addr() net.Addr { return nil }

// A failed Accept must not crash the server or stop it serving later clients.
func TestCommandSocketServeAcceptError(t *testing.T) {
	l := &fakeListener{conns: make(chan net.Conn, 1), errs: make(chan error, 2)}
	server, client := net.Pipe()
	l.errs <- errors.New("accept: too many open files")
	l.conns <- server

	done := make(chan struct{})
	go func() {
		commandSocketServe(l)
		close(done)
	}()

	// the client connection is served after the error: hanging up gets
	// the handler to close its end, which we observe as EOF.
	client.Close()
	l.errs <- net.ErrClosed

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop on closed listener")
	}
}
