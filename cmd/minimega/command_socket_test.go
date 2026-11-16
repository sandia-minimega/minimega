package main

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

func statusChanCount() int {
	meshageStatusLock.RLock()
	defer meshageStatusLock.RUnlock()
	return len(meshageStatusChans)
}

func waitStatusChans(t *testing.T, want int) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if statusChanCount() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("status channels registered = %d, want %d", statusChanCount(), want)
}

// socketClient talks to commandSocketHandle over an in-memory connection.
type socketClient struct {
	c   net.Conn
	enc *json.Encoder
	dec *json.Decoder
}

func newSocketClient(t *testing.T) *socketClient {
	s, c := net.Pipe()
	go commandSocketHandle(s)
	return &socketClient{c, json.NewEncoder(c), json.NewDecoder(c)}
}

// run sends cmd and returns the final (non-More) response.
func (s *socketClient) run(t *testing.T, cmd string) miniclient.Response {
	t.Helper()
	if err := s.enc.Encode(&miniclient.Request{Command: cmd}); err != nil {
		t.Fatal(err)
	}
	for {
		var r miniclient.Response
		s.c.SetReadDeadline(time.Now().Add(5 * time.Second))
		if err := s.dec.Decode(&r); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !r.More {
			return r
		}
	}
}

func TestCommandSocketStatusRegistration(t *testing.T) {
	a, b := newSocketClient(t), newSocketClient(t)
	for i := 0; i < 3; i++ {
		a.run(t, "echo hi")
		b.run(t, "echo hi")
	}

	// one status channel per live connection, not one per command and not
	// clobbered by the other connection
	waitStatusChans(t, 2)

	a.c.Close()
	waitStatusChans(t, 1)
	b.c.Close()
	waitStatusChans(t, 0)
}

func TestCommandSocketNoStaleStatus(t *testing.T) {
	a := newSocketClient(t)
	defer a.c.Close()
	a.run(t, "echo hi")

	// a status message arrives while no command is in flight
	meshageStatusLock.RLock()
	for _, c := range meshageStatusChans {
		select {
		case c <- "stale":
		default:
		}
	}
	meshageStatusLock.RUnlock()
	time.Sleep(100 * time.Millisecond)

	// the next command must be answered by its own response
	if r := a.run(t, "echo hi"); r.Status || len(r.Resp) == 0 {
		t.Fatalf("got stale status instead of response: %+v", r)
	}
}
