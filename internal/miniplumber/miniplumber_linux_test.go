// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniplumber

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

// zombies returns the PIDs of this process's children that have exited but
// were never waited for.
func zombies(t *testing.T) []int {
	t.Helper()

	stats, err := filepath.Glob("/proc/[0-9]*/stat")
	if err != nil {
		t.Fatal(err)
	}

	var res []int
	for _, stat := range stats {
		data, err := os.ReadFile(stat)
		if err != nil {
			// the process is gone
			continue
		}

		// pid (comm) state ppid ...; comm may contain spaces and parentheses
		s := string(data)
		i := strings.LastIndex(s, ")")
		if i < 0 {
			continue
		}

		var state string
		var ppid int
		if _, err := fmt.Sscan(s[i+1:], &state, &ppid); err != nil {
			continue
		}

		if ppid == os.Getpid() && state == "Z" {
			pid, _ := strconv.Atoi(filepath.Base(filepath.Dir(stat)))
			res = append(res, pid)
		}
	}

	return res
}

// TestPlumbProcessIsReaped checks that the process of a deleted pipeline is
// waited for, and is gone by the time the pipeline is no longer listed.
func TestPlumbProcessIsReaped(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found")
	}

	p := New(nil)

	if err := p.Plumb("in", "cat", "out"); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the pipeline to start", func() bool {
		return len(p.Pipes()) == 2 && getPipe(t, p, "in").NumReaders() == 1
	})

	if err := p.PipelineDelete("in", "cat", "out"); err != nil {
		t.Fatal(err)
	}
	waitForPipeline(t, p)

	if z := zombies(t); len(z) != 0 {
		t.Fatalf("pipeline left zombie processes: %v", z)
	}
}
