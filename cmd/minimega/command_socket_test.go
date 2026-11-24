// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
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

// expectNoLogs fails the test if the code that serves pipes logs anything at
// the warn level or above before the test, and the cleanups registered after
// this call, end. Only lines from the files under test count: goroutines that
// other tests left behind log to the same loggers.
func expectNoLogs(t *testing.T) {
	t.Helper()

	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)

	t.Cleanup(func() {
		log.DelLogger(t.Name())

		for _, line := range strings.Split(logs.String(), "\n") {
			for _, file := range []string{"command_socket.go", "plumber.go", "miniplumber.go"} {
				if strings.Contains(line, " "+file+":") {
					t.Errorf("unexpected warning or error logged: %v", line)
				}
			}
		}
	})
}

// testPlumber replaces the plumber with a new one, without a mesh, for the
// duration of the test.
func testPlumber(t *testing.T) *miniplumber.Plumber {
	t.Helper()

	old := plumber
	plumber = miniplumber.New(nil)
	t.Cleanup(func() { plumber = old })

	return plumber
}

// startCommandSocket serves the command socket from a new base directory, as
// commandSocketStart does, and returns the base directory. When the test ends
// it waits for the connection handlers, which fails the test if one is stuck.
func startCommandSocket(t *testing.T) string {
	t.Helper()

	// not t.TempDir: a socket path has to be short
	base, err := os.MkdirTemp("", "mmtest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) })

	l, err := net.Listen("unix", filepath.Join(base, "minimega"))
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

	return base
}

func dialCommandSocket(t *testing.T, base string) *miniclient.Conn {
	t.Helper()

	mm, err := miniclient.Dial(base)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { mm.Close() })

	return mm
}

// attachPipe attaches a new client to a pipe through the command socket.
func attachPipe(t *testing.T, base, pipe string) (*miniclient.Conn, io.Reader, io.WriteCloser) {
	t.Helper()

	// writers on the pipe, or 0 if there is no such pipe yet
	writers := func() int {
		for _, p := range plumber.Pipes() {
			if p.Name() == pipe {
				return p.NumWriters()
			}
		}
		return 0
	}
	before := writers()

	mm := dialCommandSocket(t, base)

	r, w := mm.Pipe(pipe)
	if r == nil || w == nil {
		t.Fatalf("Pipe: %v", mm.Error())
	}

	// the request is handled once the client is listed as a writer
	waitFor(t, "the client to attach", func() bool {
		return writers() == before+1
	})

	return mm, r, w
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
	case <-time.After(testTimeout):
		t.Fatal("timed out reading from the pipe")
		return "", nil
	}
}

// pipeReader collects what is written to a pipe, like another client would.
type pipeReader struct {
	lock  sync.Mutex
	lines []string
}

func readPipe(t *testing.T, pipe string) *pipeReader {
	t.Helper()

	r := plumber.NewReader(pipe)
	t.Cleanup(r.Close)

	res := &pipeReader{}
	go func() {
		for v := range r.C {
			res.lock.Lock()
			res.lines = append(res.lines, v)
			res.lock.Unlock()
		}
	}()

	return res
}

func (r *pipeReader) Lines() []string {
	r.lock.Lock()
	defer r.lock.Unlock()

	return append([]string{}, r.lines...)
}

