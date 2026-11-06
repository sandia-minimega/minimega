// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
)

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

// newTestTunnel returns the minimega end of a tunnel whose miniccc end is a
// real minitunnel server.
func newTestTunnel(t *testing.T) *minitunnel.Tunnel {
	t.Helper()

	guest, host := net.Pipe()
	t.Cleanup(func() {
		guest.Close()
		host.Close()
	})

	errs := make(chan error, 1)
	go func() {
		errs <- minitunnel.ListenAndServe(guest)
	}()

	tunnel, err := minitunnel.Dial(host)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := <-errs; err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	return tunnel
}

// newSilentTunnel returns the minimega end of a tunnel whose miniccc end
// answers the handshake and then drops every message without an ACK. forward
// gets a value each time the miniccc end gets a FORWARD message.
func newSilentTunnel(t *testing.T, forward chan<- struct{}) *minitunnel.Tunnel {
	t.Helper()

	guest, host := net.Pipe()
	t.Cleanup(func() {
		guest.Close()
		host.Close()
	})

	type message struct {
		Type int
		Ack  bool
	}

	go func() {
		enc, dec := gob.NewEncoder(guest), gob.NewDecoder(guest)

		var m message
		if err := dec.Decode(&m); err != nil {
			return
		}
		if err := enc.Encode(&message{Type: minitunnel.HANDSHAKE, Ack: true}); err != nil {
			return
		}

		for {
			var m message
			if err := dec.Decode(&m); err != nil {
				return
			}
			if m.Type == minitunnel.FORWARD {
				forward <- struct{}{}
			}
		}
	}()

	tunnel, err := minitunnel.Dial(host)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	return tunnel
}

// serveOnce accepts one connection on a new listener, expects "hello", and
// answers "world". It returns the listener's port and a channel that gets the
// result.
func serveOnce(t *testing.T) (int, <-chan error) {
	t.Helper()

	dest, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { dest.Close() })

	served := make(chan error, 1)
	go func() {
		conn, err := dest.Accept()
		if err != nil {
			served <- err
			return
		}
		defer conn.Close()

		buf := make([]byte, len("hello"))
		if _, err := io.ReadFull(conn, buf); err != nil {
			served <- err
			return
		}
		if string(buf) != "hello" {
			served <- fmt.Errorf("destination got %q, want %q", buf, "hello")
			return
		}

		_, err = conn.Write([]byte("world"))
		served <- err
	}()

	return dest.Addr().(*net.TCPAddr).Port, served
}

