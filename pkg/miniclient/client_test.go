// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package miniclient

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

const timeout = 10 * time.Second

// serve starts a minimega stand-in that handles one connection with handler,
// and returns the base directory to Dial. The test waits for the handler when
// it ends.
func serve(t *testing.T, handler func(conn *net.UnixConn, enc *json.Encoder, dec *json.Decoder)) string {
	t.Helper()

	// not t.TempDir: a socket path has to be short
	base, err := os.MkdirTemp("", "miniclient")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })

	l, err := net.Listen("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)

		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		handler(conn.(*net.UnixConn), json.NewEncoder(conn), json.NewDecoder(conn))
	}()

	t.Cleanup(func() {
		l.Close()

		select {
		case <-done:
		case <-time.After(timeout):
			t.Error("server handler did not finish")
		}
	})

	return base
}

func dial(t *testing.T, base string) *Conn {
	t.Helper()

	mm, err := Dial(base)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { mm.Close() })

	return mm
}

// readAll reads r until it returns an error, and returns what it read and the
// error, nil for io.EOF.
func readAll(t *testing.T, r io.Reader) (string, error) {
	t.Helper()

	type result struct {
		data []byte
		err  error
	}

	c := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		c <- result{data, err}
	}()

	select {
	case res := <-c:
		return string(res.data), res.err
	case <-time.After(timeout):
		t.Fatal("timed out reading from the pipe")
		return "", nil
	}
}

type fakePager struct {
	pages []string
}

func (p *fakePager) Page(output string) {
	p.pages = append(p.pages, output)
}

func TestDialNoServer(t *testing.T) {
	if _, err := Dial(t.TempDir()); err == nil {
		t.Fatal("dialed a base directory without a socket")
	}
}

func TestRun(t *testing.T) {
	requests := make(chan Request, 1)

	base := serve(t, func(_ *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		var r Request
		if err := dec.Decode(&r); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		requests <- r

		// a status update, a first response and the last one
		enc.Encode(Response{Status: true, Rendered: "working"})
		enc.Encode(Response{Rendered: "one", More: true})
		enc.Encode(Response{
			Rendered: "two",
			Resp:     minicli.Responses{{Host: "host", Response: "two"}},
		})
	})

	mm := dial(t, base)
	mm.TTY = true

	var got []string
	for r := range mm.Run("vm info") {
		got = append(got, r.Rendered)
	}

	if want := []string{"working", "one", "two"}; !reflect.DeepEqual(got, want) {
		t.Errorf("responses = %q, want %q", got, want)
	}
	if want := (Request{Command: "vm info", TTY: true}); <-requests != want {
		t.Errorf("request does not match %+v", want)
	}
	if err := mm.Error(); err != nil {
		t.Errorf("Error = %v, want nil", err)
	}

	// an empty command sends nothing
	for r := range mm.Run("") {
		t.Errorf("response to an empty command: %+v", r)
	}
}

func TestRunServerDisconnects(t *testing.T) {
	base := serve(t, func(_ *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
	})

	mm := dial(t, base)

	for r := range mm.Run("vm info") {
		t.Errorf("response from a server that sent none: %+v", r)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Fatalf("Error = %v, want server disconnected", err)
	}
}

func TestRunBadResponse(t *testing.T) {
	base := serve(t, func(conn *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
		conn.Write([]byte("not json\n"))
	})

	mm := dial(t, base)

	for range mm.Run("vm info") {
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Error = %v, want a decode error", err)
	}
}

func TestRunAndSuggestOnClosedConnection(t *testing.T) {
	base := serve(t, func(_ *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
	})

	mm := dial(t, base)
	mm.Close()

	for r := range mm.Run("vm info") {
		t.Errorf("response on a closed connection: %+v", r)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "encode") {
		t.Fatalf("Error = %v, want an encode error", err)
	}

	if got := mm.Suggest("vm"); got != nil {
		t.Errorf("suggestions on a closed connection: %v", got)
	}
	if r, w := mm.Pipe("foo"); r != nil || w != nil {
		t.Error("pipe on a closed connection")
	}
}

