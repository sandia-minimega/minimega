package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	"github.com/sandia-minimega/minimega/v2/pkg/miniclient"
)

// fakeMinimega serves the miniclient protocol on <dir>/minimega. Commands
// containing "screenshot" get an error response with More=true followed by a
// final response; everything else gets one final response.
func fakeMinimega(t *testing.T) {
	dir := t.TempDir()
	l, err := net.Listen("unix", filepath.Join(dir, "minimega"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				dec, enc := json.NewDecoder(c), json.NewEncoder(c)
				for {
					var req miniclient.Request
					if dec.Decode(&req) != nil {
						return
					}
					if strings.Contains(req.Command, "die") {
						return
					}
					if strings.Contains(req.Command, "screenshot") {
						enc.Encode(miniclient.Response{Resp: minicli.Responses{{Error: "boom"}}, More: true})
					}
					enc.Encode(miniclient.Response{Resp: minicli.Responses{{Response: "ok"}}})
				}
			}()
		}
	}()

	old, oldmm := *f_base, mm
	*f_base, mm = dir, nil
	t.Cleanup(func() { *f_base, mm = old, oldmm })
}

// An error response with More=true makes screenshotHandler return early; the
// shared connection must remain usable afterwards.
func TestScreenshotHandlerEarlyReturnKeepsConnectionUsable(t *testing.T) {
	fakeMinimega(t)

	oldRunTabular := runTabularFunc
	t.Cleanup(func() { runTabularFunc = oldRunTabular })
	runTabularFunc = func(*Command) []map[string]string {
		return []map[string]string{{"type": "kvm"}}
	}

	rec := httptest.NewRecorder()
	screenshotHandler(rec, httptest.NewRequest(http.MethodGet, "/vm/a/screenshot.png", nil), "a")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}

	done := make(chan string, 1)
	go func() {
		var out string
		for r := range run(&Command{Command: "version"}) {
			out += r.Rendered + r.Resp[0].Response
		}
		done <- out
	}()
	select {
	case out := <-done:
		if out != "ok" {
			t.Fatalf("second command = %q, want ok", out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second command hung: connection lock still held by abandoned Run")
	}
}

// After minimega drops the connection mid-command, the next run() must redial.
func TestRunRedialsAfterServerDisconnect(t *testing.T) {
	fakeMinimega(t)

	for range run(&Command{Command: "die"}) {
	}

	var out string
	for r := range run(&Command{Command: "version"}) {
		out += r.Resp[0].Response
	}
	if out != "ok" {
		t.Fatalf("command after disconnect = %q, want ok", out)
	}
}

// A console whose websocket never connects must not leak the minimega
// -attach process or its pty.
func TestConsoleReapedWhenWebsocketNeverConnects(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-minimega")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 60\n"), 0755); err != nil {
		t.Fatal(err)
	}

	oldConsole, oldTimeout := *f_console, consoleAttachTimeout
	t.Cleanup(func() { *f_console, consoleAttachTimeout = oldConsole, oldTimeout })
	*f_console, consoleAttachTimeout = script, 200*time.Millisecond

	consoleHandler(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/console", nil))

	ptyMu.Lock()
	var pid int
	for p := range ptys {
		pid = p
	}
	ptyMu.Unlock()
	if pid == 0 {
		t.Fatal("console was not registered")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ptyMu.Lock()
		n := len(ptys)
		ptyMu.Unlock()
		if n == 0 && syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("console %v still registered or process alive after timeout", pid)
}

// A request for a console that is gone (e.g. reaped) must not leave ptyMu
// locked, or every later console request and reaper blocks.
func TestConsoleMissingPtyReleasesLock(t *testing.T) {
	w := httptest.NewRecorder()
	consoleHandler(w, httptest.NewRequest(http.MethodGet, "/console/999999/size?rows=24&cols=80", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}

	if !ptyMu.TryLock() {
		t.Fatal("ptyMu still locked after missing-pty 404")
	}
	ptyMu.Unlock()
}
