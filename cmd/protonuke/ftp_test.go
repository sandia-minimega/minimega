package main

import (
	"net"
	"testing"
	"time"
)

// ftpClient must reach an IPv6 target: before the fix it dialed "::1:21".
func TestFTPClientIPv6Target(t *testing.T) {
	// ftpClient always dials port 21, so the listener needs a privileged port
	// on the IPv6 loopback
	l, err := net.Listen("tcp6", "[::1]:21")
	if err != nil {
		t.Skipf("cannot listen on [::1]:21: %v", err)
	}
	defer l.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			select {
			case accepted <- struct{}{}:
			default:
			}
			c.Close()
		}
	}()

	go ftpClient()

	select {
	case <-accepted:
	case <-time.After(3 * time.Second):
		t.Fatal("ftpClient never connected to IPv6 target ::1")
	}
}
