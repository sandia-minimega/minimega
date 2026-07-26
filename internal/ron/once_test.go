package ron

import (
	"encoding/gob"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"
)

// testClient is a minimal miniccc stand-in: it performs the RON handshake and
// records every command ID the server sends it.
type testClient struct {
	uuid string
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder

	got chan int
}

func newTestClient(t *testing.T, sock, uuid, hostname string) *testClient {
	t.Helper()

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial %v: %v", sock, err)
	}

	if _, err := io.WriteString(conn, "RON"); err != nil {
		t.Fatalf("write magic: %v", err)
	}

	var buf [3]byte
	for string(buf[:]) != "RON" {
		buf[0], buf[1] = buf[1], buf[2]
		if _, err := conn.Read(buf[2:]); err != nil {
			t.Fatalf("read magic: %v", err)
		}
	}

	c := &testClient{
		uuid: uuid,
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
		got:  make(chan int, 128),
	}

	// handshake message
	m := &Message{
		Type:    MESSAGE_CLIENT,
		UUID:    uuid,
		Version: "v1",
		Client: &Client{
			UUID:     uuid,
			Hostname: hostname,
			Arch:     "amd64",
			OS:       "linux",
			Version:  "v1",
		},
	}
	if err := c.enc.Encode(m); err != nil {
		t.Fatalf("handshake encode: %v", err)
	}

	// server echoes the handshake back
	var ack Message
	if err := c.dec.Decode(&ack); err != nil {
		t.Fatalf("handshake decode: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		for {
			var m Message
			if err := c.dec.Decode(&m); err != nil {
				close(c.got)
				return
			}

			if m.Type != MESSAGE_COMMAND {
				continue
			}

			for id := range m.Commands {
				c.got <- id
			}
		}
	}()

	return c
}

// received drains everything the client has been sent so far.
func (c *testClient) received(d time.Duration) []int {
	var ids []int

	deadline := time.After(d)
	for {
		select {
		case id, ok := <-c.got:
			if !ok {
				return ids
			}
			ids = append(ids, id)
		case <-deadline:
			return ids
		}
	}
}

func contains(ids []int, want int) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// newTestServer returns a ron server listening on a unix socket. It is torn
// down by closing the listener only -- Server.Destroy blocks until every client
// disconnects, which is not what these tests want.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	dir := t.TempDir()

	s, err := NewServer(dir, "", nil)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	s.UseVMs = false

	sock := filepath.Join(dir, "ron.sock")
	if err := s.ListenUnix(sock); err != nil {
		t.Fatalf("ListenUnix: %v", err)
	}

	t.Cleanup(func() {
		_ = s.CloseUnix(sock)
		s.setDestroyed()
	})

	return s, sock
}

const (
	uuidTarget    = "6468268f-97cf-410a-8652-4f84da0b8ce7"
	uuidBystander = "11111111-2222-3333-4444-555555555555"
)

// addCommand inserts a command exactly as NewCommand does, but without the
// asynchronous broadcast, so a test can drive the send ordering itself.
func addCommand(s *Server, c *Command) int {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	s.commandCounter++
	c.ID = s.commandCounter
	s.commands[c.ID] = c

	return c.ID
}

// TestOnceNotConsumedByOtherClientHandshake is the observed production failure,
// with the interleaving forced rather than raced for. A Once command filtered
// to one VM is created; before NewCommand's broadcast runs, another VM finishes
// its handshake and clientHandler issues that client's own sendCommands. With a
// single global Sent bool, that per-client send consumes the command without
// delivering it to anyone -- the filter does not match the bystander -- and the
// broadcast then skips it forever.
func TestOnceNotConsumedByOtherClientHandshake(t *testing.T) {
	s, sock := newTestServer(t)

	target := newTestClient(t, sock, uuidTarget, "rtr-edge")
	newTestClient(t, sock, uuidBystander, "bystander")

	// let both handshakes settle
	time.Sleep(300 * time.Millisecond)
	target.received(200 * time.Millisecond)

	id := addCommand(s, &Command{
		Command: []string{"bash", "/tmp/run.sh"},
		Once:    true,
		Filter:  &Filter{UUID: uuidTarget},
	})

	// the bystander's handshake send lands first
	s.sendCommands(uuidBystander)

	// ...then NewCommand's broadcast
	s.sendCommands("")

	if ids := target.received(time.Second); !contains(ids, id) {
		t.Fatalf("target never received Once command %v (got %v) -- it was consumed by a send to a client its filter did not match", id, ids)
	}
}