// expect waits until the reader has received exactly want.
func (r *pipeReader) expect(t *testing.T, want ...string) {
	t.Helper()

	deadline := time.Now().Add(testTimeout)
	for {
		got := r.Lines()
		if reflect.DeepEqual(got, want) {
			return
		}
		if len(got) > len(want) || time.Now().After(deadline) {
			t.Fatalf("pipe got %q, want %q", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

// writePipe writes to a pipe like another client would, and returns once the
// values are written.
func writePipe(t *testing.T, pipe string, values ...string) {
	t.Helper()

	w, done := plumber.NewWriterDone(pipe)
	for _, v := range values {
		w <- v
	}
	close(w)

	select {
	case <-done:
	case <-time.After(testTimeout):
		t.Fatalf("timed out writing to %v", pipe)
	}
}

func getPipe(t *testing.T, name string) *miniplumber.Pipe {
	t.Helper()

	for _, p := range plumber.Pipes() {
		if p.Name() == name {
			return p
		}
	}

	t.Fatalf("no such pipe: %v", name)
	return nil
}

// TestCommandSocketPipe attaches a client to a pipe, like minimega -pipe, and
// passes lines both ways.
func TestCommandSocketPipe(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	other := readPipe(t, pipe)
	mm, r, w := attachPipe(t, base, pipe)

	p := getPipe(t, pipe)
	if readers, writers := p.NumReaders(), p.NumWriters(); readers != 2 || writers != 1 {
		t.Fatalf("pipe has %d readers and %d writers, want 2 and 1", readers, writers)
	}

	// client to pipe
	if _, err := io.WriteString(w, "one\ntwo\n"); err != nil {
		t.Fatal(err)
	}
	other.expect(t, "one\n", "two\n")

	// pipe to client
	writePipe(t, pipe, "three")
	other.expect(t, "one\n", "two\n", "three\n")

	// The client is done writing. minimega closes the connection once the
	// client's lines are written to the pipe, and the client has read what
	// others wrote to the pipe meanwhile: it doesn't get its own lines back.
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := readAll(t, r)
	if err != nil {
		t.Errorf("read: %v", err)
	}
	if want := "three\n"; got != want {
		t.Errorf("client read %q, want %q", got, want)
	}
	if err := mm.Error(); err != nil {
		t.Errorf("client error: %v", err)
	}

	// the client is detached from the pipe
	if readers, writers := p.NumReaders(), p.NumWriters(); readers != 1 || writers != 0 {
		t.Errorf("pipe has %d readers and %d writers, want 1 and 0", readers, writers)
	}
}

// TestCommandSocketPipeWriteAndClose writes a line to a pipe and closes the
// writer right away, like `echo hello | minimega -pipe foo`. The line must be
// in the pipe by the time the client sees the end of the session, and neither
// side may report an error (sandia-minimega/minimega#905).
func TestCommandSocketPipeWriteAndClose(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	other := readPipe(t, pipe)

	var want []string
	for i := 0; i < 200; i++ {
		line := fmt.Sprintf("hello %d\n", i)
		want = append(want, line)

		mm, err := miniclient.Dial(base)
		if err != nil {
			t.Fatal(err)
		}

		r, w := mm.Pipe(pipe)
		if r == nil || w == nil {
			t.Fatalf("run %d: Pipe: %v", i, mm.Error())
		}

		if _, err := io.WriteString(w, line); err != nil {
			t.Fatalf("run %d: write: %v", i, err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("run %d: close: %v", i, err)
		}

		// the client doesn't get its own line back
		got, err := readAll(t, r)
		if err != nil {
			t.Fatalf("run %d: read: %v", i, err)
		}
		if got != "" {
			t.Fatalf("run %d: client read %q, want nothing", i, got)
		}
		if err := mm.Error(); err != nil {
			t.Fatalf("run %d: client error: %v", i, err)
		}

		// the session is over, so the line is in the pipe
		if got := getPipe(t, pipe).NumMessages(); got != int64(i+1) {
			t.Fatalf("run %d: %d messages written to the pipe, want %d", i, got, i+1)
		}

		mm.Close()
	}

	other.expect(t, want...)

	p := getPipe(t, pipe)
	if readers, writers := p.NumReaders(), p.NumWriters(); readers != 1 || writers != 0 {
		t.Errorf("pipe has %d readers and %d writers, want 1 and 0", readers, writers)
	}
}

// TestCommandSocketPipeClientDisconnects closes the connection without
// reading what the pipe sent, as a client that is killed does, and as
// minimega -pipe did before it waited for minimega to close the connection.
// What the client wrote must reach the pipe, and a client that is gone is not
// an error.
func TestCommandSocketPipeClientDisconnects(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	other := readPipe(t, pipe)

	var want []string
	for i := 0; i < 100; i++ {
		line := fmt.Sprintf("hello %d\n", i)

		conn, err := net.Dial("unix", filepath.Join(base, "minimega"))
		if err != nil {
			t.Fatal(err)
		}

		enc := json.NewEncoder(conn)
		if err := enc.Encode(miniclient.Request{PlumbPipe: pipe}); err != nil {
			t.Fatal(err)
		}
		if err := enc.Encode(line); err != nil {
			t.Fatal(err)
		}

		// every other time, wait for something to be on its way to the client
		if i%2 == 0 {
			waitFor(t, "the client to attach", func() bool {
				return getPipe(t, pipe).NumWriters() > 0
			})
			writePipe(t, pipe, "unread\n")
			want = append(want, "unread\n")
		}

		conn.Close()

		want = append(want, line)
		waitFor(t, "the client to detach", func() bool {
			p := getPipe(t, pipe)
			return p.NumReaders() == 1 && p.NumWriters() == 0 && p.NumMessages() == int64(len(want))
		})
	}

	// the two writers of a run are not ordered
	waitFor(t, "the reader to get every line", func() bool {
		return len(other.Lines()) >= len(want)
	})
	got := other.Lines()
	if len(got) != len(want) {
		t.Fatalf("pipe got %d lines, want %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for _, v := range got {
		seen[v] = true
	}
	for _, v := range want {
		if !seen[v] {
			t.Errorf("pipe did not get %q", v)
		}
	}
}

// TestCommandSocketPipeClients attaches two clients to the same pipe.
func TestCommandSocketPipeClients(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	mmA, rA, wA := attachPipe(t, base, pipe)
	mmB, rB, wB := attachPipe(t, base, pipe)

	if _, err := io.WriteString(wA, "from a\n"); err != nil {
		t.Fatal(err)
	}
	if err := wA.Close(); err != nil {
		t.Fatal(err)
	}

	// a's session ends once its line is written to the pipe
	got, err := readAll(t, rA)
	if err != nil {
		t.Errorf("a: read: %v", err)
	}
	if got != "" {
		t.Errorf("a read %q, want nothing", got)
	}

	if _, err := io.WriteString(wB, "from b\n"); err != nil {
		t.Fatal(err)
	}
	if err := wB.Close(); err != nil {
		t.Fatal(err)
	}

	got, err = readAll(t, rB)
	if err != nil {
		t.Errorf("b: read: %v", err)
	}
	// each client gets the other's lines, not its own
	if want := "from a\n"; got != want {
		t.Errorf("b read %q, want %q", got, want)
	}

	for name, mm := range map[string]*miniclient.Conn{"a": mmA, "b": mmB} {
		if err := mm.Error(); err != nil {
			t.Errorf("%v: client error: %v", name, err)
		}
	}

	p := getPipe(t, pipe)
	if readers, writers := p.NumReaders(), p.NumWriters(); readers != 0 || writers != 0 {
		t.Errorf("pipe has %d readers and %d writers, want none", readers, writers)
	}
}

// TestCommandSocketPipeLongLines sends lines up to the largest size a client
// accepts through a pipe, in both directions.
func TestCommandSocketPipeLongLines(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	other := readPipe(t, pipe)
	mm, r, w := attachPipe(t, base, pipe)

	var want []string
	for _, n := range []int{1, 4096, 64 * 1024, TOKEN_MAX - 1} {
		want = append(want, strings.Repeat("x", n)+"\n")
	}

	// client to pipe
	go func() {
		for _, v := range want {
			io.WriteString(w, v)
		}
	}()
	other.expect(t, want...)

	// pipe to client, which reads until the end of its session
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(r)
		read <- result{data, err}
	}()

	writePipe(t, pipe, want...)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case res := <-read:
		if res.err != nil {
			t.Errorf("read: %v", res.err)
		}
		if got := string(res.data); got != strings.Join(want, "") {
			t.Errorf("client read %d bytes, want %d", len(got), len(strings.Join(want, "")))
		}
	case <-time.After(testTimeout):
		t.Fatal("timed out reading from the pipe")
	}
	if err := mm.Error(); err != nil {
		t.Errorf("client error: %v", err)
	}

	other.expect(t, append(want, want...)...)
}

// TestCommandSocketPipeModes checks that what a client writes to a
// round-robin or random pipe goes to one of the other readers, never back to
// the client, while the client takes its turn for what others write.
func TestCommandSocketPipeModes(t *testing.T) {
	for name, mode := range map[string]int{"round-robin": miniplumber.MODE_RR, "random": miniplumber.MODE_RND} {
		t.Run(name, func(t *testing.T) {
			expectNoLogs(t)
			testPlumber(t)
			base := startCommandSocket(t)

			const pipe = "test//foo"

			plumber.Mode(pipe, mode)

			// no other reader: the lines are discarded, not sent back
			mm, r, w := attachPipe(t, base, pipe)
			if _, err := io.WriteString(w, "nobody reads this\n"); err != nil {
				t.Fatal(err)
			}
			waitFor(t, "the line to be written", func() bool {
				return getPipe(t, pipe).NumMessages() == 1
			})

			a, b := readPipe(t, pipe), readPipe(t, pipe)

			const n = 20
			for i := 0; i < n; i++ {
				if _, err := fmt.Fprintf(w, "from the client %d\n", i); err != nil {
					t.Fatal(err)
				}
			}
			waitFor(t, "the other readers to get the client's lines", func() bool {
				return len(a.Lines())+len(b.Lines()) == n
			})
			if mode == miniplumber.MODE_RR && len(a.Lines()) != n/2 {
				t.Errorf("readers got %d and %d lines, want %d each", len(a.Lines()), len(b.Lines()), n/2)
			}

			// the client is a reader like the others for what someone
			// else writes
			var lines []string
			for i := 0; i < 3*n; i++ {
				lines = append(lines, fmt.Sprintf("from someone else %d\n", i))
			}
			writePipe(t, pipe, lines...)

			if err := w.Close(); err != nil {
				t.Fatal(err)
			}
			got, err := readAll(t, r)
			if err != nil {
				t.Errorf("read: %v", err)
			}
			if strings.Contains(got, "client") {
				t.Errorf("client read its own lines: %q", got)
			}
			if err := mm.Error(); err != nil {
				t.Errorf("client error: %v", err)
			}

			// every line went to exactly one reader
			fromOthers := strings.Count(got, "from someone else")
			waitFor(t, "the readers to get every line", func() bool {
				return fromOthers+len(a.Lines())+len(b.Lines()) >= n+3*n
			})
			if total := fromOthers + len(a.Lines()) + len(b.Lines()); total != n+3*n {
				t.Errorf("readers got %d lines in total, want %d", total, n+3*n)
			}
			if mode == miniplumber.MODE_RR && fromOthers != n {
				t.Errorf("client read %d lines from someone else, want %d", fromOthers, n)
			}
			if mode == miniplumber.MODE_RND && fromOthers == 0 {
				t.Errorf("client read none of %d lines from someone else", 3*n)
			}
		})
	}
}

// TestCommandSocketRequests sends requests that are answered without running
// a command.
func TestCommandSocketRequests(t *testing.T) {
	expectNoLogs(t)
	base := startCommandSocket(t)

	conn, err := net.Dial("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	request := func(r miniclient.Request) miniclient.Response {
		t.Helper()

		if err := enc.Encode(r); err != nil {
			t.Fatalf("send %+v: %v", r, err)
		}

		conn.SetReadDeadline(time.Now().Add(testTimeout))

		var resp miniclient.Response
		if err := dec.Decode(&resp); err != nil {
			t.Fatalf("response to %+v: %v", r, err)
		}
		return resp
	}

	// neither a command nor a suggestion, and both
	for _, r := range []miniclient.Request{{}, {Command: "version", Suggest: "ver"}} {
		resp := request(r)
		if got := resp.Resp.Error(); !strings.Contains(got, "must specify either command or suggest") {
			t.Errorf("response to %+v: error %q", r, got)
		}
		if resp.More {
			t.Errorf("response to %+v: more to come", r)
		}
	}

	// a suggestion
	resp := request(miniclient.Request{Suggest: "clear pip"})
	if want := []string{"clear pipe "}; !reflect.DeepEqual(resp.Suggest, want) {
		t.Errorf("suggestions = %q, want %q", resp.Suggest, want)
	}

	// a command that does not parse
	resp = request(miniclient.Request{Command: "not a minimega command"})
	if got := resp.Resp.Error(); got == "" {
		t.Error("no error for a command that does not parse")
	}

	// a comment is not a command, and gets an empty response
	resp = request(miniclient.Request{Command: "# just a comment"})
	if len(resp.Resp) != 0 || resp.Rendered != "" || resp.More {
		t.Errorf("response to a comment: %+v", resp)
	}
}

// TestCommandSocketBadRequest sends something that is not a request.
func TestCommandSocketBadRequest(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.ERROR, false)
	defer log.DelLogger(t.Name())

	base := startCommandSocket(t)

	conn, err := net.Dial("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}

	// minimega logs the error and hangs up
	conn.SetReadDeadline(time.Now().Add(testTimeout))
	if _, err := conn.Read(make([]byte, 1)); err != io.EOF {
		t.Errorf("read: %v, want EOF", err)
	}

	waitFor(t, "the error to be logged", func() bool {
		return strings.Contains(logs.String(), "invalid character")
	})
}

// TestCommandSocketCommands runs pipe commands through the command socket, as
// `minimega -e` does.
func TestCommandSocketCommands(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	addTestNamespace(t, &Namespace{Name: "test", Hosts: map[string]bool{hostname: true}})
	base := startCommandSocket(t)

	mm := dialCommandSocket(t, base)

	run := func(command string) *miniclient.Response {
		t.Helper()

		var last *miniclient.Response
		for resp := range mm.Run(command) {
			if err := resp.Resp.Error(); err != "" {
				t.Fatalf("%q: %v", command, err)
			}
			last = resp
		}
		if err := mm.Error(); err != nil {
			t.Fatalf("%q: %v", command, err)
		}
		if last == nil {
			t.Fatalf("%q: no response", command)
		}
		return last
	}

	other := readPipe(t, "test//foo")

	run(`namespace test pipe foo "hello from the cli"`)
	other.expect(t, "hello from the cli\n")

	// several commands on one connection
	run("namespace test pipe foo mode round-robin")

	resp := run("namespace test pipe")
	if len(resp.Resp) != 1 {
		t.Fatalf("got %d responses, want 1", len(resp.Resp))
	}
	want := [][]string{{"test//foo", "round-robin", "1", "0", "1", "", "hello from the cli"}}
	if got := resp.Resp[0].Tabular; !reflect.DeepEqual(got, want) {
		t.Errorf("pipes = %q, want %q", got, want)
	}
	if !strings.Contains(resp.Rendered, "test//foo") {
		t.Errorf("rendered response does not list the pipe: %q", resp.Rendered)
	}
}
