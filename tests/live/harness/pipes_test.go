// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package harness

import (
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
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// fakeMinimega serves the command socket from a new base directory for the
// duration of the test, and points the harness at it. Every connection is
// handled by handler.
func fakeMinimega(t *testing.T, handler func(conn net.Conn, enc *json.Encoder, dec *json.Decoder)) {
	t.Helper()

	// not t.TempDir: a socket path has to be short
	base, err := os.MkdirTemp("", "harness")
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("unix", filepath.Join(base, "minimega"))
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}

			go func() {
				defer conn.Close()
				handler(conn, json.NewEncoder(conn), json.NewDecoder(conn))
			}()
		}
	}()

	oldBase, oldClient := mmBase, client
	mmBase = base

	t.Cleanup(func() {
		mmBase, client = oldBase, oldClient
		l.Close()
		os.RemoveAll(base)
	})
}

func TestPipeClient(t *testing.T) {
	pipes := make(chan string, 1)

	fakeMinimega(t, func(_ net.Conn, enc *json.Encoder, dec *json.Decoder) {
		var r miniclient.Request
		if err := dec.Decode(&r); err != nil {
			return
		}
		pipes <- r.PlumbPipe

		// answer every line, and say goodbye when the client is done writing
		for {
			var line string
			if err := dec.Decode(&line); err != nil {
				break
			}
			enc.Encode(strings.ToUpper(line))
		}
		enc.Encode("bye\n")
	})

	p, err := AttachPipe("foo")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := <-pipes, Namespace+"//foo"; got != want {
		t.Errorf("attached to %q, want %q", got, want)
	}

	if err := p.Write("one", "two"); err != nil {
		t.Fatal(err)
	}
	if err := poll(10*time.Second, time.Millisecond, func() error {
		if len(p.Lines()) != 2 {
			return io.ErrNoProgress
		}
		return nil
	}); err != nil {
		t.Fatalf("read %q, want two lines", p.Lines())
	}

	if err := p.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if got, want := p.Lines(), []string{"ONE\n", "TWO\n", "bye\n"}; !reflect.DeepEqual(got, want) {
		t.Errorf("read %q, want %q", got, want)
	}
}

func TestPipeClientServerDisconnects(t *testing.T) {
	fakeMinimega(t, func(_ net.Conn, _ *json.Encoder, dec *json.Decoder) {
		var r miniclient.Request
		dec.Decode(&r)
	})

	p, err := AttachPipe("foo")
	if err != nil {
		t.Fatal(err)
	}

	// minimega hangs up while the client still has its writer open
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		t.Fatal("client did not notice that the server disconnected")
	}

	if err := p.Close(); err == nil || !strings.Contains(err.Error(), "server disconnected") {
		t.Errorf("Close = %v, want server disconnected", err)
	}
	if err := p.Write("late"); err == nil {
		t.Error("wrote to a closed pipe client")
	}
}

func TestAttachPipeNoServer(t *testing.T) {
	old := mmBase
	mmBase = t.TempDir()
	defer func() { mmBase = old }()

	if _, err := AttachPipe("foo"); err == nil {
		t.Fatal("attached without a server")
	}
}

func TestPipeInfo(t *testing.T) {
	commands := make(chan string, 2)

	fakeMinimega(t, func(_ net.Conn, enc *json.Encoder, dec *json.Decoder) {
		for {
			var r miniclient.Request
			if err := dec.Decode(&r); err != nil {
				return
			}
			commands <- r.Command

			enc.Encode(miniclient.Response{Resp: minicli.Responses{{
				Host:   "minimega",
				Header: []string{"name", "mode", "readers", "writers", "count", "via", "previous"},
				Tabular: [][]string{
					{Namespace + "//bar", "all", "0", "0", "0", "", ""},
					{Namespace + "//foo", "round-robin", "2", "1", "7", "", "hello"},
				},
			}}})
		}
	})

	if err := Connect(); err != nil {
		t.Fatal(err)
	}

	got := PipeInfo(t, "foo")
	want := Row{
		"host": "minimega", "name": Namespace + "//foo", "mode": "round-robin",
		"readers": "2", "writers": "1", "count": "7", "via": "", "previous": "hello",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PipeInfo = %v, want %v", got, want)
	}
	if want := "namespace " + Namespace + " pipe"; <-commands != want {
		t.Errorf("command does not match %q", want)
	}

	if got := PipeInfo(t, "missing"); got != nil {
		t.Errorf("PipeInfo for a pipe that does not exist = %v", got)
	}
}
