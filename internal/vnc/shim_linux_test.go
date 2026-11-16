// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// pollQEMU polls the fake QEMU end of a unix socket, with no read, until
// one of the events in want occurs. It returns the events.
func pollQEMU(t *testing.T, c net.Conn, want int16) int16 {
	t.Helper()

	sc, err := c.(*net.UnixConn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		var revents int16
		var perr error
		err := sc.Control(func(fd uintptr) {
			fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLRDHUP}}
			_, perr = unix.Poll(fds, 10)
			revents = fds[0].Revents
		})
		if err != nil {
			t.Fatal(err)
		}
		if perr != nil && !errors.Is(perr, unix.EINTR) {
			t.Fatal(perr)
		}
		if revents&want != 0 {
			return revents
		}
	}

	t.Fatal("timeout waiting for the shim to end the QEMU input")
	return 0
}

// QEMU stops a VNC client at POLLHUP and does not read the input that waits
// (vnc_client_io in ui/vnc.c). When a client writes one key event and closes,
// as "vnc inject" does, the QEMU socket must not get POLLHUP before QEMU
// reads the key event.
func TestProxyClientCloseNoHangup(t *testing.T) {
	r := startQEMUProxy(t, ProxyOptions{Name: "vm-" + t.Name()})

	key := encode(t, &KeyEvent{DownFlag: 1, Key: 'a'})
	if _, err := r.client.Write(key); err != nil {
		t.Fatal(err)
	}
	r.client.Close()

	if ev := pollQEMU(t, r.server, unix.POLLRDHUP|unix.POLLHUP); ev&unix.POLLHUP != 0 {
		t.Fatalf("QEMU socket events %#x include POLLHUP before QEMU reads, so QEMU drops the key event", ev)
	}

	got, err := io.ReadAll(r.server)
	if err != nil {
		t.Fatalf("QEMU read: %v", err)
	}
	if !bytes.Equal(got, key) {
		t.Fatalf("QEMU read % x, want the key event % x", got, key)
	}
	r.server.Close()

	if err := r.wait(t); !errors.Is(err, ErrClientClosed) {
		t.Fatalf("Proxy returned %v, want ErrClientClosed", err)
	}
}
