// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// runPlumbCLI compiles a plumb or pipe command and calls its handler in ns.
func runPlumbCLI(t *testing.T, ns *Namespace, command string) (*minicli.Response, error) {
	t.Helper()

	c, err := minicli.Compile(command)
	if err != nil {
		t.Fatalf("%q does not parse: %v", command, err)
	}

	// pick the handler the way the patterns in plumbCLIHandlers do
	var fn func(*Namespace, *minicli.Command, *minicli.Response) error

	switch _, local := c.StringArgs["src"]; {
	case strings.HasPrefix(command, "clear plumb"):
		fn = cliPlumbClear
	case strings.HasPrefix(command, "clear pipe"):
		fn = cliPipeClear
	case strings.HasPrefix(command, "plumb") && local:
		fn = cliPlumbLocal
	case strings.HasPrefix(command, "plumb"):
		fn = cliPlumbBroadcast
	case strings.HasPrefix(command, "pipe") && (c.StringArgs["data"] != "" || c.BoolArgs["via"]):
		fn = cliPipeLocal
	case strings.HasPrefix(command, "pipe"):
		fn = cliPipeBroadcast
	default:
		t.Fatalf("%q is not a plumb or pipe command", command)
	}

	resp := &minicli.Response{}
	return resp, fn(ns, c, resp)
}

func mustPlumbCLI(t *testing.T, ns *Namespace, command string) *minicli.Response {
	t.Helper()

	resp, err := runPlumbCLI(t, ns, command)
	if err != nil {
		t.Fatalf("%q: %v", command, err)
	}
	return resp
}

// pipeTable runs `pipe` in ns and returns the rows by pipe name.
func pipeTable(t *testing.T, ns *Namespace) map[string][]string {
	t.Helper()

	resp := mustPlumbCLI(t, ns, "pipe")
	if want := []string{"name", "mode", "readers", "writers", "count", "via", "previous"}; !reflect.DeepEqual(resp.Header, want) {
		t.Fatalf("pipe header = %v, want %v", resp.Header, want)
	}

	rows := map[string][]string{}
	for _, row := range resp.Tabular {
		rows[row[0]] = row
	}
	return rows
}

func TestFqnsPipe(t *testing.T) {
	ns := &Namespace{Name: "test"}

	for pipe, want := range map[string]string{
		"foo":        "test//foo",
		"other//foo": "other//foo",
	} {
		if got := fqnsPipe(ns, pipe); got != want {
			t.Errorf("fqnsPipe(%q) = %q, want %q", pipe, got, want)
		}
	}
}

func TestCLIPipe(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)

	ns := &Namespace{Name: "test"}

	// a pipe is created by the first write, even without readers
	mustPlumbCLI(t, ns, `pipe foo "hello pipes!"`)

	// the write counts, and its writer goes away, shortly after
	want := []string{"test//foo", "all", "0", "0", "1", "", "hello pipes!"}
	waitFor(t, "the write", func() bool {
		return reflect.DeepEqual(pipeTable(t, ns)["test//foo"], want)
	})

	// a reader sees the next write
	other := readPipe(t, "test//foo")
	mustPlumbCLI(t, ns, `pipe foo "second message"`)
	other.expect(t, "second message\n")

	for mode, want := range map[string]string{"round-robin": "round-robin", "random": "random", "all": "all"} {
		mustPlumbCLI(t, ns, "pipe foo mode "+mode)
		if got := pipeTable(t, ns)["test//foo"][1]; got != want {
			t.Errorf("mode = %q, want %q", got, want)
		}
	}

	mustPlumbCLI(t, ns, "pipe foo mode random")
	mustPlumbCLI(t, ns, "clear pipe foo mode")
	if got := pipeTable(t, ns)["test//foo"][1]; got != "all" {
		t.Errorf("mode = %q after clear, want all", got)
	}

	mustPlumbCLI(t, ns, "pipe foo log true")
	mustPlumbCLI(t, ns, "pipe foo log false")
	mustPlumbCLI(t, ns, "clear pipe foo log")

	// Deleting a pipe closes its readers. A pipe cannot be deleted while it
	// has writers, and the writer of the message above goes away shortly
	// after the message is delivered.
	waitFor(t, "the writer to finish", func() bool {
		return pipeTable(t, ns)["test//foo"][3] == "0"
	})
	mustPlumbCLI(t, ns, "clear pipe foo")
	if got := pipeTable(t, ns); len(got) != 0 {
		t.Errorf("pipes after clear: %v", got)
	}
	if _, err := runPlumbCLI(t, ns, "clear pipe foo"); err == nil {
		t.Error("cleared a pipe that does not exist")
	}

	mustPlumbCLI(t, ns, "pipe a mode all")
	mustPlumbCLI(t, ns, "pipe b mode all")
	mustPlumbCLI(t, ns, "clear pipe")
	if got := pipeTable(t, ns); len(got) != 0 {
		t.Errorf("pipes after clear: %v", got)
	}
}

