// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"net"
	"sort"
	"testing"
	"time"
)

func TestBindClientUUIDUsesSerialIdentity(t *testing.T) {
	want := "3b440429-067f-5b75-a0c3-f436519f8ccb"
	m := &Message{Client: &Client{}}
	if err := bindClientUUID(m, want); err != nil {
		t.Fatal(err)
	}
	if m.Client.UUID != want || m.UUID != want {
		t.Fatalf("serial identity was not adopted: %#v", m)
	}
}

func TestBindClientUUIDPreservesGuestIdentity(t *testing.T) {
	want := "a5ba6920-5bcf-4022-b8cf-015425f7b05c"
	m := &Message{Client: &Client{UUID: want}}
	if err := bindClientUUID(m, "different-host-identity"); err != nil {
		t.Fatal(err)
	}
	if m.Client.UUID != want {
		t.Fatalf("guest identity changed: %q", m.Client.UUID)
	}
}

func TestBindClientUUIDRejectsUnboundIdentity(t *testing.T) {
	for name, m := range map[string]*Message{
		"missing client": {},
		"missing UUID":   {Client: &Client{}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := bindClientUUID(m, ""); err == nil {
				t.Fatal("expected an unbound client identity error")
			}
		})
	}
}

func TestSendCommandsTracksIssuedCommands(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	dec := gob.NewDecoder(clientConn)
	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Command: []string{"echo", "hello"}},
		},
		clients: map[string]*client{
			"client": {
				Client: &Client{UUID: "client"},
				conn:   serverConn,
				enc:    gob.NewEncoder(serverConn),
			},
		},
	}

	readCommand := func() {
		t.Helper()

		m := new(Message)
		if err := dec.Decode(m); err != nil {
			t.Fatal(err)
		}
		if len(m.Commands) != 1 {
			t.Fatalf("received %d commands, want 1", len(m.Commands))
		}
	}

	sendAndRead := func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			readCommand()
		}()
		s.sendCommands("")
		<-done
	}

	sendAndRead()

	if got := s.GetCommand(1).Issued; got != 1 {
		t.Fatalf("issued after first send = %d, want 1", got)
	}

	s.clients["client"].maxCommandID = 0
	sendAndRead()

	if got := s.GetCommand(1).Issued; got != 2 {
		t.Fatalf("issued after reconnect = %d, want 2", got)
	}
}

// testClient registers a client backed by an in-memory pipe and continuously
// drains messages the server sends to it, so that sendCommands never blocks on
// an unread write.
type testClient struct {
	msgs chan *Message
}

func (s *Server) addTestClient(t *testing.T, uuid string) *testClient {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})

	s.clients[uuid] = &client{
		Client: &Client{UUID: uuid},
		conn:   serverConn,
		enc:    gob.NewEncoder(serverConn),
	}

	tc := &testClient{msgs: make(chan *Message, 16)}

	go func() {
		dec := gob.NewDecoder(clientConn)

		for {
			m := new(Message)
			if err := dec.Decode(m); err != nil {
				close(tc.msgs)
				return
			}
			tc.msgs <- m
		}
	}()

	return tc
}

// recv returns the command IDs in the next message, or nil if the server did
// not send anything.
func (tc *testClient) recv(t *testing.T) []int {
	t.Helper()

	select {
	case m, ok := <-tc.msgs:
		if !ok {
			t.Fatal("client connection closed")
		}

		var ids []int
		for id := range m.Commands {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		return ids
	case <-time.After(2 * time.Second):
		return nil
	}
}

// A command marked Once must still be delivered to a client that connects after
// the command was posted. Previously the command was consumed by the broadcast
// that happened while no clients were connected, so it was never delivered.
func TestSendCommandsOnceReachesLateClient(t *testing.T) {
	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Once: true, Command: []string{"echo", "hello"}},
		},
		clients: map[string]*client{},
	}

	// broadcast with no clients connected, as NewCommand does
	s.sendCommands("")

	if got := s.GetCommand(1).Issued; got != 0 {
		t.Fatalf("issued with no clients = %d, want 0", got)
	}

	// the client connects afterwards and must receive the command
	tc := s.addTestClient(t, "client")
	s.sendCommands("client")

	if got := tc.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("late client received commands %v, want [1]", got)
	}
	if got := s.GetCommand(1).Issued; got != 1 {
		t.Fatalf("issued after late connect = %d, want 1", got)
	}
}

