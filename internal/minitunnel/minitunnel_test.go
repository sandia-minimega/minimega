// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minitunnel_test

import (
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	. "github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func init() {
	log.AddLogger("stderr", os.Stderr, log.DEBUG, true)
}

func goListenAndServe(g net.Conn) chan error {
	err := make(chan error)

	go func() {
		err <- ListenAndServe(g)
	}()

	return err
}

type DummyServer struct {
	net.Listener // embed
	sync.Mutex   // embed

	err error // any error that occured while being a dummy
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
	g, h := net.Pipe()

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
	g, h := net.Pipe()

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
	g, h := net.Pipe()

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
	g, h := net.Pipe()

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

	if s.err != nil {
		t.Fatalf("%v", s.err)
	}

	errListen := <-errChan
	if errListen != nil {
		t.Fatalf("ListenAndServe: %v", errListen)
	}
}

func TestFowardInvalid(t *testing.T) {
	g, h := net.Pipe()

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
	g, h := net.Pipe()

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
