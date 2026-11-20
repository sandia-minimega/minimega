// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// pipeSeq numbers the pipes of the tests.
var pipeSeq atomic.Int64

// pipeName returns a name for a pipe that no other test, and no other run of
// the same test, uses: miniccc has one plumber, which outlives a test.
func pipeName(t *testing.T, name string) string {
	return fmt.Sprintf("%v-%v-%d", t.Name(), name, pipeSeq.Add(1))
}

// TestRunCommandStdoutPipe runs short commands whose standard output and
// standard error are pipes. Every line must go upstream, also when the
// command exits right after it wrote the line.
func TestRunCommandStdoutPipe(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	u := newUpstream(t)

	for i := 0; i < 200; i++ {
		out, errs := pipeName(t, "out"), pipeName(t, "err")

		stdout, stderr, code := runCommand(&ron.Command{
			Command: []string{"sh", "-c", "echo to-stdout; echo to-stderr >&2"},
			Stdout:  out,
			Stderr:  errs,
		})
		if stdout != "" || stderr != "" || code != 0 {
			t.Fatalf("run %d: stdout %q, stderr %q, exit code %d", i, stdout, stderr, code)
		}

		u.expect(t, out, "new writer", `data "to-stdout\n"`, "close writer")
		u.expect(t, errs, "new writer", `data "to-stderr\n"`, "close writer")
	}

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}

// TestRunCommandStdinPipe runs a command whose standard input is a pipe.
// miniccc gives up the pipe when the command has exited.
func TestRunCommandStdinPipe(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	u := newUpstream(t)

	in, out := pipeName(t, "in"), pipeName(t, "out")

	type result struct {
		stdout, stderr string
		code           int
	}
	res := make(chan result, 1)

	go func() {
		// head exits after the first line
		stdout, stderr, code := runCommand(&ron.Command{
			Command: []string{"head", "-n", "1"},
			Stdin:   in,
			Stdout:  out,
		})
		res <- result{stdout, stderr, code}
	}()

	u.expect(t, in, "new reader")
	pipeMessage(&ron.Message{Type: ron.MESSAGE_PIPE, Pipe: in, PipeMode: ron.PIPE_DATA, PipeData: "one\n"})

	select {
	case r := <-res:
		if r.stdout != "" || r.stderr != "" || r.code != 0 {
			t.Errorf("stdout %q, stderr %q, exit code %d", r.stdout, r.stderr, r.code)
		}
	case <-time.After(testTimeout):
		t.Fatal("command did not exit")
	}

	u.expect(t, out, "new writer", `data "one\n"`, "close writer")
	u.expect(t, in, "new reader", "close reader")

	// lines for the command that is gone don't block miniccc
	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		for i := 0; i < 10; i++ {
			pipeMessage(&ron.Message{Type: ron.MESSAGE_PIPE, Pipe: in, PipeMode: ron.PIPE_DATA, PipeData: "late\n"})
		}
	}()

	select {
	case <-wrote:
	case <-time.After(testTimeout):
		t.Fatal("writes to the pipe of a command that exited block")
	}

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}

// TestRunCommandBackgroundPipe runs a short command in the background whose
// standard output is a pipe. Its output goes upstream, and miniccc forgets
// the process when it has exited.
func TestRunCommandBackgroundPipe(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	u := newUpstream(t)

	client.Lock()
	client.Processes = map[int]*Process{}
	client.Unlock()
	defer func() {
		client.Lock()
		client.Processes = nil
		client.Unlock()
	}()

	for i := 0; i < 50; i++ {
		out := pipeName(t, "out")

		stdout, stderr, code := runCommand(&ron.Command{
			Command:    []string{"echo", "to-stdout"},
			Stdout:     out,
			Background: true,
		})
		if stdout != "" || stderr != "" || code != 0 {
			t.Fatalf("run %d: stdout %q, stderr %q, exit code %d", i, stdout, stderr, code)
		}

		u.expect(t, out, "new writer", `data "to-stdout\n"`, "close writer")
	}

	processes := func() int {
		client.Lock()
		defer client.Unlock()
		return len(client.Processes)
	}
	for deadline := time.Now().Add(testTimeout); processes() != 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("miniccc still lists %d processes after they exited", processes())
		}
	}

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}

// TestRunCommandOutputHeldOpen runs commands that exit while a process that
// they started still has their output open. miniccc sends the output of the
// command upstream and does not wait for the other process.
func TestRunCommandOutputHeldOpen(t *testing.T) {
	logs := &syncBuffer{}
	log.AddLogger(t.Name(), logs, log.WARN, false)
	defer log.DelLogger(t.Name())

	defer func(d time.Duration) { outputWait = d }(outputWait)
	outputWait = 100 * time.Millisecond

	u := newUpstream(t)

	client.Lock()
	client.Processes = map[int]*Process{}
	client.Unlock()
	defer func() {
		client.Lock()
		client.Processes = nil
		client.Unlock()
	}()

	// the process that the command leaves behind runs until the file is gone
	hold := filepath.Join(t.TempDir(), "hold")
	if err := os.WriteFile(hold, nil, 0600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(hold)

	script := fmt.Sprintf("echo to-stdout; echo to-stderr >&2; while [ -e %v ]; do sleep 0.1; done &", hold)

	for _, background := range []bool{false, true} {
		out, errs := pipeName(t, "out"), pipeName(t, "err")

		res := make(chan [2]string, 1)
		go func() {
			stdout, stderr, code := runCommand(&ron.Command{
				Command:    []string{"sh", "-c", script},
				Stdout:     out,
				Stderr:     errs,
				Background: background,
			})
			res <- [2]string{stdout + stderr, fmt.Sprint(code)}
		}()

		select {
		case r := <-res:
			if r != [2]string{"", "0"} {
				t.Errorf("background %v: output %q, exit code %v", background, r[0], r[1])
			}
		case <-time.After(testTimeout):
			t.Fatalf("background %v: miniccc waits for the process that has the output open", background)
		}

		u.expect(t, out, "new writer", `data "to-stdout\n"`, "close writer")
		u.expect(t, errs, "new writer", `data "to-stderr\n"`, "close writer")
	}

	// the command of the background run has exited, so miniccc forgets it
	processes := func() int {
		client.Lock()
		defer client.Unlock()
		return len(client.Processes)
	}
	for deadline := time.Now().Add(testTimeout); processes() != 0; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("miniccc still lists %d processes after they exited", processes())
		}
	}

	if s := logs.String(); s != "" {
		t.Errorf("unexpected warnings or errors logged:\n%v", s)
	}
}
