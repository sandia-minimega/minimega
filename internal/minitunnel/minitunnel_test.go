// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel_test

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	. "github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"go.uber.org/goleak"
)

func init() {
	log.AddLogger("stderr", os.Stderr, log.DEBUG, true)
}

// pipe returns both ends of a transport and closes them when the test ends,
// which stops the mux of each tunnel.
func pipe(t *testing.T) (net.Conn, net.Conn) {
	g, h := net.Pipe()
	t.Cleanup(func() {
		g.Close()
		h.Close()
	})

	return g, h
}

func goListenAndServe(g net.Conn) chan error {
	err := make(chan error, 1)

	go func() {
		err <- ListenAndServe(g)
	}()

	return err
}

type DummyServer struct {
	net.Listener // embed
	sync.Mutex   // embed

	err error // any error that occurred while being a dummy
}

func NewDummyServer(typ, laddr string) (*DummyServer, error) {
	ln, err := net.Listen(typ, laddr)
	if err != nil {
		return nil, err
	}

	return &DummyServer{Listener: ln}, nil
}

func (d *DummyServer) Expect(input, output string) {
	d.Lock()
	defer d.Unlock()

	rconn, err := d.Accept()
	if err != nil {
		d.err = err
		return
	}
	defer rconn.Close()

	var buf = make([]byte, 10)
	n, err := rconn.Read(buf)
	if err != nil {
		d.err = fmt.Errorf("%v %v %v", err, n, string(buf[:n]))
		return
	}

	if string(buf[:n]) != input {
		d.err = fmt.Errorf("invalid message: `%v` != `%v`", string(buf[:n]), input)
		return
	}

	_, d.err = rconn.Write([]byte(output))
}

type DummyClient struct {
	net.Conn
}

func NewDummyClient(typ, addr string) (*DummyClient, error) {
	conn, err := net.Dial(typ, addr)
	if err != nil {
		return nil, err
	}

	return &DummyClient{conn}, nil
}

func (d *DummyClient) Send(s string) error {
	_, err := d.Write([]byte(s))
	return err
}

func (d *DummyClient) Receive(expect string) error {
	var buf = make([]byte, 10)
	n, err := d.Read(buf)
	if err != nil {
		return fmt.Errorf("%v %v %v", err, n, string(buf[:n]))
	}

	if string(buf[:n]) != expect {
		return fmt.Errorf("invalid message: `%v` != `%v`", string(buf[:n]), expect)
	}

	return nil
}

