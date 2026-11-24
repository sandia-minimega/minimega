// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qmp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeQMP accepts one connection, completes the QMP handshake, and answers one
// human-monitor-command with reply. It sends the result on the returned channel.
func fakeQMP(t *testing.T, reply string) (string, <-chan error) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "qmp")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	done := make(chan error, 1)
	go func() {
		done <- serveQMP(l, reply)
	}()

	return path, done
}

func serveQMP(l net.Listener, reply string) error {
	conn, err := l.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	dec := json.NewDecoder(conn)
	if _, err := fmt.Fprintln(conn, `{"QMP":{"version":{"qemu":{"micro":0,"minor":0,"major":8}},"capabilities":[]}}`); err != nil {
		return err
	}

	var req map[string]interface{}
	if err := dec.Decode(&req); err != nil {
		return err
	}
	if req["execute"] != "qmp_capabilities" {
		return fmt.Errorf("got %v, want qmp_capabilities", req["execute"])
	}
	if _, err := fmt.Fprintln(conn, `{"return":{}}`); err != nil {
		return err
	}

	req = nil
	if err := dec.Decode(&req); err != nil {
		return err
	}
	if req["execute"] != "human-monitor-command" {
		return fmt.Errorf("got %v, want human-monitor-command", req["execute"])
	}
	args, _ := req["arguments"].(map[string]interface{})
	if args["command-line"] != "info status" {
		return fmt.Errorf("got command-line %v, want info status", args["command-line"])
	}

	_, err = fmt.Fprintln(conn, reply)
	return err
}

func TestHumanMonitorCommand(t *testing.T) {
	tests := []struct {
		name    string
		reply   string
		want    string
		wantErr string
	}{
		{
			name:  "text",
			reply: `{"return":"VM status: running\r\n"}`,
			want:  "VM status: running\r\n",
		},
		{
			name:    "error",
			reply:   `{"error":{"class":"GenericError","desc":"device not found"}}`,
			wantErr: "device not found",
		},
		{
			name:    "error not a map",
			reply:   `{"error":"boom"}`,
			wantErr: "human-monitor-command: boom",
		},
		{
			name:    "non-string return",
			reply:   `{"return":{}}`,
			wantErr: "unexpected response type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, done := fakeQMP(t, tt.reply)

			q, err := Dial(path)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			t.Cleanup(func() { q.conn.Close() })

			got, err := q.HumanMonitorCommand("info status")
			if serr := <-done; serr != nil {
				t.Fatalf("fake server: %v", serr)
			}

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("HumanMonitorCommand: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// serveOverlap completes the QMP handshake and answers two
// human-monitor-command requests with their command lines. It closes first
// when it has the first request. It returns an error when the second request
// comes before the reply to the first request.
func serveOverlap(l net.Listener, first chan<- struct{}) error {
	conn, err := l.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()

	r := bufio.NewReader(conn)
	if _, err := fmt.Fprintln(conn, `{"QMP":{}}`); err != nil {
		return err
	}
	if _, err := r.ReadBytes('\n'); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(conn, `{"return":{}}`); err != nil {
		return err
	}

	for i := 0; i < 2; i++ {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return err
		}
		var req struct {
			Arguments struct {
				CommandLine string `json:"command-line"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			return err
		}

		if i == 0 {
			close(first)

			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			if _, err := r.Peek(1); err == nil {
				return errors.New("got the second request before the reply to the first request")
			}
			conn.SetReadDeadline(time.Time{})
		}

		reply, err := json.Marshal(map[string]string{"return": req.Arguments.CommandLine})
		if err != nil {
			return err
		}
		if _, err := conn.Write(append(reply, '\n')); err != nil {
			return err
		}
	}

	return nil
}

// TestRequestsDoNotOverlap checks that a request waits for the reply to the
// request before it. A QMP reply does not name its request, so callers that
// overlap can get the replies of each other.
func TestRequestsDoNotOverlap(t *testing.T) {
	hmp := func(q *Conn) (string, error) {
		return q.HumanMonitorCommand("b")
	}
	raw := func(q *Conn) (string, error) {
		return q.Raw(`{"execute":"human-monitor-command","arguments":{"command-line":"b"}}` + "\n")
	}

	tests := []struct {
		name   string
		second func(*Conn) (string, error)
		want   string
	}{
		{"command", hmp, "b"},
		{"raw", raw, `{"return":"b"}` + "\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "qmp")
			l, err := net.Listen("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { l.Close() })

			first := make(chan struct{})
			served := make(chan error, 1)
			go func() {
				served <- serveOverlap(l, first)
			}()

			q, err := Dial(path)
			if err != nil {
				t.Fatal(err)
			}

			results := make(chan error, 2)
			call := func(name, want string, f func(*Conn) (string, error)) {
				got, err := f(&q)
				switch {
				case err != nil:
					results <- fmt.Errorf("%v request: %v", name, err)
				case got != want:
					results <- fmt.Errorf("%v request got the reply %q, want %q", name, got, want)
				default:
					results <- nil
				}
			}

			go call("first", "a", func(q *Conn) (string, error) {
				return q.HumanMonitorCommand("a")
			})
			select {
			case <-first:
			case err := <-served:
				t.Fatalf("server stopped before the first request: %v", err)
			}
			go call("second", tt.want, tt.second)

			if err := <-served; err != nil {
				t.Error(err)
			}
			for i := 0; i < 2; i++ {
				if err := <-results; err != nil {
					t.Error(err)
				}
			}
		})
	}
}
