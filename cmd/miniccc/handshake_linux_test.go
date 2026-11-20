// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"bytes"
	"encoding/gob"
	"io"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

const (
	testUUIDA = "a5ba6920-5bcf-4022-b8cf-015425f7b05c"
	testUUIDB = "3b440429-067f-5b75-a0c3-f436519f8ccb"
)

var testClientOnce sync.Once

// newTestClient sets up the client from the flags, as NewClient does. It
// makes the client channels only once, because the command handler of an
// earlier client loop can still read them.
func newTestClient() {
	testClientOnce.Do(NewClient)

	client.Lock()
	defer client.Unlock()

	client.claim = resolveClientUUID(*f_uuid, getUUID)
	client.UUID = client.claim
	client.acked = false
}

// saveClientFlags restores the dial flags when the test ends.
func saveClientFlags(t *testing.T) {
	family, serial, uuid := *f_family, *f_serial, *f_uuid
	t.Cleanup(func() {
		*f_family, *f_serial, *f_uuid = family, serial, uuid
	})
}

// resetClientState clears the client identity and connection.
func resetClientState() {
	client.Lock()
	defer client.Unlock()

	client.UUID, client.claim, client.acked = "", "", false
	client.conn, client.enc, client.dec = nil, nil, nil
}

func TestGuestUUID(t *testing.T) {
	saveClientFlags(t)

	for _, tt := range []struct {
		family, serial string
		want           string
	}{
		{"tcp", "", testUUIDA},
		{"unix", "", testUUIDA},
		{"vsock", "", ""},
		{"tcp", "/dev/virtio-ports/cc", ""},
	} {
		*f_family, *f_serial = tt.family, tt.serial

		read := false
		got := guestUUID(func(string) ([]byte, error) {
			read = true
			return []byte("A5BA6920-5BCF-4022-B8CF-015425F7B05C\n"), nil
		})
		if got != tt.want {
			t.Errorf("guestUUID(%v, %q) = %q, want %q", tt.family, tt.serial, got, tt.want)
		}
		if hostBoundChannel() && read {
			t.Errorf("guestUUID(%v, %q) read the DMI UUID on a host-bound channel", tt.family, tt.serial)
		}
	}
}

// TestHandshakeClaim checks the UUID that each handshake claims when the host
// binds the first connection to testUUIDA and the second to testUUIDB.
func TestHandshakeClaim(t *testing.T) {
	saveClientFlags(t)
	defer resetClientState()

	const own = "6f1b6a3e-2f5c-4d1e-9a7b-0c8d9e0f1a2b"

	for _, tt := range []struct {
		name                 string
		family, serial, uuid string
		claims               [2]string
		final                string
	}{
		{"tcp", "tcp", "", own, [2]string{own, own}, own},
		{"unix", "unix", "", own, [2]string{own, own}, own},
		{"vsock", "vsock", "", "", [2]string{"", ""}, testUUIDB},
		{"serial", "tcp", "/dev/virtio-ports/cc", "", [2]string{"", ""}, testUUIDB},
		{"vsock -uuid", "vsock", "", testUUIDA, [2]string{testUUIDA, testUUIDA}, testUUIDA},
	} {
		t.Run(tt.name, func(t *testing.T) {
			*f_family, *f_serial, *f_uuid = tt.family, tt.serial, tt.uuid
			newTestClient()

			var buf bytes.Buffer
			send := func() string {
				buf.Reset()
				client.Lock()
				client.enc = gob.NewEncoder(&buf)
				client.Unlock()

				heartbeat()

				var m ron.Message
				if err := gob.NewDecoder(&buf).Decode(&m); err != nil {
					t.Fatal(err)
				}
				if m.UUID != m.Client.UUID {
					t.Fatalf("message UUID %q != client UUID %q", m.UUID, m.Client.UUID)
				}
				return m.Client.UUID
			}

			for i, bound := range []string{testUUIDA, testUUIDB} {
				// a new connection, as dial starts it
				client.Lock()
				client.acked = false
				client.Unlock()

				claim := send()
				if claim != tt.claims[i] {
					t.Errorf("handshake %d claimed %q, want %q", i+1, claim, tt.claims[i])
				}

				ack := claim
				if ack == "" {
					ack = bound
				}
				uuid := ackHandshake(&ron.Client{UUID: ack})

				if got := send(); got != uuid {
					t.Errorf("heartbeat %d after the ack sent %q, want %q", i+1, got, uuid)
				}
			}

			if got := clientUUID(); got != tt.final {
				t.Errorf("client UUID = %q, want %q", got, tt.final)
			}
		})
	}
}

// failWriter is a connection on which each write fails.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

