// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"net"
	"os"
	"path/filepath"
	"strings"
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

// A client that sees a command's response count go up must be able to read
// that response from disk: cc users poll "cc commands" and then read the
// response files. A large stdout widens the window between the two.
func TestResponseCountedOnlyAfterFilesWritten(t *testing.T) {
	s := &Server{
		path:        t.TempDir(),
		commands:    map[int]*Command{1: {ID: 1}},
		clients:     map[string]*client{},
		responses:   make(chan *Client, 1),
		handlerDone: make(chan struct{}),
	}
	go s.responseHandler()
	defer func() {
		close(s.responses)
		<-s.handlerDone
	}()

	stdout := strings.Repeat("x", 64<<20)
	s.responses <- &Client{
		UUID:      "client",
		Responses: []*Response{{ID: 1, Stdout: stdout, Stderr: "oops", ExitCode: 3, RecordExitCode: true}},
	}

	// spin rather than sleep so the check runs as soon as the count moves
	deadline := time.Now().Add(10 * time.Second)
	for len(s.GetCommand(1).CheckedIn) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("response was never counted")
		}
	}

	dir := filepath.Join(s.responsePath(nil), "1", "client")
	code, err := os.ReadFile(filepath.Join(dir, "exitcode"))
	if err != nil || string(code) != "3" {
		t.Errorf("exitcode after check-in = %q, %v; want \"3\"", code, err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "stdout"))
	if err != nil || len(out) != len(stdout) {
		t.Errorf("stdout after check-in has %d bytes, %v; want %d", len(out), err, len(stdout))
	}
	errOut, err := os.ReadFile(filepath.Join(dir, "stderr"))
	if err != nil || string(errOut) != "oops" {
		t.Errorf("stderr after check-in = %q, %v; want \"oops\"", errOut, err)
	}
}
