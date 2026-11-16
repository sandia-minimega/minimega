// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// otherArch is an architecture that is not the architecture of the test.
func otherArch() string {
	if runtime.GOARCH == "arm64" {
		return "amd64"
	}
	return "arm64"
}

func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func writeStaged(t *testing.T, dir, name, data string) string {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	fpath := filepath.Join(dir, name)
	if err := os.WriteFile(fpath, []byte(data), 0755); err != nil {
		t.Fatal(err)
	}
	return fpath
}

// captureWarnings returns a function that returns the warnings logged since
// the call.
func captureWarnings(t *testing.T) func() []string {
	t.Helper()

	ring := log.NewRing(64)
	log.AddLogRing(t.Name(), ring, log.WARN)
	t.Cleanup(func() { log.DelLogger(t.Name()) })

	return ring.Dump
}

func TestUpdateFilePathSelectsPlatformBinary(t *testing.T) {
	base := t.TempDir()
	s := &Server{path: base, subpath: "ns"}

	generic := writeStaged(t, base, "miniccc", "generic")
	writeStaged(t, base, "miniccc.exe", "generic exe")
	other := writeStaged(t, filepath.Join(base, "ns"), "miniccc-linux-"+otherArch(), "other")
	native := writeStaged(t, base, "miniccc-windows-"+runtime.GOARCH+".exe", "native exe")

	for _, tc := range []struct {
		goos, goarch, want string
	}{
		{"linux", runtime.GOARCH, generic},
		{"linux", otherArch(), other},
		{"windows", runtime.GOARCH, native},
	} {
		got, err := s.updateFilePath(tc.goos, tc.goarch)
		if err != nil || got != tc.want {
			t.Errorf("updateFilePath(%q, %q) = %q, %v; want %q", tc.goos, tc.goarch, got, err, tc.want)
		}
	}

	if got, err := s.updateFilePath("windows", otherArch()); err == nil {
		t.Errorf("updateFilePath used %q for a client arch that differs from the host, want error", got)
	}
	if got, err := s.updateFilePath("linux", "../../miniccc"); err == nil {
		t.Errorf("updateFilePath accepted an invalid arch: %q", got)
	}
}