// TestHeartbeatKeepsResponses checks that the responses and the tags stay
// queued until a heartbeat after the ack sends them. The server does not read
// the ones in the handshake, and a failed write does not send them.
func TestHeartbeatKeepsResponses(t *testing.T) {
	saveClientFlags(t)
	defer resetClientState()

	*f_family, *f_serial, *f_uuid = "vsock", "", ""
	newTestClient()

	queue := func() {
		client.Lock()
		client.Responses, client.Tags = nil, nil
		client.Unlock()

		appendResponse(&ron.Response{ID: 7, Stdout: "out"})
		addTag("k", "v")
	}

	// send sends one heartbeat on w and returns the number of responses and
	// tags that are still queued.
	send := func(w io.Writer) (int, int) {
		client.Lock()
		client.enc, client.lost = gob.NewEncoder(w), make(chan error, 1)
		client.Unlock()

		heartbeat()

		client.Lock()
		defer client.Unlock()

		return len(client.Responses), len(client.Tags)
	}

	decode := func(buf *bytes.Buffer) *ron.Client {
		var m ron.Message
		if err := gob.NewDecoder(buf).Decode(&m); err != nil {
			t.Fatal(err)
		}
		return m.Client
	}

	var buf bytes.Buffer

	// the handshake carries none, and all stay queued
	queue()
	if r, tags := send(&buf); r != 1 || tags != 1 {
		t.Errorf("after the handshake: %d responses and %d tags queued, want 1 and 1", r, tags)
	}
	if c := decode(&buf); len(c.Responses) != 0 || len(c.Tags) != 0 {
		t.Errorf("the handshake carried %d responses and %d tags, want none", len(c.Responses), len(c.Tags))
	}

	ackHandshake(&ron.Client{UUID: testUUIDA})

	// a heartbeat that fails keeps them
	if r, tags := send(failWriter{}); r != 1 || tags != 1 {
		t.Errorf("after a failed heartbeat: %d responses and %d tags queued, want 1 and 1", r, tags)
	}

	// the next heartbeat sends them
	buf.Reset()
	if r, tags := send(&buf); r != 0 || tags != 0 {
		t.Errorf("after a heartbeat: %d responses and %d tags queued, want none", r, tags)
	}
	if c := decode(&buf); len(c.Responses) != 1 || c.Responses[0].ID != 7 || c.Tags["k"] != "v" {
		t.Errorf("the heartbeat carried responses %v and tags %v", c.Responses, c.Tags)
	}
}

// serveBound acts as the host end of a channel bound to uuid: it answers the
// handshake, sends the UUIDs that the client claimed on claims, and reads
// messages until host closes. Like ron.Server, it refuses another UUID.
func serveBound(host net.Conn, uuid string, claims chan<- [2]string) {
	defer host.Close()

	var magic [3]byte
	if _, err := io.ReadFull(host, magic[:]); err != nil {
		return
	}
	if _, err := io.WriteString(host, "RON"); err != nil {
		return
	}

	enc, dec := gob.NewEncoder(host), gob.NewDecoder(host)

	var m ron.Message
	if err := dec.Decode(&m); err != nil || m.Client == nil {
		return
	}
	claims <- [2]string{m.UUID, m.Client.UUID}

	switch m.Client.UUID {
	case "":
		m.UUID, m.Client.UUID = uuid, uuid
	case uuid:
	default:
		return
	}

	if err := enc.Encode(&m); err != nil {
		return
	}

	for dec.Decode(&m) == nil {
	}
}

// waitBound waits until the client has an acknowledged handshake as want.
func waitBound(t *testing.T, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		client.Lock()
		got, acked := client.UUID, client.acked
		client.Unlock()

		if acked && got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("client UUID = %q (acked %v), want %q", got, acked, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestHostBoundReconnectClaimsNoUUID restores a snapshot of a vsock guest
// into another VM: the second connection is bound to another UUID.
func TestHostBoundReconnectClaimsNoUUID(t *testing.T) {
	saveClientFlags(t)
	*f_family, *f_serial, *f_uuid = "vsock", "", ""
	newTestClient()

	bound := []string{testUUIDA, testUUIDB}
	hosts := make(chan net.Conn, len(bound))
	claims := make(chan [2]string, len(bound))
	stopped := make(chan struct{})

	dialer := vsockDialer
	calls := 0
	vsockDialer = func(int) (net.Conn, error) {
		if calls == len(bound) {
			// stop the client loop; dial releases the client lock
			close(stopped)
			runtime.Goexit()
		}

		guest, host := net.Pipe()
		go serveBound(host, bound[calls], claims)
		calls++
		hosts <- host

		return guest, nil
	}

	go resetClient()

	for i, want := range bound {
		var host net.Conn
		select {
		case host = <-hosts:
		case <-time.After(10 * time.Second):
			t.Fatalf("connection %d: client did not dial", i+1)
		}

		select {
		case got := <-claims:
			if got != [2]string{} {
				t.Errorf("connection %d: handshake claimed message UUID %q, client UUID %q; want empty", i+1, got[0], got[1])
			}
		case <-time.After(10 * time.Second):
			t.Fatalf("connection %d: no handshake", i+1)
		}

		waitBound(t, want)

		// the connection resets, as when the snapshot is restored
		host.Close()
	}

	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not reconnect")
	}

	resetClientState()
	vsockDialer = dialer
}