func TestRunAndPrint(t *testing.T) {
	base := serve(t, func(_ *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		for {
			var r Request
			if err := dec.Decode(&r); err != nil {
				return
			}
			enc.Encode(Response{
				Rendered: "out: " + r.Command,
				Resp:     minicli.Responses{{Host: "host", Error: "err: " + r.Command}},
			})
		}
	})

	mm := dial(t, base)

	pager := &fakePager{}
	mm.Pager = pager

	// capture what is printed
	stdout, stderr := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outW, errW

	mm.RunAndPrint("paged", true)
	mm.RunAndPrint("printed", false)

	outW.Close()
	errW.Close()
	os.Stdout, os.Stderr = stdout, stderr

	gotOut, _ := readAll(t, outR)
	gotErr, _ := readAll(t, errR)

	if want := []string{"out: paged"}; !reflect.DeepEqual(pager.pages, want) {
		t.Errorf("paged %q, want %q", pager.pages, want)
	}
	if want := "out: printed\n"; gotOut != want {
		t.Errorf("stdout = %q, want %q", gotOut, want)
	}
	if want := "Error (host): err: paged\nError (host): err: printed\n"; gotErr != want {
		t.Errorf("stderr = %q, want %q", gotErr, want)
	}
}

func TestSuggest(t *testing.T) {
	base := serve(t, func(conn *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		var r Request
		if err := dec.Decode(&r); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		enc.Encode(Response{Suggest: []string{r.Suggest + " info", r.Suggest + " launch"}})

		// answer the second request with garbage, and none after that
		dec.Decode(&r)
		conn.Write([]byte("not json\n"))
	})

	mm := dial(t, base)

	if got, want := mm.Suggest("vm"), []string{"vm info", "vm launch"}; !reflect.DeepEqual(got, want) {
		t.Errorf("suggestions = %q, want %q", got, want)
	}
	if err := mm.Error(); err != nil {
		t.Errorf("Error = %v, want nil", err)
	}

	if got := mm.Suggest("vm"); got != nil {
		t.Errorf("suggestions from a bad response: %q", got)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("Error = %v, want a decode error", err)
	}
}

func TestSuggestServerDisconnects(t *testing.T) {
	base := serve(t, func(_ *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
	})

	mm := dial(t, base)

	if got := mm.Suggest("vm"); got != nil {
		t.Errorf("suggestions from a server that sent none: %q", got)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Fatalf("Error = %v, want server disconnected", err)
	}
}