func TestFileSHA256UsesCache(t *testing.T) {
	s := &Server{}
	fpath := writeStaged(t, t.TempDir(), "miniccc", "aaaa")
	mtime := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(fpath, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	if got, err := s.fileSHA256(fpath); err != nil || got != sha256Hex("aaaa") {
		t.Fatalf("fileSHA256 = %q, %v; want %q", got, err, sha256Hex("aaaa"))
	}

	// same size and modification time, so the cached hash is used
	if err := os.WriteFile(fpath, []byte("bbbb"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(fpath, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.fileSHA256(fpath); got != sha256Hex("aaaa") {
		t.Fatalf("fileSHA256 read the file again for an unchanged size and modification time")
	}

	if err := os.Chtimes(fpath, mtime.Add(time.Minute), mtime.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.fileSHA256(fpath); got != sha256Hex("bbbb") {
		t.Fatalf("fileSHA256 = %q after the file changed, want %q", got, sha256Hex("bbbb"))
	}
}

func TestUpdateFileForGatesOnCapabilityAndHash(t *testing.T) {
	base := t.TempDir()
	staged := writeStaged(t, base, "miniccc", "new binary")
	s := &Server{path: base, autoUpdate: true}

	old := &Client{Hostname: "old", OS: "linux", Arch: runtime.GOARCH, Version: "old-revision"}
	warnings := captureWarnings(t)
	if got := s.updateFileFor(old); got != "" {
		t.Errorf("updateFileFor sent %q to a client without the capability", got)
	}
	if w := strings.Join(warnings(), "\n"); !strings.Contains(w, "old") || !strings.Contains(w, "old-revision") || !strings.Contains(w, "manual update") {
		t.Errorf("no manual update warning for the old client, got %q", w)
	}

	current := &Client{Hostname: "current", OS: "linux", Arch: runtime.GOARCH, Version: "old-revision",
		Capabilities: []string{"unknown", CapAutoUpdate}, SHA256: sha256Hex("new binary")}
	if got := s.updateFileFor(current); got != "" {
		t.Errorf("updateFileFor sent %q to a client that runs the staged binary", got)
	}

	current.Version = version.Revision
	current.SHA256 = sha256Hex("different binary")
	if got := s.updateFileFor(current); got != staged {
		t.Errorf("updateFileFor = %q for a client with a different binary, want %q", got, staged)
	}

	s.SetAutoUpdate(false)
	if got := s.updateFileFor(current); got != "" {
		t.Errorf("updateFileFor sent %q with auto-update off", got)
	}
}

// testClient adds a connected client to the server and returns a decoder for
// the messages that the server sends to it.
func testClient(t *testing.T, s *Server, uuid string) (*client, *gob.Decoder) {
	t.Helper()

	serverConn, clientConn := net.Pipe()
	t.Cleanup(func() {
		serverConn.Close()
		clientConn.Close()
	})

	// fail instead of hang when a message is not sent or not read
	deadline := time.Now().Add(5 * time.Second)
	serverConn.SetWriteDeadline(deadline)
	clientConn.SetReadDeadline(deadline)

	c := &client{
		Client: &Client{UUID: uuid, Hostname: uuid},
		conn:   serverConn,
		enc:    gob.NewEncoder(serverConn),
	}

	s.clientLock.Lock()
	if s.clients == nil {
		s.clients = make(map[string]*client)
	}
	s.clients[uuid] = c
	s.clientLock.Unlock()

	return c, gob.NewDecoder(clientConn)
}

// readCommands reads the next command message, skipping other messages.
func readCommands(dec *gob.Decoder) map[int]*Command {
	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			return nil
		}
		if m.Type == MESSAGE_COMMAND {
			return m.Commands
		}
	}
}

func TestUpdatingClientHoldsCommands(t *testing.T) {
	s := &Server{commands: map[int]*Command{}}
	updating, dec := testClient(t, s, "updating")
	other, otherDec := testClient(t, s, "other")
	updating.updating = true

	s.commands[1] = &Command{ID: 1, Command: []string{"once"}, Once: true}
	s.commands[2] = &Command{ID: 2, Command: []string{"always"}}

	done := make(chan map[int]*Command)
	go func() { done <- readCommands(otherDec) }()
	s.sendCommands("")
	if got := <-done; len(got) != 2 {
		t.Fatalf("other client got %d commands, want 2", len(got))
	}
	if updating.maxCommandID != 0 {
		t.Fatalf("updating client maxCommandID = %d, want 0", updating.maxCommandID)
	}

	go func() { done <- readCommands(dec) }()
	if !s.endUpdate(updating, 0) {
		t.Fatal("endUpdate did not release the held commands")
	}
	got := <-done
	if len(got) != 2 || got[1] == nil {
		t.Fatalf("updating client got %v after the update, want commands 1 and 2", got)
	}

	// the other client reconnects and must not run the once command again
	other.maxCommandID = 0
	go func() { done <- readCommands(otherDec) }()
	s.sendCommands("other")
	if got := <-done; got[1] != nil || len(got) != 1 {
		t.Fatalf("other client got %v after a reconnect, want only command 2", got)
	}
}

func TestUpdateTimeoutReleasesCommands(t *testing.T) {
	base := t.TempDir()
	staged := writeStaged(t, base, "miniccc", "new binary")
	s := &Server{path: base, commands: map[int]*Command{}, updateTimeout: 50 * time.Millisecond}
	s.commands[1] = &Command{ID: 1, Command: []string{"hostname"}}
	c, dec := testClient(t, s, "client")

	warnings := captureWarnings(t)

	s.clientLock.Lock()
	s.startUpdate(c, staged)
	s.clientLock.Unlock()

	var data []byte
	for {
		var m Message
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m.Type == MESSAGE_COMMAND {
			t.Fatal("server sent commands during the update")
		}
		if m.Type == MESSAGE_FILE {
			if !m.File.Update {
				t.Fatal("update chunk is not marked as an update")
			}
			data = append(data, m.File.Data...)
			if m.File.EOF {
				break
			}
		}
	}
	if string(data) != "new binary" {
		t.Fatalf("update data = %q, want %q", data, "new binary")
	}

	if got := readCommands(dec); got[1] == nil {
		t.Fatalf("got %v after the update timeout, want command 1", got)
	}
	if w := strings.Join(warnings(), "\n"); !strings.Contains(w, "did not reconnect") {
		t.Errorf("no warning for the update timeout, got %q", w)
	}
}

func TestUpdateSendErrorReleasesCommands(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "miniccc")
	s := &Server{commands: map[int]*Command{}, updateTimeout: time.Hour}
	s.commands[1] = &Command{ID: 1, Command: []string{"hostname"}}
	c, dec := testClient(t, s, "client")

	s.clientLock.Lock()
	s.startUpdate(c, missing)
	s.clientLock.Unlock()

	var m Message
	if err := dec.Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Type != MESSAGE_FILE || m.Error == "" {
		t.Fatalf("first message = %v with error %q, want a file error", m.Type, m.Error)
	}

	if got := readCommands(dec); got[1] == nil {
		t.Fatalf("got %v after a failed update send, want command 1 at once", got)
	}

	s.clientLock.Lock()
	updating := c.updating
	s.clientLock.Unlock()
	if updating {
		t.Fatal("client is still updating after a failed update send")
	}
}

func TestHandshakeUpdatesOnlyCapableClients(t *testing.T) {
	base := t.TempDir()
	writeStaged(t, base, "miniccc", "new binary")

	s := &Server{path: base, clients: map[string]*client{}, commands: map[int]*Command{}, autoUpdate: true}
	warnings := captureWarnings(t)

	handshake := func(cl *Client) {
		t.Helper()

		serverConn, clientConn := net.Pipe()
		t.Cleanup(func() {
			serverConn.Close()
			clientConn.Close()
		})

		go func() {
			io.WriteString(clientConn, "RON")
			io.ReadFull(clientConn, make([]byte, 3))
			m := Message{Type: MESSAGE_CLIENT, UUID: cl.UUID, Client: cl, Version: "v1"}
			gob.NewEncoder(clientConn).Encode(&m)
			io.Copy(io.Discard, clientConn)
		}()

		c, err := s.handshake(serverConn, "")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { close(c.cancelHeartbeat) })
	}

	handshake(&Client{UUID: "old", Hostname: "old", OS: "linux", Arch: runtime.GOARCH, Version: "old-revision"})
	handshake(&Client{UUID: "new", Hostname: "new", OS: "linux", Arch: runtime.GOARCH, Version: "old-revision",
		Capabilities: []string{CapAutoUpdate}, SHA256: sha256Hex("old binary"),
		Processes: map[int]*Process{7: {PID: 7, Command: []string{"sleep"}}}})

	s.clientLock.Lock()
	oldUpdating := s.clients["old"] != nil && s.clients["old"].updating
	newUpdating := s.clients["new"] != nil && s.clients["new"].updating
	s.clientLock.Unlock()

	if oldUpdating {
		t.Error("server started an update for a client without the auto-update capability")
	}
	if !newUpdating {
		t.Error("server did not start an update for a client with the auto-update capability")
	}

	w := strings.Join(warnings(), "\n")
	if !strings.Contains(w, "mismatched miniccc version on old: old-revision") {
		t.Errorf("missing the existing version mismatch warning, got %q", w)
	}
	if !strings.Contains(w, "kills 1 background processes") {
		t.Errorf("missing the background process warning, got %q", w)
	}
}

func TestGetClientsCopiesCapabilities(t *testing.T) {
	s := &Server{}
	c, _ := testClient(t, s, "client")
	c.Capabilities = []string{CapAutoUpdate}
	c.SHA256 = "abc"

	got := s.GetClients()["client"]
	if len(got.Capabilities) != 1 || got.Capabilities[0] != CapAutoUpdate || got.SHA256 != "abc" {
		t.Fatalf("GetClients = %+v, want the capabilities and hash", got)
	}

	got.Capabilities[0] = "changed"
	if c.Capabilities[0] != CapAutoUpdate {
		t.Fatal("GetClients did not copy the capabilities")
	}
}
