// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"compress/gzip"
	"io"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// StopFB returns after the reader goroutine writes the end of the gzip stream
// and closes the file. The file is a FIFO that the test fills, so the writes
// of the reader wait until the test reads the FIFO.
func TestFBRecordStopWaitsForReader(t *testing.T) {
	_, addr := newFBServer(t, 0)

	path := filepath.Join(t.TempDir(), "fb")
	if err := unix.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { unix.Close(fd) })

	// fill each page of the FIFO, and then each byte that is free
	var filled int
	for _, size := range []int{4096, 1} {
		for {
			n, err := unix.Write(fd, make([]byte, size))
			if err == unix.EAGAIN {
				break
			} else if err != nil {
				t.Fatal(err)
			}
			filled += n
		}
	}

	rec := NewRecorder()
	if err := rec.RecordFB("vm", addr, path); err != nil {
		t.Fatal(err)
	}

	stopped := make(chan error, 1)
	go func() { stopped <- rec.StopFB("vm") }()

	var stopErr error
	returned := false
	select {
	case stopErr = <-stopped:
		returned = true
		t.Error("StopFB returned before the reader closed the file")
	case <-time.After(200 * time.Millisecond):
	}

	// read the FIFO until StopFB returns and the FIFO is empty
	var out []byte
	buf := make([]byte, 65536)
	deadline := time.Now().Add(testTimeout)
	for {
		n, err := unix.Read(fd, buf)
		if n > 0 {
			out = append(out, buf[:n]...)
		}
		if err == nil {
			continue
		} else if err != unix.EAGAIN {
			t.Fatal(err)
		}

		if returned {
			break
		}
		select {
		case stopErr = <-stopped:
			returned = true
		case <-time.After(time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for StopFB")
		}
	}

	if stopErr != nil {
		t.Errorf("StopFB: %v", stopErr)
	}
	if len(out) < filled {
		t.Fatalf("read %d bytes from the FIFO, want more than the %d filled", len(out), filled)
	}

	zr, err := gzip.NewReader(bytes.NewReader(out[filled:]))
	if err != nil {
		t.Fatalf("fb file is not gzip: %v", err)
	}
	if _, err := io.ReadAll(zr); err != nil {
		t.Errorf("read fb file to EOF: %v", err)
	}
}
