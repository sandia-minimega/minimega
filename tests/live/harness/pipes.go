// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// PipeClient is attached to a named pipe over a command socket connection of
// its own, the way `minimega -pipe` is. It collects the lines it reads from
// the pipe.
type PipeClient struct {
	conn *miniclient.Conn
	w    io.WriteCloser

	lock  sync.Mutex
	lines []string

	// done is closed when minimega has closed the connection; err is then
	// why, nil if it did so because the client was done writing
	done chan struct{}
	err  error
}

// AttachPipe attaches a new client to a named pipe in the test namespace.
// minimega handles the request after AttachPipe returns: wait until PipeInfo
// lists the client as a reader before writing something it must read.
func AttachPipe(pipe string) (*PipeClient, error) {
	c, err := dial()
	if err != nil {
		return nil, err
	}

	r, w := c.conn.Pipe(Namespace + "//" + pipe)
	if r == nil || w == nil {
		err := c.conn.Error()
		c.conn.Close()
		return nil, fmt.Errorf("attach to pipe %v: %w", pipe, err)
	}

	p := &PipeClient{conn: c.conn, w: w, done: make(chan struct{})}

	go func() {
		defer close(p.done)

		br := bufio.NewReader(r)
		for {
			line, err := br.ReadString('\n')
			if line != "" {
				p.lock.Lock()
				p.lines = append(p.lines, line)
				p.lock.Unlock()
			}
			if err != nil {
				if err != io.EOF {
					p.err = err
				}
				return
			}
		}
	}()

	return p, nil
}

// Write writes lines to the pipe. A newline is added to each.
func (p *PipeClient) Write(lines ...string) error {
	for _, line := range lines {
		if _, err := io.WriteString(p.w, line+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// Lines returns the lines read from the pipe so far, each with its newline.
func (p *PipeClient) Lines() []string {
	p.lock.Lock()
	defer p.lock.Unlock()

	return append([]string{}, p.lines...)
}

// Close tells minimega that the client is done writing and waits for it to
// close the connection, which it does once everything the client wrote is in
// the pipe. It returns the error that ended the session, if any.
func (p *PipeClient) Close() error {
	defer p.conn.Close()

	p.w.Close()

	select {
	case <-p.done:
		return p.err
	case <-time.After(time.Minute):
		return errors.New("minimega did not close the pipe connection")
	}
}

// PipeInfo returns the row for a named pipe from the `pipe` command in the
// test namespace (name, mode, readers, writers, count, via, previous), or nil
// if there is no such pipe.
func PipeInfo(t testing.TB, pipe string) Row {
	t.Helper()

	for _, r := range MustTable(t, "pipe") {
		if r["name"] == Namespace+"//"+pipe {
			return r
		}
	}
	return nil
}
