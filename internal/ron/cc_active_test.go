// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"io"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

const ccTestUUID = "64b94cc6-925b-403a-a975-a7723079e74d"

// ccFakeVM is a ron.VM that records each SetCCActive call.
type ccFakeVM struct {
	uuid string

	mu    sync.Mutex
	calls []bool
}

func (vm *ccFakeVM) GetNamespace() string        { return "" }
func (vm *ccFakeVM) GetUUID() string             { return vm.uuid }
func (vm *ccFakeVM) GetTags() map[string]string  { return map[string]string{} }
func (vm *ccFakeVM) SetTag(string, string)       {}
func (vm *ccFakeVM) Info(string) (string, error) { return "", nil }

func (vm *ccFakeVM) SetCCActive(active bool) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	vm.calls = append(vm.calls, active)
}

func (vm *ccFakeVM) wantCalls(t *testing.T, want ...bool) {
	t.Helper()

	vm.mu.Lock()
	defer vm.mu.Unlock()

	if !reflect.DeepEqual(vm.calls, want) {
		t.Fatalf("SetCCActive calls = %v, want %v", vm.calls, want)
	}
}

func newCCTestServer(vms ...VM) *Server {
	s := &Server{
		UseVMs:    true,
		commands:  map[int]*Command{},
		clients:   map[string]*client{},
		vms:       map[string]VM{},
		responses: make(chan *Client, 1024),
	}

	for _, vm := range vms {
		s.RegisterVM(vm)
	}

	return s
}

// connectCCClient runs the server handshake against an in-memory miniccc
// client that sends uuid. It returns the server's client and the client end of
// the connection. The client end reads and discards all later messages.
func connectCCClient(t *testing.T, s *Server, uuid string) (*client, net.Conn) {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() { clientConn.Close() })

	errc := make(chan error, 1)
	go func() {
		if _, err := io.WriteString(clientConn, "RON"); err != nil {
			errc <- err
			return
		}

		var magic [3]byte
		if _, err := io.ReadFull(clientConn, magic[:]); err != nil {
			errc <- err
			return
		}

		dec := gob.NewDecoder(clientConn)
		m := Message{Type: MESSAGE_CLIENT, UUID: uuid, Version: "v1", Client: &Client{UUID: uuid}}
		if err := gob.NewEncoder(clientConn).Encode(&m); err != nil {
			errc <- err
			return
		}
		if err := dec.Decode(&m); err != nil {
			errc <- err
			return
		}
		errc <- nil

		for dec.Decode(&m) == nil {
		}
	}()

	c, err := s.handshake(serverConn, "")
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("client handshake: %v", err)
	}

	return c, clientConn
}

// startClientHandler runs clientHandler for c and returns a channel that is
// closed when the handler returns.
func startClientHandler(s *Server, c *client) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.clientHandler(c)
	}()

	return done
}

func waitHandler(t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("client handler did not return")
	}
}

// expire makes the reaper treat c as a client that stopped checking in.
func expire(s *Server, c *client) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c.checkin = time.Now().Add(-2 * CLIENT_EXPIRED * time.Second)
}

func TestCCActiveConnectAndDisconnect(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	c, clientConn := connectCCClient(t, s, ccTestUUID)
	vm.wantCalls(t, true)

	done := startClientHandler(s, c)
	clientConn.Close()
	waitHandler(t, done)

	vm.wantCalls(t, true, false)
	if s.HasClient(ccTestUUID) {
		t.Fatal("client is still in the client list after disconnect")
	}
}

func TestCCActiveMangledUUID(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	// QEMU can report the UUID with the first three fields byte-swapped
	c, clientConn := connectCCClient(t, s, unmangle(ccTestUUID))
	vm.wantCalls(t, true)

	done := startClientHandler(s, c)
	clientConn.Close()
	waitHandler(t, done)

	vm.wantCalls(t, true, false)
}

func TestCCActiveReaperExpiresClient(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	c, _ := connectCCClient(t, s, ccTestUUID)
	done := startClientHandler(s, c)

	expire(s, c)
	s.reapClients()
	vm.wantCalls(t, true, false)

	// the reaper closed the connection, so the handler returns and removes
	// nothing more
	waitHandler(t, done)
	vm.wantCalls(t, true, false)
}

func TestCCActiveReconnectKeepsNewClient(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	old, _ := connectCCClient(t, s, ccTestUUID)
	expire(s, old)
	s.reapClients()

	recent, _ := connectCCClient(t, s, ccTestUUID)
	t.Cleanup(func() { s.removeClient(recent) })
	vm.wantCalls(t, true, false, true)

	// the handler of the old connection returns after the new client is added
	waitHandler(t, startClientHandler(s, old))

	vm.wantCalls(t, true, false, true)
	s.clientLock.Lock()
	got := s.clients[ccTestUUID]
	s.clientLock.Unlock()
	if got != recent {
		t.Fatal("removal of the old connection removed the new client")
	}
}

func TestCCActiveUnregisterVM(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	c, _ := connectCCClient(t, s, ccTestUUID)
	s.UnregisterVM(vm)
	vm.wantCalls(t, true, false)

	s.removeClient(c)
	vm.wantCalls(t, true, false)
}

func TestCCActiveReaperSetsConnectedClients(t *testing.T) {
	vm := &ccFakeVM{uuid: ccTestUUID}
	s := newCCTestServer(vm)

	c, _ := connectCCClient(t, s, ccTestUUID)
	t.Cleanup(func() { s.removeClient(c) })

	s.reapClients()
	vm.wantCalls(t, true, true)

	if !s.HasClient(ccTestUUID) {
		t.Fatal("reaper removed a client that checked in")
	}
}
