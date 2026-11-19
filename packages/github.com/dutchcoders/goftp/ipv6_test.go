package goftp

import (
	"io"
	"net"
	"testing"
)

// The data connection must reach an IPv6 server: before the fix it dialed
// "[:<port>" for a control address of "[::1]:21".
func TestNewConnectionIPv6(t *testing.T) {
	l, err := net.Listen("tcp6", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback: %v", err)
	}
	defer l.Close()

	got := make(chan string, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			got <- err.Error()
			return
		}
		defer c.Close()
		b, _ := io.ReadAll(c)
		got <- string(b)
	}()

	ftp := &FTP{addr: "[::1]:21"}
	conn, err := ftp.newConnection(l.Addr().(*net.TCPAddr).Port)
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("hello"))
	conn.Close()

	if s := <-got; s != "hello" {
		t.Errorf("data connection got %q, want hello", s)
	}
}