// checkReverse sends "hello" through the reverse tunnel on src and expects
// "world" back from the destination.
func checkReverse(t *testing.T, src int, served <-chan error) {
	t.Helper()

	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", src))
	if err != nil {
		t.Fatalf("dial reverse tunnel: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	buf := make([]byte, len("world"))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Fatalf("client got %q, want %q", buf, "world")
	}

	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

func TestReverseTunnelsMatchingClients(t *testing.T) {
	dst, served := serveOnce(t)

	s := &Server{
		clients: map[string]*client{
			"match": {Client: &Client{UUID: "match", OS: "linux"}, tunnel: newTestTunnel(t)},
			// Reverse fails on this client's nil tunnel unless the filter skips it
			"other": {Client: &Client{UUID: "other", OS: "windows"}},
		},
	}

	src := freePort(t)

	if err := s.Reverse(&Filter{OS: "linux"}, src, "127.0.0.1", dst); err != nil {
		t.Fatalf("Reverse: %v", err)
	}

	checkReverse(t, src, served)
}

func TestReverseNoMatchingClients(t *testing.T) {
	s := &Server{
		clients: map[string]*client{
			"other": {Client: &Client{UUID: "other", OS: "windows"}},
		},
	}

	err := s.Reverse(&Filter{OS: "linux"}, freePort(t), "127.0.0.1", 80)
	if err == nil || err.Error() != "no clients match filter: os=linux" || !errors.Is(err, ErrNoClients) {
		t.Fatalf("Reverse with filter = %v, want no clients match filter", err)
	}

	s = &Server{clients: map[string]*client{}}

	err = s.Reverse(nil, freePort(t), "127.0.0.1", 80)
	if err == nil || err.Error() != "no clients connected" || !errors.Is(err, ErrNoClients) {
		t.Fatalf("Reverse without clients = %v, want no clients connected", err)
	}
}

func TestReverseContinuesAfterFailedClients(t *testing.T) {
	dst, served := serveOnce(t)

	s := &Server{
		clients: map[string]*client{
			"a-uninitialized": {Client: &Client{UUID: "a-uninitialized"}},
			"b-working":       {Client: &Client{UUID: "b-working"}, tunnel: newTestTunnel(t)},
			"c-uninitialized": {Client: &Client{UUID: "c-uninitialized"}},
		},
	}

	src := freePort(t)

	err := s.Reverse(nil, src, "127.0.0.1", dst)
	if err == nil {
		t.Fatal("Reverse succeeded, want an error for each uninitialized client")
	}

	want := "a-uninitialized: tunnel has not been initialized\n" +
		"c-uninitialized: tunnel has not been initialized"
	if err.Error() != want {
		t.Fatalf("Reverse error = %q, want %q", err, want)
	}

	checkReverse(t, src, served)
}

func TestReverseTimesOutWithoutHoldingClientLock(t *testing.T) {
	defer func(d time.Duration) { reverseTimeout = d }(reverseTimeout)
	reverseTimeout = 100 * time.Millisecond

	forward := make(chan struct{}, 1)

	s := &Server{
		clients: map[string]*client{
			"silent": {Client: &Client{UUID: "silent"}, tunnel: newSilentTunnel(t, forward)},
		},
	}

	src := freePort(t)

	errs := make(chan error, 1)
	go func() {
		errs <- s.Reverse(nil, src, "127.0.0.1", 80)
	}()

	select {
	case <-forward:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not get the FORWARD message")
	}

	if !s.clientLock.TryLock() {
		t.Error("Reverse holds clientLock while it waits for the client")
	} else {
		s.clientLock.Unlock()
	}

	select {
	case err := <-errs:
		if err == nil || !strings.HasPrefix(err.Error(), "silent: no response from client") {
			t.Fatalf("Reverse error = %v, want a timeout for client silent", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Reverse did not time out")
	}
}

// newHeldTunnel returns the minimega end of a tunnel whose miniccc end answers
// the handshake, sends the TID of each FORWARD message to forward, and sends an
// ACK for each TID that the test sends to the returned channel.
func newHeldTunnel(t *testing.T, forward chan<- int) (*minitunnel.Tunnel, chan<- int) {
	t.Helper()

	guest, host := net.Pipe()
	t.Cleanup(func() {
		guest.Close()
		host.Close()
	})

	type message struct {
		Type int
		Ack  bool
		TID  int
	}

	answers := make(chan int)
	t.Cleanup(func() { close(answers) })

	go func() {
		enc, dec := gob.NewEncoder(guest), gob.NewDecoder(guest)

		var m message
		if err := dec.Decode(&m); err != nil {
			return
		}
		if err := enc.Encode(&message{Type: minitunnel.HANDSHAKE, Ack: true}); err != nil {
			return
		}

		go func() {
			for tid := range answers {
				if err := enc.Encode(&message{Type: minitunnel.DATA, Ack: true, TID: tid}); err != nil {
					return
				}
			}
		}()

		for {
			var m message
			if err := dec.Decode(&m); err != nil {
				return
			}
			if m.Type == minitunnel.FORWARD {
				forward <- m.TID
			}
		}
	}()

	tunnel, err := minitunnel.Dial(host)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	return tunnel, answers
}

func TestReverseSkipsClientWithPendingRequest(t *testing.T) {
	defer func(d time.Duration) { reverseTimeout = d }(reverseTimeout)
	reverseTimeout = 100 * time.Millisecond

	forward := make(chan int, 1)
	tunnel, answers := newHeldTunnel(t, forward)

	c := &client{Client: &Client{UUID: "held"}, tunnel: tunnel}
	s := &Server{clients: map[string]*client{"held": c}}

	first, second := freePort(t), freePort(t)

	err := s.Reverse(nil, first, "127.0.0.1", 80)
	if err == nil || !strings.HasPrefix(err.Error(), "held: no response from client") {
		t.Fatalf("first Reverse error = %v, want a timeout for client held", err)
	}

	var tid int
	select {
	case tid = <-forward:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not get the first FORWARD message")
	}

	err = s.Reverse(nil, second, "127.0.0.1", 81)
	if err == nil || err.Error() != "held: a reverse request is already pending" {
		t.Fatalf("second Reverse error = %v, want a pending request for client held", err)
	}

	select {
	case <-forward:
		t.Fatal("client got a FORWARD message while a request was pending")
	default:
	}

	answers <- tid

	deadline := time.Now().Add(5 * time.Second)
	for {
		s.clientLock.Lock()
		pending := c.reversePending
		s.clientLock.Unlock()

		if !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("pending marker was not cleared after the client answered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	errs := make(chan error, 1)
	go func() {
		errs <- s.Reverse(nil, second, "127.0.0.1", 81)
	}()

	select {
	case tid = <-forward:
	case <-time.After(5 * time.Second):
		t.Fatal("client did not get the third FORWARD message")
	}
	answers <- tid

	if err := <-errs; err != nil {
		t.Fatalf("third Reverse: %v", err)
	}
}

type fakeVM struct {
	uuid string
	name string
	tags map[string]string
}

func (vm *fakeVM) GetNamespace() string { return "" }
func (vm *fakeVM) GetUUID() string      { return vm.uuid }
func (vm *fakeVM) SetCCActive(bool)     {}
func (vm *fakeVM) SetTag(k, v string)   { vm.tags[k] = v }

func (vm *fakeVM) GetTags() map[string]string {
	res := map[string]string{}
	for k, v := range vm.tags {
		res[k] = v
	}
	return res
}

func (vm *fakeVM) Info(k string) (string, error) {
	if k == "name" {
		return vm.name, nil
	}
	return "", fmt.Errorf("invalid info field: %v", k)
}

func TestReverseMatchesVMNameAndTags(t *testing.T) {
	dst, served := serveOnce(t)

	s := &Server{
		UseVMs: true,
		clients: map[string]*client{
			"named":  {Client: &Client{UUID: "named"}, tunnel: newTestTunnel(t)},
			"tagged": {Client: &Client{UUID: "tagged"}},
			// only the guest reports this tag, and the VM tags replace it
			"other":        {Client: &Client{UUID: "other", Tags: map[string]string{"role": "web"}}},
			"unregistered": {Client: &Client{UUID: "unregistered"}},
		},
		vms: map[string]VM{
			"named":  &fakeVM{uuid: "named", name: "server", tags: map[string]string{}},
			"tagged": &fakeVM{uuid: "tagged", name: "web", tags: map[string]string{"role": "web"}},
			"other":  &fakeVM{uuid: "other", name: "client", tags: map[string]string{}},
		},
	}

	src := freePort(t)

	if err := s.Reverse(&Filter{Tags: map[string]string{"name": "server"}}, src, "127.0.0.1", dst); err != nil {
		t.Fatalf("Reverse with name=server: %v", err)
	}

	checkReverse(t, src, served)

	err := s.Reverse(&Filter{Tags: map[string]string{"role": "web"}}, freePort(t), "127.0.0.1", 80)
	if err == nil || err.Error() != "tagged: tunnel has not been initialized" {
		t.Fatalf("Reverse with role=web error = %v, want an error for client tagged only", err)
	}
}
