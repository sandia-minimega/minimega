// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

type restartCall struct {
	path, update string
	data         []byte
	perm         os.FileMode
	// stopped is true when stop ran before restart
	stopped bool
}

func newTestUpdater(t *testing.T, perm os.FileMode) (*updater, string, *[]restartCall) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "miniccc")
	if err := os.WriteFile(path, []byte("old"), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}

	var calls []restartCall
	var stopped bool
	u := &updater{
		executable: func() (string, error) { return path, nil },
		stop:       func() { stopped = true },
		restart: func(path, update string) error {
			data, err := os.ReadFile(update)
			if err != nil {
				return err
			}
			fi, err := os.Stat(update)
			if err != nil {
				return err
			}
			calls = append(calls, restartCall{path, update, data, fi.Mode().Perm(), stopped})
			return nil
		},
	}

	return u, path, &calls
}

func TestUpdateKeepsInstalledPermissions(t *testing.T) {
	u, path, calls := newTestUpdater(t, 0755)

	chunks := []*ron.File{
		{Name: "miniccc", Perm: 0644, Data: []byte("new "), Offset: 0},
		{Name: "miniccc", Perm: 0644, Data: []byte("binary"), Offset: 4, EOF: true},
	}
	for _, f := range chunks {
		if err := u.handle(f); err != nil {
			t.Fatal(err)
		}
	}

	if len(*calls) != 1 {
		t.Fatalf("got %d restarts, want 1", len(*calls))
	}
	got := (*calls)[0]
	if got.path != path || got.update != updateTempPath(path) {
		t.Errorf("restart(%q, %q), want restart(%q, %q)", got.path, got.update, path, updateTempPath(path))
	}
	if string(got.data) != "new binary" {
		t.Errorf("staged data = %q, want %q", got.data, "new binary")
	}
	if got.perm != 0755 {
		t.Errorf("staged permissions = %v, want %v", got.perm, os.FileMode(0755))
	}
	if !got.stopped {
		t.Error("background processes were not stopped before the restart")
	}
}

func TestUpdateWriteFailureDiscardsTransfer(t *testing.T) {
	u, path, calls := newTestUpdater(t, 0755)

	if err := u.handle(&ron.File{Data: []byte("new"), Offset: 0}); err != nil {
		t.Fatal(err)
	}
	if err := u.handle(&ron.File{Data: []byte("bad"), Offset: -1}); err == nil {
		t.Fatal("negative offset write succeeded, want error")
	}
	if err := u.handle(&ron.File{Offset: 3, EOF: true}); err == nil {
		t.Fatal("EOF after a failed chunk succeeded, want error")
	}

	if len(*calls) != 0 {
		t.Fatalf("restart ran %d times after a failed chunk", len(*calls))
	}
	if _, err := os.Stat(updateTempPath(path)); !os.IsNotExist(err) {
		t.Errorf("staged file still exists after a failed chunk: %v", err)
	}
}

func TestUpdateServerErrorDiscardsTransfer(t *testing.T) {
	u, path, calls := newTestUpdater(t, 0755)

	if err := u.handle(&ron.File{Data: []byte("new"), Offset: 0}); err != nil {
		t.Fatal(err)
	}
	u.abort()
	if err := u.handle(&ron.File{Offset: 3, EOF: true}); err == nil {
		t.Fatal("EOF after an aborted transfer succeeded, want error")
	}

	if len(*calls) != 0 {
		t.Fatalf("restart ran %d times after an aborted transfer", len(*calls))
	}
	if _, err := os.Stat(updateTempPath(path)); !os.IsNotExist(err) {
		t.Errorf("staged file still exists after an aborted transfer: %v", err)
	}
}

func TestInstallUpdateRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "miniccc")
	update := updateTempPath(path)
	if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(update, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}

	restore, err := installUpdate(path, update)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("installed binary = %q, want %q", got, "new")
	}

	if err := restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Fatalf("restored binary = %q, want %q", got, "old")
	}
}

func TestProbeReportsHashAndCapability(t *testing.T) {
	u, path, _ := newTestUpdater(t, 0755)
	want := sha256.Sum256([]byte("old"))

	sum, caps := u.probe()
	if sum != hex.EncodeToString(want[:]) {
		t.Errorf("probe hash = %q, want %q", sum, hex.EncodeToString(want[:]))
	}
	if len(caps) != 1 || caps[0] != ron.CapAutoUpdate {
		t.Errorf("probe capabilities = %v, want [%v]", caps, ron.CapAutoUpdate)
	}

	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Errorf("probe left files in the binary folder: %v, %v", entries, err)
	}
}

func TestProbeWithoutWritableFolder(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only folder")
	}

	u, path, _ := newTestUpdater(t, 0755)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0755) })

	sum, caps := u.probe()
	if sum == "" {
		t.Error("probe did not report the hash of a read-only binary")
	}
	if len(caps) != 0 {
		t.Errorf("probe capabilities = %v for a read-only folder, want none", caps)
	}
}

func TestHeartbeatReportsUpdateInfo(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	client.enc = gob.NewEncoder(clientConn)
	client.Processes = map[int]*Process{}
	client.Tags = map[string]string{}
	client.Capabilities = []string{ron.CapAutoUpdate}
	client.SHA256 = "abc"
	defer func() {
		client.enc = nil
		client.Capabilities = nil
		client.SHA256 = ""
	}()

	go heartbeat()

	var m ron.Message
	if err := gob.NewDecoder(serverConn).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m.Client == nil || len(m.Client.Capabilities) != 1 || m.Client.Capabilities[0] != ron.CapAutoUpdate || m.Client.SHA256 != "abc" {
		t.Fatalf("heartbeat client = %+v, want the capabilities and hash", m.Client)
	}
}

func TestExitForRestartExitsWithError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "miniccc")
	update := updateTempPath(path)
	if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(update, []byte("new"), 0755); err != nil {
		t.Fatal(err)
	}

	code := -1
	if err := exitForRestart(path, update, func(c int) { code = c }); err != nil {
		t.Fatal(err)
	}
	if code == 0 || code == -1 {
		t.Errorf("exit code = %d, want a non-zero exit", code)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("installed binary = %q, want %q", got, "new")
	}
}
