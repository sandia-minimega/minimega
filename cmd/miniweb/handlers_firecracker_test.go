// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func setFirecrackerVMInfo(t *testing.T) {
	t.Helper()

	old := runTabularFunc
	t.Cleanup(func() { runTabularFunc = old })

	runTabularFunc = func(cmd *Command) []map[string]string {
		return []map[string]string{{"host": "test-host", "type": "firecracker", "vnc_port": "", "console_port": ""}}
	}
}

func TestConnectHandlerFirecracker(t *testing.T) {
	setFirecrackerVMInfo(t)

	for _, path := range []string{"/vm/fc1/connect/", "/vm/fc1/connect/ws"} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()

		connectHandler(w, req, "fc1")

		if w.Code != http.StatusNotFound {
			t.Errorf("%v: got status %d, want %d", path, w.Code, http.StatusNotFound)
		}
		if !strings.Contains(w.Body.String(), "no console for firecracker VMs") {
			t.Errorf("%v: got body %q, want no console message", path, w.Body.String())
		}
	}
}

func TestScreenshotHandlerFirecracker(t *testing.T) {
	setFirecrackerVMInfo(t)

	req := httptest.NewRequest("GET", "/vm/fc1/screenshot.png", nil)
	w := httptest.NewRecorder()

	screenshotHandler(w, req, "fc1")

	if w.Code != http.StatusNotFound {
		t.Errorf("got status %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestConnectWsHandlerFirecrackerDoesNotDial(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port

	srv := httptest.NewServer(websocket.Handler(connectWsHandler("firecracker", "127.0.0.1", port)))
	defer srv.Close()

	ws, err := websocket.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), "", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := ws.Read(make([]byte, 1)); err == nil {
		t.Errorf("Read() = nil, want the server to close the websocket")
	}

	l.(*net.TCPListener).SetDeadline(time.Now().Add(200 * time.Millisecond))
	if c, err := l.Accept(); err == nil {
		c.Close()
		t.Errorf("connectWsHandler(firecracker) dialed 127.0.0.1:%d", port)
	}
}
