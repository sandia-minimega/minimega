// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel

import (
	"encoding/gob"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRemote struct {
	enc *gob.Encoder
	dec *gob.Decoder
}

// startPolicyTunnel dials a tunnel with policy p and returns it with a fake
// remote end that the test drives with raw tunnel messages.
func startPolicyTunnel(t *testing.T, p Policy) (*Tunnel, *fakeRemote) {
	t.Helper()

	local, remote := net.Pipe()
	t.Cleanup(func() {
		local.Close()
		remote.Close()
	})

	r := &fakeRemote{enc: gob.NewEncoder(remote), dec: gob.NewDecoder(remote)}

	errs := make(chan error, 1)
	go func() {
		var hs tunnelMessage
		if err := r.dec.Decode(&hs); err != nil {
			errs <- err
			return
		}
		errs <- r.enc.Encode(&tunnelMessage{Type: HANDSHAKE, Ack: true})
	}()

	tun, err := DialWithPolicy(local, p)
	if err != nil {
		t.Fatalf("DialWithPolicy: %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("remote handshake: %v", err)
	}

	return tun, r
}

func (r *fakeRemote) send(t *testing.T, m *tunnelMessage) {
	t.Helper()

	if err := r.enc.Encode(m); err != nil {
		t.Fatalf("send: %v", err)
	}
}

func (r *fakeRemote) recv(t *testing.T) *tunnelMessage {
	t.Helper()

	res := make(chan *tunnelMessage, 1)
	go func() {
		m := new(tunnelMessage)
		if err := r.dec.Decode(m); err != nil {
			res <- nil
			return
		}
		res <- m
	}()

	select {
	case m := <-res:
		if m == nil {
			t.Fatal("tunnel closed")
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for tunnel message")
	}

	return nil
}

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

func listen(t *testing.T) (net.Listener, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	return ln, ln.Addr().(*net.TCPAddr).Port
}

// accepted reports whether ln accepts a connection within d.
func accepted(t *testing.T, ln net.Listener, d time.Duration) bool {
	t.Helper()

	ln.(*net.TCPListener).SetDeadline(time.Now().Add(d))
	conn, err := ln.Accept()
	if err != nil {
		return false
	}
	t.Cleanup(func() { conn.Close() })

	return true
}

func TestPolicyDenyForward(t *testing.T) {
	var refused int32
	_, r := startPolicyTunnel(t, Policy{
		DenyForward: true,
		OnRefuse:    func(string) { atomic.AddInt32(&refused, 1) },
	})

	port := freePort(t)
	r.send(t, &tunnelMessage{Type: FORWARD, TID: 7, Source: port, Host: "127.0.0.1", Port: 1})

	m := r.recv(t)
	if m.TID != 7 || !m.Ack || !strings.Contains(m.Error, "refused") {
		t.Fatalf("unexpected reply: %+v", m)
	}
	if atomic.LoadInt32(&refused) != 1 {
		t.Fatalf("OnRefuse calls = %d, want 1", refused)
	}

	if conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port))); err == nil {
		conn.Close()
		t.Fatal("refused forward is listening")
	}
}

func TestPolicyZeroAllowsForward(t *testing.T) {
	_, r := startPolicyTunnel(t, Policy{})

	port := freePort(t)
	r.send(t, &tunnelMessage{Type: FORWARD, TID: 8, Source: port, Host: "127.0.0.1", Port: 1})

	if m := r.recv(t); m.TID != 8 || m.Error != "" {
		t.Fatalf("unexpected reply: %+v", m)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("forward is not listening: %v", err)
	}
	conn.Close()
}

func TestPolicyAllowConnect(t *testing.T) {
	allowed, allowedPort := listen(t)
	other, otherPort := listen(t)

	var refused int32
	_, r := startPolicyTunnel(t, Policy{
		AllowConnect: func(host string, port int) bool {
			return host == "127.0.0.1" && port == allowedPort
		},
		OnRefuse: func(string) { atomic.AddInt32(&refused, 1) },
	})

	r.send(t, &tunnelMessage{Type: CONNECT, TID: 1, Host: "127.0.0.1", Port: allowedPort})
	if !accepted(t, allowed, 5*time.Second) {
		t.Fatal("allowed connect did not dial")
	}

	r.send(t, &tunnelMessage{Type: CONNECT, TID: 2, Host: "127.0.0.1", Port: otherPort})
	if m := r.recv(t); m.Type != CLOSED || m.TID != 2 || !strings.Contains(m.Error, "refused") {
		t.Fatalf("unexpected reply: %+v", m)
	}
	if accepted(t, other, 200*time.Millisecond) {
		t.Fatal("refused connect dialed")
	}
	if atomic.LoadInt32(&refused) != 1 {
		t.Fatalf("OnRefuse calls = %d, want 1", refused)
	}
}

func TestPolicyMaxStreams(t *testing.T) {
	ln, port := listen(t)

	_, r := startPolicyTunnel(t, Policy{MaxStreams: 2})

	for tid := 1; tid <= 2; tid++ {
		r.send(t, &tunnelMessage{Type: CONNECT, TID: tid, Host: "127.0.0.1", Port: port})
		if !accepted(t, ln, 5*time.Second) {
			t.Fatalf("stream %d did not dial", tid)
		}
	}

	r.send(t, &tunnelMessage{Type: CONNECT, TID: 3, Host: "127.0.0.1", Port: port})
	if m := r.recv(t); m.Type != CLOSED || m.TID != 3 || !strings.Contains(m.Error, "too many") {
		t.Fatalf("unexpected reply: %+v", m)
	}
}

func TestDuplicateTIDRefused(t *testing.T) {
	ln, port := listen(t)

	_, r := startPolicyTunnel(t, Policy{})

	r.send(t, &tunnelMessage{Type: CONNECT, TID: 5, Host: "127.0.0.1", Port: port})
	if !accepted(t, ln, 5*time.Second) {
		t.Fatal("first connect did not dial")
	}

	r.send(t, &tunnelMessage{Type: CONNECT, TID: 5, Host: "127.0.0.1", Port: port})
	if m := r.recv(t); m.Type != CLOSED || m.TID != 5 || !strings.Contains(m.Error, "already exists") {
		t.Fatalf("unexpected reply: %+v", m)
	}
}

func TestMaxMessageEndsTunnel(t *testing.T) {
	tun, r := startPolicyTunnel(t, Policy{MaxMessage: 1024})

	go r.enc.Encode(&tunnelMessage{Type: DATA, TID: 99, Data: make([]byte, 4096)})

	select {
	case <-tun.quit:
	case <-time.After(5 * time.Second):
		t.Fatal("oversize message did not end the tunnel")
	}
}
