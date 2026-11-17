// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
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

// dialGuest connects to s over TCP the way miniccc does and completes the
// handshake.
func dialGuest(t *testing.T, s *Server, uuid string) (net.Conn, *gob.Encoder, *gob.Decoder) {
	t.Helper()

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

	enc, dec := gob.NewEncoder(conn), gob.NewDecoder(conn)
	m := &Message{Type: MESSAGE_CLIENT, UUID: uuid, Client: &Client{UUID: uuid}, Version: "v1"}
	if err := enc.Encode(m); err != nil {
		t.Fatal(err)
	}

	for deadline := time.Now().Add(5 * time.Second); s.Clients() != 1; {
		if time.Now().After(deadline) {
			t.Fatalf("Clients() = %d, want 1", s.Clients())
		}
		time.Sleep(time.Millisecond)
	}

	return conn, enc, dec
}

// waitClosed waits for the server to drop the guest connection.
func waitClosed(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.Copy(io.Discard, conn)
}

// A guest must not be able to write outside the response folder by choosing
// the name of a file it uploads.
func TestGuestFileUploadCannotEscapeResponseDir(t *testing.T) {
	outer := t.TempDir()
	base := filepath.Join(outer, "base")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}

	s := newTestServer(t, base, "ns")
	s.UseVMs = false
	defer s.Destroy()

	conn, enc, _ := dialGuest(t, s, "c6a2a0f4-5f0e-4b8e-9d5c-2f0b1c9e7a31")

	f := &File{ID: 1, Name: "../../../../../pwned", Perm: 0o644, Data: []byte("evil"), EOF: true}
	if err := enc.Encode(&Message{Type: MESSAGE_FILE, File: f}); err != nil {
		t.Fatal(err)
	}

	waitClosed(conn)
	mustNotExist(t, filepath.Join(outer, "pwned"))
}

// A guest must not be able to make the server read files outside its files
// folder by asking for a name containing "..".
func TestGuestFileRequestCannotEscapeFilesDir(t *testing.T) {
	outer := t.TempDir()
	base := filepath.Join(outer, "base")
	if err := os.Mkdir(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "secret"), []byte("hunter2"), 0o600); err != nil {
		t.Fatal(err)
	}

	s := newTestServer(t, base, "ns")
	s.UseVMs = false
	defer s.Destroy()

	conn, enc, dec := dialGuest(t, s, "c6a2a0f4-5f0e-4b8e-9d5c-2f0b1c9e7a31")

	if err := enc.Encode(&Message{Type: MESSAGE_FILE, File: &File{Name: "../../secret"}}); err != nil {
		t.Fatal(err)
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			return
		}
		if m.File != nil && strings.Contains(string(m.File.Data), "hunter2") {
			t.Fatalf("server sent a file from outside its files folder: %q", m.File.Name)
		}
	}
}

