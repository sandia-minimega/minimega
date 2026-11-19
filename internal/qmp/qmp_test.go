// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qmp

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"testing"
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
