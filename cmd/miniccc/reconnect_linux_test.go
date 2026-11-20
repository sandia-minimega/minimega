// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// faultConn is the guest end of a connection. After fail, Read and Write
// return the given errors, as on a vsock connection after a transport reset.
type faultConn struct {
	net.Conn

	mu                sync.Mutex
	readErr, writeErr error
}

// fail makes Read return readErr and Write return writeErr. A readErr also
// ends a pending Read.
func (c *faultConn) fail(readErr, writeErr error) {
	c.mu.Lock()
	c.readErr, c.writeErr = readErr, writeErr
	c.mu.Unlock()

	if readErr != nil {
		c.Conn.Close()
	}
}

func (c *faultConn) errs() (error, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.readErr, c.writeErr
}

func (c *faultConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if readErr, _ := c.errs(); readErr != nil {
		return 0, readErr
	}

	return n, err
}

func (c *faultConn) Write(b []byte) (int, error) {
	if _, writeErr := c.errs(); writeErr != nil {
		return 0, writeErr
	}

	return c.Conn.Write(b)
}

// logBuffer collects the log lines of a test.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

// reconnectHost is the host side of a client loop that the test runs: each
// dial gets a new connection, bound to the next UUID in bound. The dial after
// the last one stops the client loop.
type reconnectHost struct {
	guests  chan *faultConn
	claims  chan [2]string
	stopped chan struct{}
}

// startClient starts the client loop. The first refuse dials after the first
// connection fail.
func startClient(t *testing.T, bound []string, refuse int) *reconnectHost {
	t.Helper()

	saveClientFlags(t)
	*f_family, *f_serial, *f_uuid = "vsock", "", ""
	newTestClient()

	h := &reconnectHost{
		guests:  make(chan *faultConn, len(bound)),
		claims:  make(chan [2]string, len(bound)),
		stopped: make(chan struct{}),
	}

	dialer, retry := vsockDialer, firstRetryInterval
	t.Cleanup(func() {
		select {
		case <-h.stopped:
		case <-time.After(10 * time.Second):
			t.Error("client loop did not stop")
		}

		resetClientState()
		vsockDialer, firstRetryInterval = dialer, retry
	})

	firstRetryInterval = time.Millisecond

	calls, refused := 0, 0
	vsockDialer = func(int) (net.Conn, error) {
		if calls == len(bound) {
			// stop the client loop; dial releases the client lock
			close(h.stopped)
			runtime.Goexit()
		}

		if calls > 0 && refused < refuse {
			refused++
			return nil, &os.SyscallError{Syscall: "connect", Err: syscall.ECONNRESET}
		}

		guest, host := net.Pipe()
		go serveBound(host, bound[calls], h.claims)
		calls++

		g := &faultConn{Conn: guest}
		h.guests <- g

		return g, nil
	}

	go resetClient()

	return h
}

// connected waits for the next connection and its handshake as want.
func (h *reconnectHost) connected(t *testing.T, want string) *faultConn {
	t.Helper()

	var g *faultConn
	select {
	case g = <-h.guests:
	case <-h.stopped:
		t.Fatal("client loop stopped")
	case <-time.After(10 * time.Second):
		t.Fatal("client did not dial")
	}

	select {
	case <-h.claims:
	case <-time.After(10 * time.Second):
		t.Fatal("no handshake")
	}

	waitBound(t, want)

	return g
}

// captureLog collects the info log of the test.
func captureLog(t *testing.T) *logBuffer {
	t.Helper()

	name := "test-" + t.Name()
	logs := &logBuffer{}
	log.AddLogger(name, logs, log.INFO, false)
	t.Cleanup(func() { log.DelLogger(name) })

	return logs
}