func TestHandshake(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	_, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestTunnel(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	s, err := NewDummyServer("tcp", ":4445")
	if err != nil {
		t.Fatalf("%v", err)
	}
	go s.Expect("hello", "world")
	defer s.Close()

	err = tun.Forward(4444, "localhost", 4445)
	if err != nil {
		t.Fatalf("%v", err)
	}

	c, err := NewDummyClient("tcp", ":4444")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()

	if err := c.Send("hello"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.Receive("world"); err != nil {
		t.Fatalf("%v", err)
	}

	s.Lock()
	if s.err != nil {
		t.Fatalf("%v", s.err)
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestMultiTunnel(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	s, err := NewDummyServer("tcp", ":4447")
	if err != nil {
		t.Fatalf("%v", err)
	}
	go s.Expect("hello", "world")
	defer s.Close()

	s2, err := NewDummyServer("tcp", ":4449")
	if err != nil {
		t.Fatalf("%v", err)
	}
	go s2.Expect("yellow", "mellow")
	defer s2.Close()

	err = tun.Forward(4446, "localhost", 4447)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = tun.Forward(4448, "localhost", 4449)
	if err != nil {
		t.Fatalf("%v", err)
	}

	c, err := NewDummyClient("tcp", ":4446")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()
	c2, err := NewDummyClient("tcp", ":4448")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c2.Close()

	errs := []error{
		c.Send("hello"),
		c2.Send("yellow"),
		c.Receive("world"),
		c2.Receive("mellow"),
	}
	for _, err := range errs {
		if err != nil {
			t.Fatalf("%v", err)
		}
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestReverse(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	s, err := NewDummyServer("tcp", ":4450")
	if err != nil {
		t.Fatalf("%v", err)
	}
	go s.Expect("hello", "world")
	defer s.Close()

	err = tun.Reverse(4451, "localhost", 4450)
	if err != nil {
		t.Fatalf("%v", err)
	}

	c, err := NewDummyClient("tcp", ":4451")
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer c.Close()

	if err := c.Send("hello"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.Receive("world"); err != nil {
		t.Fatalf("%v", err)
	}

	s.Lock()
	if s.err != nil {
		t.Fatalf("%v", s.err)
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestFowardInvalid(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	err := tun.Forward(-1, "localhost", 450)
	if err == nil {
		t.Fatalf("nil error on Forward")
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestReverseInvalid(t *testing.T) {
	g, h := pipe(t)

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	err := tun.Reverse(-1, "localhost", 450)
	if err == nil {
		t.Fatalf("nil error on Forward")
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestClosedTransportStopsTunnel(t *testing.T) {
	ignore := goleak.IgnoreCurrent()

	g, h := net.Pipe()
	errChan := goListenAndServe(g)

	if _, err := Dial(h); err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if err := <-errChan; err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	h.Close()

	goleak.VerifyNone(t, ignore)
}

func TestListReverses(t *testing.T) {
	g, h := net.Pipe()
	defer g.Close()
	defer h.Close()

	errChan := goListenAndServe(g)

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	if errListen := <-errChan; errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}

	if got := tun.ListReverses(); len(got) != 0 {
		t.Fatalf("ListReverses before Reverse = %v, want none", got)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	src := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if err := tun.Reverse(src, "localhost", 80); err != nil {
		t.Fatalf("Reverse: %v", err)
	}

	// the remote end refuses both of these, so neither is recorded
	if err := tun.Reverse(src, "localhost", 81); err == nil {
		t.Fatal("nil error on Reverse to a port in use")
	}
	if err := tun.Reverse(-1, "localhost", 82); err == nil {
		t.Fatal("nil error on Reverse to an invalid port")
	}

	want := []ReverseTunnel{{Source: src, Host: "localhost", Dest: 80}}
	if got := tun.ListReverses(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ListReverses = %v, want %v", got, want)
	}

	h.Close()

	deadline := time.Now().Add(5 * time.Second)
	for len(tun.ListReverses()) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("ListReverses after close = %v, want none", tun.ListReverses())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// freePort returns a TCP port that nothing listens on.
func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

// forwardTo sets up a tunnel and a forward to a target on a fresh port and
// returns the forwarded port and the target's listener.
func forwardTo(t *testing.T) (int, net.Listener) {
	g, h := pipe(t)
	goListenAndServe(g)

	tun, err := Dial(h)
	if err != nil {
		t.Fatal(err)
	}

	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { target.Close() })

	src := freePort(t)
	if err := tun.Forward(src, "127.0.0.1", target.Addr().(*net.TCPAddr).Port); err != nil {
		t.Fatal(err)
	}

	return src, target
}

// A target that replies and closes must look closed to the forwarded client.
func TestRemoteCloseReachesClient(t *testing.T) {
	src, target := forwardTo(t)

	go func() {
		c, err := target.Accept()
		if err != nil {
			return
		}
		c.Write([]byte("hi"))
		c.Close()
	}()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", src))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))

	buf := make([]byte, 10)
	n, err := c.Read(buf)
	if err != nil || string(buf[:n]) != "hi" {
		t.Fatalf("first read = %q, %v", buf[:n], err)
	}
	if _, err := c.Read(buf); err != io.EOF {
		t.Fatalf("second read error = %v, want EOF", err)
	}
}

// A client that closes must make the tunnel close the connection to the target.
func TestClientCloseReachesTarget(t *testing.T) {
	src, target := forwardTo(t)

	got := make(chan error, 1)
	go func() {
		c, err := target.Accept()
		if err != nil {
			got <- err
			return
		}
		defer c.Close()
		c.SetReadDeadline(time.Now().Add(2 * time.Second))
		buf := make([]byte, 10)
		for {
			if _, err := c.Read(buf); err != nil {
				got <- err
				return
			}
		}
	}()

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", src))
	if err != nil {
		t.Fatal(err)
	}
	c.Write([]byte("x"))
	c.Close()

	if err := <-got; err != io.EOF {
		t.Fatalf("target read error = %v, want EOF", err)
	}
}

// wireMessage mirrors the unexported tunnelMessage so a test can play the
// remote end of the transport by hand.
type wireMessage struct {
	Type   int
	Ack    bool
	TID    int
	Source int
	Host   string
	Port   int
	Error  string
	Data   []byte
}

// A remote end that keeps streaming while the local client closes must not
// crash the process (send on closed channel in the mux).
func TestClientCloseWhileRemoteStreams(t *testing.T) {
	g, h := pipe(t)

	// the hand-written remote end
	enc, dec := gob.NewEncoder(g), gob.NewDecoder(g)
	go func() {
		var hs wireMessage
		if dec.Decode(&hs) != nil {
			return
		}
		if enc.Encode(&wireMessage{Type: HANDSHAKE, Ack: true}) != nil {
			return
		}

		for {
			var m wireMessage
			if dec.Decode(&m) != nil {
				return
			}
			if m.Type != CONNECT {
				continue
			}
			// stream until the transport goes away
			go func(tid int) {
				for enc.Encode(&wireMessage{Type: DATA, TID: tid, Data: []byte("x")}) == nil {
				}
			}(m.TID)
		}
	}()

	tun, err := Dial(h)
	if err != nil {
		t.Fatal(err)
	}

	src := freePort(t)
	if err := tun.Forward(src, "127.0.0.1", 1); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 300; i++ {
		c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", src))
		if err != nil {
			t.Fatal(err)
		}
		c.Read(make([]byte, 1))
		c.Close()
	}
}
