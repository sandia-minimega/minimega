// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel

import (
	"net"
	"testing"
	"time"
)

// TestForwardCloseWhileAccepting checks that a forward can add connections
// while another goroutine closes it, and that close closes each added
// connection. Run it with -race.
func TestForwardCloseWhileAccepting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	f := &forward{listener: ln}

	before, peer := net.Pipe()
	defer peer.Close()

	if !f.addConnection(before) {
		t.Fatal("addConnection() = false before close")
	}

	closed := make(chan struct{})
	go func() {
		defer close(closed)
		f.close()
	}()

	late, latePeer := net.Pipe()
	defer late.Close()
	defer latePeer.Close()

	added := f.addConnection(late)
	<-closed

	if !added {
		late.Close()
	}

	// a connection that close closed fails at once
	for name, c := range map[string]net.Conn{"before": before, "late": late} {
		c.SetReadDeadline(time.Now().Add(time.Second))
		if _, err := c.Read(make([]byte, 1)); err == nil || isTimeout(err) {
			t.Errorf("read on the %v connection = %v, want a closed connection", name, err)
		}
	}

	if f.addConnection(late) {
		t.Error("addConnection() = true after close")
	}
}

func isTimeout(err error) bool {
	ne, ok := err.(net.Error)
	return ok && ne.Timeout()
}