func TestCLIPipeVia(t *testing.T) {
	if _, err := exec.LookPath("tr"); err != nil {
		t.Skip("tr not found")
	}

	expectNoLogs(t)
	testPlumber(t)

	ns := &Namespace{Name: "test"}

	mustPlumbCLI(t, ns, "pipe foo via tr a-z A-Z")
	if got := pipeTable(t, ns)["test//foo"][5]; got != "tr a-z A-Z" {
		t.Errorf("via = %q", got)
	}

	other := readPipe(t, "test//foo")
	mustPlumbCLI(t, ns, "pipe foo hello")
	other.expect(t, "HELLO\n")

	mustPlumbCLI(t, ns, "clear pipe foo via")
	if got := pipeTable(t, ns)["test//foo"][5]; got != "" {
		t.Errorf("via = %q after clear", got)
	}

	mustPlumbCLI(t, ns, "pipe foo hello")
	other.expect(t, "HELLO\n", "hello\n")
}

// TestCLIPipeNamespaces checks that pipes and pipelines are listed in their
// own namespace only, also when one namespace's name ends with another's.
func TestCLIPipeNamespaces(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)

	live := &Namespace{Name: "live"}
	alive := &Namespace{Name: "alive"}
	e := &Namespace{Name: "e"}

	mustPlumbCLI(t, live, "pipe foo mode all")
	mustPlumbCLI(t, alive, "pipe bar mode all")

	// a pipe in another namespace is named in full
	mustPlumbCLI(t, live, "pipe alive//baz mode all")

	if got, want := pipeTable(t, live), []string{"live//foo"}; len(got) != 1 || got[want[0]] == nil {
		t.Errorf("pipes in live = %v, want %v", got, want)
	}
	if got := pipeTable(t, alive); len(got) != 2 || got["alive//bar"] == nil || got["alive//baz"] == nil {
		t.Errorf("pipes in alive = %v, want alive//bar and alive//baz", got)
	}

	path := filepath.Join(t.TempDir(), "out")
	mustPlumbCLI(t, live, "plumb foo file://"+path)
	mustPlumbCLI(t, alive, "plumb bar baz")
	mustPlumbCLI(t, alive, "plumb other//in out")

	pipelines := func(ns *Namespace) []string {
		var res []string
		for _, row := range mustPlumbCLI(t, ns, "plumb").Tabular {
			res = append(res, row[0])
		}
		return res
	}

	if got, want := pipelines(live), []string{"live//foo file://" + path}; !reflect.DeepEqual(got, want) {
		t.Errorf("pipelines in live = %q, want %q", got, want)
	}
	if got, want := pipelines(alive), []string{"alive//bar alive//baz", "other//in alive//out"}; !reflect.DeepEqual(got, want) {
		t.Errorf("pipelines in alive = %q, want %q", got, want)
	}
	// neither live//foo nor file:///... is a pipe in namespace e
	if got := pipelines(e); len(got) != 0 {
		t.Errorf("pipelines in %v = %q, want none", e, got)
	}
	if got := pipeTable(t, e); len(got) != 0 {
		t.Errorf("pipes in %v = %v, want none", e, got)
	}

	mustPlumbCLI(t, live, "clear plumb")
	waitFor(t, "the pipelines to stop", func() bool {
		return len(plumber.Pipelines()) == 0
	})
}

