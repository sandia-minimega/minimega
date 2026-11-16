// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"io"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

// serverGreeting is the first message of the fake VNC server.
const serverGreeting = "RFB 003.008\n"

// When the remote side closes, connectWsHandler closes the websocket, so the
// browser sees the end without a write of its own.
func TestConnectWsHandlerRemoteClose(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		c.Write([]byte(serverGreeting))
		c.Close()
	}()

	port := l.Addr().(*net.TCPAddr).Port
	readUntilClose(t, websocket.Handler(connectWsHandler("kvm", "127.0.0.1", port)), serverGreeting)
}

// When the pty closes, consoleWsHandler closes the websocket.
func TestConsoleWsHandlerRemoteClose(t *testing.T) {
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	syscall.CloseOnExec(fds[0])
	syscall.CloseOnExec(fds[1])
	tty := os.NewFile(uintptr(fds[0]), "tty")
	peer := os.NewFile(uintptr(fds[1]), "peer")

	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	go func() {
		peer.WriteString("$ ")
		peer.Close()
	}()

	readUntilClose(t, websocket.Handler(consoleWsHandler(tty, cmd.Process.Pid)), "$ ")
}

// readUntilClose connects to h and reads the websocket until the server
// closes it. It checks that the data is want.
func readUntilClose(t *testing.T, h websocket.Handler, want string) {
	t.Helper()

	srv := httptest.NewServer(h)
	defer srv.Close()

	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, err := websocket.Dial(url, "", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()

	ws.SetReadDeadline(time.Now().Add(5 * time.Second))

	got, err := io.ReadAll(ws)
	if err != nil {
		t.Fatalf("read websocket: %v (want EOF after the remote side closes)", err)
	}
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
