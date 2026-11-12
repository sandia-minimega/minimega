// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"go.uber.org/goleak"
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

func TestDestroyStopsServerGoroutines(t *testing.T) {
	ignore := goleak.IgnoreCurrent()

	s := newTestServer(t, t.TempDir(), "ns")
	// let clientReaper reach its wait, so that Destroy must interrupt it
	time.Sleep(10 * time.Millisecond)
	s.Destroy()

	goleak.VerifyNone(t, ignore)
}

func TestDestroyReapsSilentTCPClient(t *testing.T) {
	s, err := newServer(t.TempDir(), "ns", nil, 10*time.Millisecond, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("newServer: %v", err)
	}
	s.UseVMs = false

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if err := s.Listen(port); err != nil {
		t.Fatalf("Listen(%d): %v", port, err)
	}

	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	if _, err := io.WriteString(conn, "RON"); err != nil {
		t.Fatal(err)
	}
	var magic [3]byte
	if _, err := io.ReadFull(conn, magic[:]); err != nil || string(magic[:]) != "RON" {
		t.Fatalf("read magic bytes = %q, %v", magic, err)
	}

	m := &Message{
		Type:    MESSAGE_CLIENT,
		UUID:    "c6a2a0f4-5f0e-4b8e-9d5c-2f0b1c9e7a31",
		Client:  &Client{UUID: "c6a2a0f4-5f0e-4b8e-9d5c-2f0b1c9e7a31"},
		Version: "v1",
	}
	if err := gob.NewEncoder(conn).Encode(m); err != nil {
		t.Fatal(err)
	}

	for deadline := time.Now().Add(5 * time.Second); s.Clients() != 1; {
		if time.Now().After(deadline) {
			t.Fatalf("Clients() = %d, want 1", s.Clients())
		}
		time.Sleep(time.Millisecond)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Destroy()
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Destroy did not return while a silent TCP client was connected")
	}
}
