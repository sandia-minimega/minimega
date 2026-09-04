// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestRestartUpdatedKeepsBinaryWhenExecFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "miniccc")
	update := updateTempPath(path)
	if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	// Not an ELF file and no "#!" line, so execve fails with ENOEXEC.
	if err := os.WriteFile(update, []byte{0, 1, 2, 3}, 0755); err != nil {
		t.Fatal(err)
	}

	if err := restartUpdated(path, update); err == nil {
		t.Fatal("restartUpdated succeeded with an invalid binary")
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "old" {
		t.Fatalf("installed binary = %q, %v; want %q", got, err, "old")
	}
}

func TestKillBackgroundStopsProcesses(t *testing.T) {
	client.Processes = map[int]*Process{}
	defer func() { client.Processes = nil }()

	if _, stderr, code := runCommand("", "", "", []string{"sleep", "60"}, true); code != 0 {
		t.Fatalf("start background process: %v, %v", code, stderr)
	}

	client.Lock()
	var pid int
	for k := range client.Processes {
		pid = k
	}
	client.Unlock()
	if pid == 0 {
		t.Fatal("background process is not registered")
	}

	killBackground()

	client.Lock()
	n := len(client.Processes)
	client.Unlock()
	if n != 0 {
		t.Errorf("%d background processes remain after killBackground", n)
	}
	if err := syscall.Kill(pid, 0); err == nil {
		t.Errorf("background process %d is still running", pid)
	}
}
