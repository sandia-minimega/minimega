// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniplumber

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestPlumbFileTerminalFinishesBeforeRemoval uses a full FIFO to hold the
// final file write and checks that the pipeline stays until the write is done.
func TestPlumbFileTerminalFinishesBeforeRemoval(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}

	fd, err := syscall.Open(fifo, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(fd)

	filled := 0
	for _, size := range []int{4096, 1} {
		buf := make([]byte, size)
		for {
			n, err := syscall.Write(fd, buf)
			if errors.Is(err, syscall.EAGAIN) {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			filled += n
		}
	}

	plumber := New(nil)
	if err := plumber.Plumb("printf hello", "file://"+fifo); err != nil {
		t.Fatalf("Plumb returned error: %v", err)
	}

	deadline := time.After(500 * time.Millisecond)
	for waiting := true; waiting; {
		if len(plumber.Pipelines()) == 0 {
			t.Fatal("pipeline was removed before the file terminal wrote its data")
		}
		select {
		case <-deadline:
			waiting = false
		case <-time.After(time.Millisecond):
		}
	}

	want := filled + len("hello\n")
	got := make([]byte, 0, want)
	buf := make([]byte, 4096)
	readDeadline := time.Now().Add(TIMEOUT)
	for len(got) < want {
		n, err := syscall.Read(fd, buf)
		if errors.Is(err, syscall.EAGAIN) {
			if time.Now().After(readDeadline) {
				t.Fatalf("read %d of %d bytes from the FIFO", len(got), want)
			}
			time.Sleep(time.Millisecond)
			continue
		} else if err != nil {
			t.Fatal(err)
		}
		got = append(got, buf[:n]...)
	}

	if tail := string(got[filled:]); tail != "hello\n" {
		t.Fatalf("file terminal wrote %q, want %q", tail, "hello\n")
	}

	waitForPipeline(t, plumber)
}