// A guest that sends a message without the payload its type requires must
// only lose its own connection, not crash the server.
func TestGuestMessageWithoutPayloadDropsGuest(t *testing.T) {
	for _, typ := range []Type{MESSAGE_FILE, MESSAGE_CLIENT} {
		t.Run(typ.String(), func(t *testing.T) {
			s := newTestServer(t, t.TempDir(), "ns")
			s.UseVMs = false
			defer s.Destroy()

			conn, enc, _ := dialGuest(t, s, "c6a2a0f4-5f0e-4b8e-9d5c-2f0b1c9e7a31")

			if err := enc.Encode(&Message{Type: typ}); err != nil {
				t.Fatal(err)
			}

			waitClosed(conn)

			for deadline := time.Now().Add(2 * time.Second); s.Clients() != 0; {
				if time.Now().After(deadline) {
					t.Fatalf("Clients() = %d, want 0", s.Clients())
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

// GetExitCode is called from the cc exitcode command while clients connect
// and disconnect.
func TestGetExitCodeWhileClientsChange(t *testing.T) {
	s := newTestServer(t, t.TempDir(), "ns")
	defer s.Destroy()

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}

			uuid := strconv.Itoa(i)
			s.clientLock.Lock()
			s.clients[uuid] = &client{Client: &Client{UUID: uuid, Hostname: "h" + uuid}}
			delete(s.clients, uuid)
			s.clientLock.Unlock()
		}
	}()

	for i := 0; i < 2000; i++ {
		s.GetExitCode(1, "nosuchhost")
	}

	close(stop)
	<-done
}

// drainConn returns the server end of a pipe whose peer reads and discards
// everything the server sends.
func drainConn(t *testing.T) net.Conn {
	t.Helper()

	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	go io.Copy(io.Discard, b)

	return a
}

func newGuestClient(uuid string, conn net.Conn) *client {
	return &client{
		Client:          &Client{UUID: uuid},
		conn:            conn,
		enc:             gob.NewEncoder(conn),
		dec:             gob.NewDecoder(conn),
		pipeReaders:     make(map[string]*miniplumber.Reader),
		pipeWriters:     make(map[string]chan<- string),
		cancelHeartbeat: make(chan struct{}),
	}
}

// A client that the reaper expires must release its plumber readers just as
// if it had disconnected.
func TestReaperReleasesExpiredClientPipes(t *testing.T) {
	s, err := newServer(t.TempDir(), "ns", nil, 10*time.Millisecond, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.UseVMs = false
	defer s.Destroy()

	c := newGuestClient("c1", drainConn(t))
	r := &miniplumber.Reader{C: make(chan string), Done: make(chan struct{})}
	c.pipeReaders["foo"] = r

	s.clientLock.Lock()
	s.clients["c1"] = c
	s.clientLock.Unlock()

	select {
	case <-r.Done:
	case <-time.After(5 * time.Second):
		t.Fatal("expired client's plumber reader was not closed")
	}
}

// The handler of a dropped connection must not evict the client that has
// since reconnected under the same UUID.
func TestOldHandlerDoesNotEvictReconnectedClient(t *testing.T) {
	s := newTestServer(t, t.TempDir(), "ns")
	s.UseVMs = false
	defer s.Destroy()

	// the old connection was already reaped, so it is not in s.clients
	oldLocal, oldRemote := net.Pipe()
	go io.Copy(io.Discard, oldRemote)
	old := newGuestClient("c1", oldLocal)

	fresh := newGuestClient("c1", drainConn(t))
	s.clientLock.Lock()
	s.clients["c1"] = fresh
	s.clientLock.Unlock()
	defer s.removeClient(fresh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.clientHandler(old)
	}()

	oldRemote.Close()
	<-done

	s.clientLock.Lock()
	got := s.clients["c1"]
	s.clientLock.Unlock()

	if got != fresh {
		t.Fatal("old handler removed the reconnected client")
	}
}

// A handler still dispatching pipe messages for a client the reaper dropped
// must not send to or close a writer the drop already closed.
func TestPipeMessagesAfterDropDoNotPanic(t *testing.T) {
	s := newTestServer(t, t.TempDir(), "ns")
	s.UseVMs = false
	defer s.Destroy()

	c := newGuestClient("c1", drainConn(t))
	c.pipeWriters["foo"] = make(chan string, 1)

	s.clientLock.Lock()
	s.clients["c1"] = c
	s.clientLock.Unlock()
	s.removeClient(c)

	p := miniplumber.New(nil)
	c.pipeHandler(p, &Message{Pipe: "foo", PipeMode: PIPE_DATA, PipeData: "x"})
	c.pipeHandler(p, &Message{Pipe: "foo", PipeMode: PIPE_CLOSE_WRITER})
}

// A pipe write stalled on a consumer that never reads must not block the
// reaper, and with it every client lookup, when the client expires.
func TestReaperNotBlockedByStalledPipeWrite(t *testing.T) {
	s, err := newServer(t.TempDir(), "ns", nil, 10*time.Millisecond, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	s.UseVMs = false
	defer s.Destroy()

	c := newGuestClient("c1", drainConn(t))
	c.pipeWriters["foo"] = make(chan string) // never read

	s.clientLock.Lock()
	s.clients["c1"] = c
	s.clientLock.Unlock()

	sent := make(chan struct{})
	go func() {
		defer close(sent)
		c.pipeHandler(miniplumber.New(nil), &Message{Pipe: "foo", PipeMode: PIPE_DATA, PipeData: "x"})
	}()

	reaped := make(chan struct{})
	go func() {
		defer close(reaped)
		for s.Clients() != 0 {
			time.Sleep(time.Millisecond)
		}
	}()

	for _, ch := range []chan struct{}{reaped, sent} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("reaper blocked by a stalled pipe write")
		}
	}
}