func TestCLIPlumb(t *testing.T) {
	if _, err := exec.LookPath("cat"); err != nil {
		t.Skip("cat not found")
	}

	expectNoLogs(t)
	testPlumber(t)

	ns := &Namespace{Name: "test"}

	// pipes get the namespace, programs and file terminals do not
	path := filepath.Join(t.TempDir(), "out")
	mustPlumbCLI(t, ns, "plumb in cat mid")
	mustPlumbCLI(t, ns, "plumb mid file://"+path)

	if _, err := runPlumbCLI(t, ns, "plumb in cat mid"); err == nil {
		t.Error("created the same pipeline twice")
	}

	resp := mustPlumbCLI(t, ns, "plumb")
	want := [][]string{{"test//in cat test//mid"}, {"test//mid file://" + path}}
	if !reflect.DeepEqual(resp.Tabular, want) {
		t.Fatalf("pipelines = %q, want %q", resp.Tabular, want)
	}

	waitFor(t, "the pipelines to start", func() bool {
		pipes := pipeTable(t, ns)
		return pipes["test//in"] != nil && pipes["test//in"][2] == "1" &&
			pipes["test//mid"] != nil && pipes["test//mid"][2] == "1" && pipes["test//mid"][3] == "1"
	})

	mustPlumbCLI(t, ns, `pipe in "through the pipeline"`)

	waitFor(t, "the message to reach the file", func() bool {
		data, _ := os.ReadFile(path)
		return string(data) == "through the pipeline\n"
	})

	// delete one pipeline by name, the other with the rest
	mustPlumbCLI(t, ns, "clear plumb test//in cat test//mid")
	if _, err := runPlumbCLI(t, ns, "clear plumb test//in cat test//mid"); err == nil {
		waitFor(t, "the pipeline to stop", func() bool {
			return len(plumber.Pipelines()) == 1
		})
	}

	mustPlumbCLI(t, ns, "clear plumb")
	waitFor(t, "the pipelines to stop", func() bool {
		return len(plumber.Pipelines()) == 0
	})
}

// TestPipeMM runs what `minimega -pipe` does with its standard input and
// output against the command socket.
func TestPipeMM(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	other := readPipe(t, pipe)

	// echo hello | minimega -pipe foo
	for i := 0; i < 50; i++ {
		var out bytes.Buffer

		mm := dialCommandSocket(t, base)
		if err := pipeMM(mm, pipe, strings.NewReader("hello\nworld\n"), &out); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}

		// the lines are in the pipe when the client is done
		if got := getPipe(t, pipe).NumMessages(); got != int64(2*(i+1)) {
			t.Fatalf("run %d: %d messages written to the pipe, want %d", i, got, 2*(i+1))
		}
		// the client doesn't get its own lines back
		if got := out.String(); got != "" {
			t.Fatalf("run %d: client printed %q, want nothing", i, got)
		}

		mm.Close()
	}

	waitFor(t, "the reader to get every line", func() bool {
		return len(other.Lines()) >= 100
	})
	if got := other.Lines(); len(got) != 100 {
		t.Errorf("pipe got %d lines, want 100", len(got))
	}
}

// TestPipeMMReads attaches a client that has nothing to write until its input
// is closed, like `minimega -pipe foo` on a terminal.
func TestPipeMMReads(t *testing.T) {
	expectNoLogs(t)
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	stdin, stdinW := io.Pipe()
	var out syncBuffer

	done := make(chan error, 1)
	mm := dialCommandSocket(t, base)
	go func() {
		done <- pipeMM(mm, pipe, stdin, &out)
	}()

	waitFor(t, "the client to attach", func() bool {
		for _, p := range plumber.Pipes() {
			if p.Name() == pipe {
				return p.NumReaders() == 1 && p.NumWriters() == 1
			}
		}
		return false
	})

	writePipe(t, pipe, "one", "two")
	waitFor(t, "the client to print the lines", func() bool {
		return out.String() == "one\ntwo\n"
	})

	select {
	case err := <-done:
		t.Fatalf("client returned before its input was closed: %v", err)
	default:
	}

	// ^D
	stdinW.Close()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("client: %v", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("client did not return when its input was closed")
	}
}

