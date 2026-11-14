// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// fakeServer is a minimal RFB 3.3 server on a loopback listener. It completes
// the handshake that Dial expects and records every byte the client sends.
type fakeServer struct {
	addr string

	mu   sync.Mutex
	recv bytes.Buffer
}

func newFakeServer(t *testing.T) *fakeServer {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	s := &fakeServer{addr: l.Addr().String()}

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()

	return s
}

func (s *fakeServer) serve(c net.Conn) {
	defer c.Close()

	io.WriteString(c, "RFB 003.003\n")
	if _, err := io.ReadFull(c, make([]byte, 12)); err != nil {
		return
	}
	c.Write([]byte{0, 0, 0, 1}) // security type none
	if _, err := io.ReadFull(c, make([]byte, 1)); err != nil {
		return
	}

	// server init: width, height, pixel format, name
	binary.Write(c, binary.BigEndian, uint16(640))
	binary.Write(c, binary.BigEndian, uint16(480))
	binary.Write(c, binary.BigEndian, PixelFormat{BitsPerPixel: 32, Depth: 24, TrueColorFlag: 1})
	binary.Write(c, binary.BigEndian, uint32(4))
	io.WriteString(c, "test")

	buf := make([]byte, 512)
	for {
		n, err := c.Read(buf)
		s.mu.Lock()
		s.recv.Write(buf[:n])
		s.mu.Unlock()
		if err != nil {
			return
		}
	}
}

// gotKey waits for a KeyEvent with the given keysym to reach the server.
func (s *fakeServer) gotKey(key byte) bool {
	// RFC 6143 7.5.4: type 4, down-flag, 2 padding, 4 byte key
	want := []byte{4, 1, 0, 0, 0, 0, 0, key}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ok := bytes.Contains(s.recv.Bytes(), want)
		s.mu.Unlock()
		if ok {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}

	return false
}

func writeKbr(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// within fails the test, rather than hanging it, if f does not return in time.
func within(t *testing.T, d time.Duration, what string, f func() error) {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- f() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(d):
		t.Fatalf("%s: still blocked after %v (deadlock)", what, d)
	}
}

// Injecting LoadFile with no running playback starts one. Used to deadlock on
// Player.mu.
func TestInjectLoadFileNoPlayback(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	writeKbr(t, filepath.Join(dir, "a.kbr"), "0:KeyEvent,true,a\n")

	p := NewPlayer()
	within(t, 5*time.Second, "Inject", func() error {
		return p.Inject("vm", srv.addr, dir, "LoadFile,"+filepath.Join(dir, "a.kbr"))
	})

	if !srv.gotKey('a') {
		t.Fatal("playback did not send the key event")
	}

	// the player must still be usable
	within(t, 5*time.Second, "Info", func() error { p.Info(); return nil })
}

// A relative LoadFile with no running playback resolves against dir, like
// `vnc play` does against the files directory.
func TestInjectLoadFileRelative(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	writeKbr(t, filepath.Join(dir, "rel.kbr"), "0:KeyEvent,true,b\n")

	p := NewPlayer()
	within(t, 5*time.Second, "Inject", func() error {
		return p.Inject("vm", srv.addr, dir, "LoadFile,rel.kbr")
	})

	if !srv.gotKey('b') {
		t.Fatal("relative LoadFile was not resolved against dir")
	}
}

// Injecting LoadFile into a running playback hands a *LoadFileEvent to the
// play loop. Used to panic on a type assertion. The nested file is relative to
// the parent file's directory, not to dir.
func TestInjectLoadFileRunning(t *testing.T) {
	srv := newFakeServer(t)
	parentDir := t.TempDir()
	// the parent waits long enough for the inject to arrive
	writeKbr(t, filepath.Join(parentDir, "parent.kbr"), "30s:KeyEvent,true,z\n")
	writeKbr(t, filepath.Join(parentDir, "child.kbr"), "0:KeyEvent,true,c\n")

	p := NewPlayer()
	if err := p.Playback("vm", srv.addr, filepath.Join(parentDir, "parent.kbr")); err != nil {
		t.Fatal(err)
	}
	defer within(t, 5*time.Second, "Stop", func() error { return p.Stop("vm") })

	within(t, 5*time.Second, "Inject", func() error {
		return p.Inject("vm", srv.addr, t.TempDir(), "LoadFile,child.kbr")
	})

	if !srv.gotKey('c') {
		t.Fatal("nested LoadFile did not play")
	}
}

// A control signal sent right after a playback starts must not deadlock. The
// play loop takes the playback lock (addDuration, setFile, setStep) before it
// reaches its select, so a sender that held that lock across the unbuffered
// send blocked forever, and Player.mu with it.
func TestSignalRightAfterStart(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	writeKbr(t, filepath.Join(dir, "parent.kbr"), "30s:KeyEvent,true,z\n")
	writeKbr(t, filepath.Join(dir, "child.kbr"), "30s:KeyEvent,true,c\n")

	p := NewPlayer()
	for i := 0; i < 200; i++ {
		if err := p.Playback("vm", srv.addr, filepath.Join(dir, "parent.kbr")); err != nil {
			t.Fatal(err)
		}
		within(t, 5*time.Second, "Inject", func() error {
			return p.Inject("vm", srv.addr, dir, "LoadFile,child.kbr")
		})
		within(t, 5*time.Second, "Stop", func() error { return p.Stop("vm") })
	}
}