// TestOnceNotResentAfterReconnect is the guard on the documented intent: a
// client that disconnects and reconnects must not be sent the Once command a
// second time.
func TestOnceNotResentAfterReconnect(t *testing.T) {
	s, sock := newTestServer(t)

	c1 := newTestClient(t, sock, uuidTarget, "rtr-edge")

	s.NewCommand(&Command{
		Command: []string{"bash", "/tmp/run.sh"},
		Once:    true,
		Filter:  &Filter{UUID: uuidTarget},
	})

	if ids := c1.received(time.Second); !contains(ids, 1) {
		t.Fatalf("target did not get Once command 1, got %v", ids)
	}

	// drop the connection and wait for the server to notice
	_ = c1.conn.Close()
	for i := 0; i < 100 && s.HasClient(uuidTarget); i++ {
		time.Sleep(20 * time.Millisecond)
	}
	if s.HasClient(uuidTarget) {
		t.Fatal("server never removed the disconnected client")
	}

	// same UUID reconnects -- a fresh client struct with maxCommandID == 0
	c2 := newTestClient(t, sock, uuidTarget, "rtr-edge")

	if ids := c2.received(time.Second); contains(ids, 1) {
		t.Fatalf("Once command 1 was re-sent after reconnect (got %v)", ids)
	}
}

// TestOnceDeliveredToLateJoiner covers the broader-filter case: a Once command
// that matches several clients must still reach a client that was not yet
// connected when the command was created.
func TestOnceDeliveredToLateJoiner(t *testing.T) {
	s, sock := newTestServer(t)

	early := newTestClient(t, sock, uuidBystander, "early")

	s.NewCommand(&Command{
		Command: []string{"bash", "/tmp/run.sh"},
		Once:    true,
		Filter:  &Filter{OS: "linux"},
	})

	if ids := early.received(time.Second); !contains(ids, 1) {
		t.Fatalf("early client did not get Once command 1, got %v", ids)
	}

	late := newTestClient(t, sock, uuidTarget, "late")

	if ids := late.received(time.Second); !contains(ids, 1) {
		t.Fatalf("late joiner never received Once command 1 (got %v)", ids)
	}
}

// TestClearCommandsResetsOnceRecords guards the ID-reuse hazard: ClearCommands
// resets the ID counter, so retained delivery records would suppress the new
// commands that reuse those IDs.
func TestClearCommandsResetsOnceRecords(t *testing.T) {
	s, sock := newTestServer(t)

	c := newTestClient(t, sock, uuidTarget, "rtr-edge")

	s.NewCommand(&Command{Command: []string{"first"}, Once: true, Filter: &Filter{UUID: uuidTarget}})
	if ids := c.received(time.Second); !contains(ids, 1) {
		t.Fatalf("did not get first Once command, got %v", ids)
	}

	s.ClearCommands()

	// new command reuses ID 1
	s.NewCommand(&Command{Command: []string{"second"}, Once: true, Filter: &Filter{UUID: uuidTarget}})
	if ids := c.received(time.Second); !contains(ids, 1) {
		t.Fatalf("Once command reusing ID 1 after ClearCommands was suppressed (got %v)", ids)
	}
}

// TestDeleteCommandFreesOnceRecords checks the cleanup path.
func TestDeleteCommandFreesOnceRecords(t *testing.T) {
	s, sock := newTestServer(t)

	c := newTestClient(t, sock, uuidTarget, "rtr-edge")

	id := s.NewCommand(&Command{Command: []string{"x"}, Once: true, Filter: &Filter{UUID: uuidTarget}})
	c.received(500 * time.Millisecond)

	if !s.onceDelivered(id, uuidTarget) {
		t.Fatal("delivery was not recorded")
	}

	if err := s.DeleteCommand(id); err != nil {
		t.Fatalf("DeleteCommand: %v", err)
	}

	if s.onceDelivered(id, uuidTarget) {
		t.Fatal("delivery record leaked after DeleteCommand")
	}
}