// TestPipeMMServerDisconnects checks that the client fails when minimega goes
// away, without waiting for its input to be closed.
func TestPipeMMServerDisconnects(t *testing.T) {
	base, err := os.MkdirTemp("", "mmtest")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(base)

	l, err := net.Listen("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// accept the request, send a line, and hang up
		var r miniclient.Request
		json.NewDecoder(conn).Decode(&r)
		json.NewEncoder(conn).Encode("goodbye\n")
	}()

	mm := dialCommandSocket(t, base)

	// an input that is never closed
	stdin, stdinW := io.Pipe()
	defer stdinW.Close()

	var out syncBuffer

	done := make(chan error, 1)
	go func() {
		done <- pipeMM(mm, "test//foo", stdin, &out)
	}()

	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "server disconnected") {
			t.Errorf("client error = %v, want server disconnected", err)
		}
	case <-time.After(testTimeout):
		t.Fatal("client did not return when the server disconnected")
	}

	if got, want := out.String(), "goodbye\n"; got != want {
		t.Errorf("client printed %q, want %q", got, want)
	}
}

// TestPipeMMResult checks that a session that ends cleanly still returns an
// error from its input, also when both results are ready at once.
func TestPipeMMResult(t *testing.T) {
	inputErr := errors.New("input failed")

	for _, tc := range []struct {
		out, in error
		want    error
	}{
		{nil, nil, nil},
		{nil, inputErr, inputErr},
		{io.ErrClosedPipe, nil, io.ErrClosedPipe},
	} {
		// select chooses at random between the results that are ready
		for i := 0; i < 100; i++ {
			outErr := make(chan error, 1)
			inErr := make(chan error, 1)
			outErr <- tc.out
			inErr <- tc.in

			if err := pipeMMResult(outErr, inErr); err != tc.want {
				t.Fatalf("output %v, input %v: error = %v, want %v", tc.out, tc.in, err, tc.want)
			}
		}
	}

	// an output error does not wait for an input that is never closed
	outErr := make(chan error, 1)
	outErr <- io.ErrClosedPipe
	if err := pipeMMResult(outErr, make(chan error)); err != io.ErrClosedPipe {
		t.Fatalf("error = %v, want %v", err, io.ErrClosedPipe)
	}
}

// failWriter fails every write.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestPipeMMErrors(t *testing.T) {
	testPlumber(t)
	base := startCommandSocket(t)

	const pipe = "test//foo"

	// the connection is already closed
	mm := dialCommandSocket(t, base)
	mm.Close()
	if err := pipeMM(mm, pipe, strings.NewReader(""), io.Discard); err == nil {
		t.Error("no error on a closed connection")
	}

	// the output cannot be written to, with an input that is never closed
	mm = dialCommandSocket(t, base)
	stdin, stdinW := io.Pipe()
	defer stdinW.Close()

	done := make(chan error, 1)
	go func() {
		done <- pipeMM(mm, pipe, stdin, failWriter{})
	}()

	waitFor(t, "the client to attach", func() bool {
		for _, p := range plumber.Pipes() {
			if p.Name() == pipe {
				return p.NumReaders() == 1
			}
		}
		return false
	})
	writePipe(t, pipe, "hello")

	select {
	case err := <-done:
		if err != io.ErrClosedPipe {
			t.Errorf("error = %v, want %v", err, io.ErrClosedPipe)
		}
	case <-time.After(testTimeout):
		t.Fatal("client did not return when it could not write its output")
	}
	mm.Close()

	// a line that is too long to send
	mm = dialCommandSocket(t, base)
	long := strings.NewReader(strings.Repeat("x", TOKEN_MAX+1) + "\n")
	if err := pipeMM(mm, pipe, long, io.Discard); err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Errorf("error = %v, want token too long", err)
	}
	mm.Close()
}
