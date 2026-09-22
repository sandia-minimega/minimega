// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// serveDropThenRespond drops the first connection after it reads a request,
// as minimega does when it exits mid-request, and answers on the second.
func serveDropThenRespond(l net.Listener) error {
	for i := 0; i < 2; i++ {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		var req miniclient.Request
		if err := json.NewDecoder(conn).Decode(&req); err != nil {
			conn.Close()
			return fmt.Errorf("decode request %d: %v", i, err)
		}

		if i == 1 {
			err = json.NewEncoder(conn).Encode(miniclient.Response{Rendered: "ok"})
		}
		conn.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func TestRunRedialsAfterDisconnectDuringRequest(t *testing.T) {
	// Keep the socket path short: unix socket paths are limited to ~100 bytes.
	dir, err := os.MkdirTemp("", "miniweb")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	l, err := net.Listen("unix", filepath.Join(dir, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	oldBase := *f_base
	*f_base = dir
	t.Cleanup(func() {
		mmMu.Lock()
		defer mmMu.Unlock()

		if mm != nil {
			mm.Close()
		}
		mm = nil
		*f_base = oldBase
	})

	serverErr := make(chan error, 1)
	go func() { serverErr <- serveDropThenRespond(l) }()

	for resp := range run(&Command{Command: "version"}) {
		t.Fatalf("got response %q from a request the server dropped", resp.Rendered)
	}

	var got []string
	for resp := range run(&Command{Command: "version"}) {
		got = append(got, resp.Rendered)
	}

	if len(got) != 1 || got[0] != "ok" {
		t.Fatalf("got responses %q after the server returned, want [\"ok\"]", got)
	}

	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
}
