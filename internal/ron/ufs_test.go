// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

// blockedWriter signals the first Write and then blocks until release closes.
type blockedWriter struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (w *blockedWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release

	return len(p), nil
}

func TestUFSMessageDoesNotWaitForDisconnectSend(t *testing.T) {
	w := &blockedWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(w.release) }) }
	defer release()

	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	ufsConn, peer := net.Pipe()
	defer ufsConn.Close()
	defer peer.Close()

	c := &client{
		Client:      &Client{UUID: "client"},
		enc:         gob.NewEncoder(w),
		ufsListener: l,
		ufsConn:     ufsConn,
	}
	s := &Server{clients: map[string]*client{"client": c}}

	disconnected := make(chan error, 1)
	go func() { disconnected <- s.DisconnectUFS("client") }()

	select {
	case <-w.started:
	case <-time.After(5 * time.Second):
		t.Fatal("DisconnectUFS did not send UFS_CLOSE")
	}

	// the guest sends UFS data while the host is blocked on UFS_CLOSE
	received := make(chan struct{})
	go func() {
		c.ufsMessage(&Message{Type: MESSAGE_UFS, UfsMode: UFS_DATA, Tunnel: []byte("data")})
		close(received)
	}()

	select {
	case <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("ufsMessage blocked while DisconnectUFS was sending")
	}

	release()

	select {
	case err := <-disconnected:
		if err != nil {
			t.Fatalf("DisconnectUFS: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DisconnectUFS did not return")
	}

	c.Lock()
	gotListener, gotConn := c.ufsListener, c.ufsConn
	c.Unlock()
	if gotListener != nil || gotConn != nil {
		t.Fatalf("UFS state was not cleared: listener=%v conn=%v", gotListener, gotConn)
	}

	if _, err := l.Accept(); err == nil {
		t.Fatal("UFS listener is still open")
	}
	if _, err := peer.Read(make([]byte, 1)); err != io.EOF {
		t.Fatalf("UFS connection is still open: read error %v", err)
	}
}

func TestDisconnectUFSNotRunning(t *testing.T) {
	c := &client{Client: &Client{UUID: "client"}}
	s := &Server{clients: map[string]*client{"client": c}}

	if err := s.DisconnectUFS("client"); err == nil {
		t.Fatal("expected an error when UFS is not running")
	}
}