// A command marked Once must not be redelivered to a client that already ran
// it, even after that client reconnects and its watermark is reset.
func TestSendCommandsOnceNotResentAfterReconnect(t *testing.T) {
	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Once: true, Command: []string{"echo", "hello"}},
		},
		clients: map[string]*client{},
	}

	tc := s.addTestClient(t, "client")
	s.sendCommands("")

	if got := tc.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("received commands %v, want [1]", got)
	}

	// simulate a reconnect, which rebuilds the client and resets the watermark
	s.clients["client"].maxCommandID = 0
	s.sendCommands("")

	if got := tc.recv(t); got != nil {
		t.Fatalf("once command resent after reconnect: %v", got)
	}
	if got := s.GetCommand(1).Issued; got != 1 {
		t.Fatalf("issued after reconnect = %d, want 1", got)
	}
}

// Once is tracked per client, so a client that was not connected when the
// command was posted still receives it after another client already ran it.
func TestSendCommandsOnceIsPerClient(t *testing.T) {
	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Once: true, Command: []string{"echo", "hello"}},
		},
		clients: map[string]*client{},
	}

	first := s.addTestClient(t, "first")
	s.sendCommands("")

	if got := first.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("first client received %v, want [1]", got)
	}

	second := s.addTestClient(t, "second")
	s.sendCommands("second")

	if got := second.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("second client received %v, want [1]", got)
	}
	if got := s.GetCommand(1).Issued; got != 2 {
		t.Fatalf("issued = %d, want 2", got)
	}
}

// A command that a filter excludes must not advance the client's watermark,
// since the client never received it.
func TestSendCommandsFilteredCommandDoesNotBurnWatermark(t *testing.T) {
	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Command: []string{"echo", "world"}},
			2: {ID: 2, Command: []string{"echo", "hello"}, Filter: &Filter{UUID: "other"}},
		},
		clients: map[string]*client{},
	}

	tc := s.addTestClient(t, "client")
	s.sendCommands("")

	// only the unfiltered command is delivered
	if got := tc.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("client received %v, want [1]", got)
	}

	// the watermark reflects what was delivered, not the filtered command
	if got := s.clients["client"].maxCommandID; got != 1 {
		t.Fatalf("maxCommandID = %d, want 1", got)
	}
	if got := s.GetCommand(2).Issued; got != 0 {
		t.Fatalf("issued for filtered command = %d, want 0", got)
	}
}

// A send failure must not consume a Once command; the client should still get it
// once the connection works again.
func TestSendCommandsOnceSurvivesSendFailure(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	clientConn.Close()
	serverConn.Close()

	s := &Server{
		commands: map[int]*Command{
			1: {ID: 1, Once: true, Command: []string{"echo", "hello"}},
		},
		clients: map[string]*client{
			"client": {
				Client: &Client{UUID: "client"},
				conn:   serverConn,
				enc:    gob.NewEncoder(serverConn),
			},
		},
	}

	s.sendCommands("")

	if got := s.GetCommand(1).Issued; got != 0 {
		t.Fatalf("issued after failed send = %d, want 0", got)
	}
	if got := s.clients["client"].maxCommandID; got != 0 {
		t.Fatalf("maxCommandID after failed send = %d, want 0", got)
	}

	// the client reconnects and must still receive the command
	tc := s.addTestClient(t, "client")
	s.sendCommands("client")

	if got := tc.recv(t); len(got) != 1 || got[0] != 1 {
		t.Fatalf("received %v after reconnect, want [1]", got)
	}
}
