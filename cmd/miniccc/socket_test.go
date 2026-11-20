// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const testTimeout = 10 * time.Second

// syncBuffer is a bytes.Buffer that can be written to by the logger while a
// test reads it.
type syncBuffer struct {
	lock sync.Mutex
	buf  bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.lock.Lock()
	defer b.lock.Unlock()

	return b.buf.String()
}

// upstream stands in for the minimega server: it collects the pipe messages
// that miniccc sends.
type upstream struct {
	lock     sync.Mutex
	messages []ron.Message
}

func newUpstream(t *testing.T) *upstream {
	t.Helper()

	local, remote := net.Pipe()

	oldConn, oldEnc := client.conn, client.enc
	client.writeMu.Lock()
	client.conn, client.enc = local, gob.NewEncoder(local)
	client.writeMu.Unlock()

	u := &upstream{}
	done := make(chan struct{})

	go func() {
		defer close(done)

		dec := gob.NewDecoder(remote)
		for {
			var m ron.Message
			if err := dec.Decode(&m); err != nil {
				return
			}

			u.lock.Lock()
			u.messages = append(u.messages, m)
			u.lock.Unlock()
		}
	}()

	t.Cleanup(func() {
		local.Close()
		remote.Close()
		<-done

		client.writeMu.Lock()
		client.conn, client.enc = oldConn, oldEnc
		client.writeMu.Unlock()
	})

	return u
}

// modes returns what was sent upstream for a pipe: one entry per message,
// the mode, and for data the line too.
func (u *upstream) sent(pipe string) []string {
	u.lock.Lock()
	defer u.lock.Unlock()

	var res []string
	for _, m := range u.messages {
		if m.Type != ron.MESSAGE_PIPE || m.Pipe != pipe {
			continue
		}

		switch m.PipeMode {
		case ron.PIPE_NEW_READER:
			res = append(res, "new reader")
		case ron.PIPE_NEW_WRITER:
			res = append(res, "new writer")
		case ron.PIPE_CLOSE_READER:
			res = append(res, "close reader")
		case ron.PIPE_CLOSE_WRITER:
			res = append(res, "close writer")
		case ron.PIPE_DATA:
			res = append(res, fmt.Sprintf("data %q", m.PipeData))
		}
	}
	return res
}

// startSocket serves the miniccc command socket and returns its path. When
// the test ends it waits for the connection handlers, which fails the test if
// one is stuck.
func startSocket(t *testing.T) string {
	t.Helper()

	// not t.TempDir: a socket path has to be short
	dir, err := os.MkdirTemp("", "miniccc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	path := filepath.Join(dir, "miniccc")

	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}

	var handlers sync.WaitGroup
	accepting := make(chan struct{})

	go func() {
		defer close(accepting)

		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			handlers.Add(1)
			go func() {
				defer handlers.Done()
				commandSocketHandle(conn)
			}()
		}
	}()

	t.Cleanup(func() {
		l.Close()
		<-accepting

		done := make(chan struct{})
		go func() {
			handlers.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(testTimeout):
			t.Error("command socket handlers did not finish")
		}
	})

	return path
}

// dialPipe connects to the command socket like `miniccc -pipe` does.
func dialPipe(t *testing.T, path, pipe string) (net.Conn, *gob.Encoder, *gob.Decoder) {
	t.Helper()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	enc := gob.NewEncoder(conn)
	if err := enc.Encode(MODE_PIPE); err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(pipe); err != nil {
		t.Fatal(err)
	}

	return conn, enc, gob.NewDecoder(conn)
}

// expect waits until exactly want was sent upstream for a pipe, in this
// order.
func (u *upstream) expect(t *testing.T, pipe string, want ...string) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for {
		got := u.sent(pipe)
		if reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sent upstream for %v: %q, want %q", pipe, got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// expectDetached waits until miniccc gave up the pipe upstream, after having
// sent exactly the data lines in want. The reader and the writer are closed
// independently, but the writer only after its last line.
func (u *upstream) expectDetached(t *testing.T, pipe string, want ...string) {
	t.Helper()

	check := func(got []string) bool {
		var data []string
		closed := map[string]int{}

		for i, v := range got {
			switch v {
			case "new reader", "new writer":
			case "close reader", "close writer":
				closed[v] = i
			default:
				data = append(data, v)
				if _, ok := closed["close writer"]; ok {
					return false
				}
			}
		}

		return len(closed) == 2 && len(got) == len(want)+4 && reflect.DeepEqual(data, want)
	}

	deadline := time.Now().Add(testTimeout)
	for {
		got := u.sent(pipe)
		if check(got) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("sent upstream for %v: %q, want data %q and the pipe closed", pipe, got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestPipeConn attaches a client to a pipe through the command socket, and
// passes lines both ways.
func TestPipeConn(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	u := newUpstream(t)
	path := startSocket(t)

	const pipe = "foo"

	conn, enc, dec := dialPipe(t, path, pipe)

	// miniccc registers upstream as a reader and a writer of the pipe
	u.expect(t, pipe, "new reader", "new writer")

	// client to pipe: lines go upstream
	for _, line := range []string{"one\n", "two\n"} {
		if err := enc.Encode(line); err != nil {
			t.Fatal(err)
		}
	}
	u.expect(t, pipe, "new reader", "new writer", `data "one\n"`, `data "two\n"`)

	// pipe to client: lines come from upstream
	pipeMessage(&ron.Message{Type: ron.MESSAGE_PIPE, Pipe: pipe, PipeMode: ron.PIPE_DATA, PipeData: "three\n"})

	conn.SetReadDeadline(time.Now().Add(testTimeout))

	var got string
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != "three\n" {
		t.Errorf("client read %q, want %q", got, "three\n")
	}

	// the client is done: miniccc gives up the pipe and hangs up
	conn.(*net.UnixConn).CloseWrite()

	if err := dec.Decode(&got); err != io.EOF {
		t.Errorf("read after close: %v, want EOF", err)
	}
	u.expectDetached(t, pipe, `data "one\n"`, `data "two\n"`)

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}

// TestPipeConnClientDisconnects closes the connection without reading what
// the pipe sent, as `miniccc -pipe` does when its input ends. What the client
// wrote must go upstream, and a client that is gone is not an error.
func TestPipeConnClientDisconnects(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	u := newUpstream(t)
	path := startSocket(t)

	for i := 0; i < 50; i++ {
		pipe := fmt.Sprintf("foo%d", i)
		line := fmt.Sprintf("hello %d\n", i)

		conn, enc, _ := dialPipe(t, path, pipe)
		u.expect(t, pipe, "new reader", "new writer")

		if err := enc.Encode(line); err != nil {
			t.Fatal(err)
		}

		// something is on its way to the client when it leaves
		pipeMessage(&ron.Message{Type: ron.MESSAGE_PIPE, Pipe: pipe, PipeMode: ron.PIPE_DATA, PipeData: "unread\n"})
		conn.Close()

		u.expectDetached(t, pipe, fmt.Sprintf("data %q", line))
	}

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}