// TestReconnect ends the connection with each error that a lost connection
// can give, on a read, on a write, or on both, and checks that miniccc
// connects again once and completes the handshake.
func TestReconnect(t *testing.T) {
	for _, e := range []struct {
		name string
		err  error
	}{
		{"EOF", io.EOF},
		{"ECONNRESET", syscall.ECONNRESET},
		{"ENOTCONN", syscall.ENOTCONN},
		{"EPIPE", syscall.EPIPE},
		{"ESHUTDOWN", syscall.ESHUTDOWN},
		{"ECONNABORTED", syscall.ECONNABORTED},
	} {
		for _, side := range []string{"read", "write", "both"} {
			t.Run(side+"/"+e.name, func(t *testing.T) {
				logs := captureLog(t)
				h := startClient(t, []string{testUUIDA, testUUIDB}, 0)

				first := h.connected(t, testUUIDA)

				// errors as the vsock connection, an *os.File, gives them
				opErr := func(op string) error {
					if errors.Is(e.err, io.EOF) {
						return e.err
					}
					return &os.PathError{Op: op, Path: "vsock:2:9002", Err: e.err}
				}

				var readErr, writeErr error
				if side != "write" {
					readErr = opErr("read")
				}
				if side != "read" {
					writeErr = opErr("write")
				}

				first.fail(readErr, writeErr)
				if writeErr != nil {
					// the next heartbeat writes on the failed connection
					heartbeat()
				}

				second := h.connected(t, testUUIDB)

				select {
				case <-h.stopped:
					t.Fatal("client connected again after the second handshake")
				case <-time.After(100 * time.Millisecond):
				}

				got := logs.String()
				if n := strings.Count(got, "cc connection lost: "); n != 1 {
					t.Errorf("logged %d lost connections, want 1:\n%s", n, got)
				}
				if !strings.Contains(got, e.err.Error()+"; reconnecting") {
					t.Errorf("log does not give the cause %q:\n%s", e.err, got)
				}

				// end the last connection, so that the client loop dials and stops
				second.fail(io.EOF, nil)
			})
		}
	}
}

// TestServeStalledHandshake checks that the loss of the connection ends a
// handshake that blocks because the server does not read it.
func TestServeStalledHandshake(t *testing.T) {
	saveClientFlags(t)
	*f_family, *f_serial, *f_uuid = "vsock", "", ""
	newTestClient()
	t.Cleanup(resetClientState)

	// a pipe has no buffer: the handshake blocks, because the host never reads
	guest, host := net.Pipe()
	defer host.Close()
	defer guest.Close()

	lost := make(chan error, 1)

	client.Lock()
	client.conn, client.dec = guest, gob.NewDecoder(guest)
	client.writeMu.Lock()
	client.enc, client.lost = gob.NewEncoder(guest), lost
	client.writeMu.Unlock()
	client.Unlock()

	wait := make(chan func() error, 1)
	go func() { wait <- serve() }()

	// as mux does when no message arrives in time
	connLost(lost, errTimeout)

	select {
	case w := <-wait:
		if err := w(); !errors.Is(err, errTimeout) {
			t.Errorf("got cause %v, want %v", err, errTimeout)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not return: the handshake still blocks")
	}
}

// TestReconnectRetriesDial checks that miniccc keeps dialing when the host
// refuses the first attempts after a lost connection.
func TestReconnectRetriesDial(t *testing.T) {
	h := startClient(t, []string{testUUIDA, testUUIDB}, 3)

	first := h.connected(t, testUUIDA)
	first.fail(&os.PathError{Op: "read", Path: "vsock:2:9002", Err: syscall.ENOTCONN}, nil)

	h.connected(t, testUUIDB).fail(io.EOF, nil)
}

func TestNextRetry(t *testing.T) {
	var got []time.Duration
	for wait := time.Second; len(got) < 6; wait = nextRetry(wait) {
		got = append(got, wait)
	}

	want := []time.Duration{
		time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second,
		RetryInterval, RetryInterval,
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("waits = %v, want %v", got, want)
		}
	}
}