// TestPipe attaches to a pipe, writes to it and reads from it. The server
// keeps sending after the client closed its writer, as minimega does while it
// writes the client's last lines to the pipe, and the client must read that
// too before it sees the end of the pipe.
func TestPipe(t *testing.T) {
	type session struct {
		pipe  string
		lines []string
	}
	sessions := make(chan session, 1)

	base := serve(t, func(conn *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		var r Request
		if err := dec.Decode(&r); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}

		s := session{pipe: r.PlumbPipe}

		enc.Encode("hello from the pipe\n")

		// read what the client writes until it closes its writer
		for {
			var line string
			if err := dec.Decode(&line); err != nil {
				if err != io.EOF {
					t.Errorf("decode line: %v", err)
				}
				break
			}
			s.lines = append(s.lines, line)
		}

		// the client must still be listening
		if err := enc.Encode("written\n"); err != nil {
			t.Errorf("send after the client closed its writer: %v", err)
		}

		sessions <- s
	})

	mm := dial(t, base)

	r, w := mm.Pipe("minimega//foo")
	if r == nil || w == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	// two lines in one write, one line in two writes, and a last line that
	// does not end in a newline
	for _, s := range []string{"one\ntwo\n", "thr", "ee\n", "four"} {
		if _, err := io.WriteString(w, s); err != nil {
			t.Fatalf("write %q: %v", s, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := readAll(t, r)
	if err != nil {
		t.Errorf("read: %v", err)
	}
	if want := "hello from the pipe\nwritten\n"; got != want {
		t.Errorf("read %q, want %q", got, want)
	}

	s := <-sessions
	if s.pipe != "minimega//foo" {
		t.Errorf("attached to pipe %q", s.pipe)
	}
	if want := []string{"one\n", "two\n", "three\n", "four\n"}; !reflect.DeepEqual(s.lines, want) {
		t.Errorf("server got %q, want %q", s.lines, want)
	}

	if err := mm.Error(); err != nil {
		t.Errorf("Error = %v, want nil", err)
	}
	if _, err := io.WriteString(w, "late\n"); err == nil {
		t.Error("write after close succeeded")
	}
}

// TestPipeServerDisconnects checks that the reader and the writer fail when
// the server goes away before the client is done.
func TestPipeServerDisconnects(t *testing.T) {
	base := serve(t, func(_ *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
		enc.Encode("last words\n")
	})

	mm := dial(t, base)

	r, w := mm.Pipe("minimega//foo")
	if r == nil || w == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	got, err := readAll(t, r)
	if err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Errorf("read error = %v, want server disconnected", err)
	}
	if want := "last words\n"; got != want {
		t.Errorf("read %q, want %q", got, want)
	}

	if _, err := io.WriteString(w, "hello\n"); err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Errorf("write error = %v, want server disconnected", err)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Errorf("Error = %v, want server disconnected", err)
	}
}

func TestPipeBadData(t *testing.T) {
	base := serve(t, func(conn *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
		conn.Write([]byte("not json\n"))

		// stay until the client gives up
		io.Copy(io.Discard, conn)
	})

	mm := dial(t, base)

	r, _ := mm.Pipe("minimega//foo")
	if r == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	if _, err := readAll(t, r); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("read error = %v, want a decode error", err)
	}
	if err := mm.Error(); err == nil {
		t.Error("Error = nil")
	}
}

// TestPipeClose checks that Close ends a session whose reader nobody reads,
// so that the reader and the writer fail.
func TestPipeClose(t *testing.T) {
	sent := make(chan struct{})

	base := serve(t, func(conn *net.UnixConn, enc *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)
		enc.Encode("hello\n")
		close(sent)

		// stay until the client closes the connection
		io.Copy(io.Discard, conn)
	})

	mm := dial(t, base)

	r, w := mm.Pipe("minimega//foo")
	if r == nil || w == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	// let the client wait to hand over the line that nobody reads
	<-sent
	time.Sleep(50 * time.Millisecond)

	if err := mm.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	got, err := readAll(t, r)
	if err == nil || got != "" {
		t.Errorf("read %q, %v after Close, want nothing and an error", got, err)
	}
	if _, err := io.WriteString(w, "late\n"); err == nil {
		t.Error("write after Close succeeded")
	}
}

// TestPipeLineTooLong checks that a line that is too long ends the session
// with an error, without sending part of it.
func TestPipeLineTooLong(t *testing.T) {
	lines := make(chan []string, 1)

	base := serve(t, func(conn *net.UnixConn, _ *json.Encoder, dec *json.Decoder) {
		var r Request
		dec.Decode(&r)

		var got []string
		for {
			var line string
			if err := dec.Decode(&line); err != nil {
				break
			}
			got = append(got, line)
		}
		lines <- got
	})

	mm := dial(t, base)

	r, w := mm.Pipe("minimega//foo")
	if r == nil || w == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	go func() {
		bw := bufio.NewWriter(w)
		bw.WriteString("short\n")
		bw.WriteString(strings.Repeat("x", TOKEN_MAX+1))
		bw.Flush()
	}()

	if _, err := readAll(t, r); err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Errorf("read error = %v, want token too long", err)
	}
	if err := mm.Error(); err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Errorf("Error = %v, want token too long", err)
	}

	if got, want := <-lines, []string{"short\n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("server got %d lines, want %q", len(got), want)
	}
}
