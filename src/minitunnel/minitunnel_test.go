// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minitunnel_test

import (
	"fmt"
	log "minilog"
	. "minitunnel"
	"net"
	"os"
	"testing"
)

func init() {
	log.AddLogger("stdio", os.Stderr, log.DEBUG, true)
}

type DummyServer struct {
	net.Listener

	err error // any error that occured while being a dummy
}

func NewDummyServer(typ, laddr string) (*DummyServer, error) {
	ln, err := net.Listen(typ, laddr)
	if err != nil {
		return nil, err
	}

	return &DummyServer{ln, nil}, nil
}

func (d *DummyServer) Expect(input, output string) {
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

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

	_, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}
}

func TestTunnel(t *testing.T) {
	g, h := net.Pipe()

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

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

	if s.err != nil {
		t.Fatalf("%v", s.err)
	}
}

func TestMultiTunnel(t *testing.T) {
	g, h := net.Pipe()

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

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
}

func TestReverse(t *testing.T) {
	g, h := net.Pipe()

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

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
}

func TestFowardInvalid(t *testing.T) {
	g, h := net.Pipe()

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	err := tun.Forward(-1, "localhost", 450)
	if err == nil {
		t.Fatalf("nil error on Forward")
	}
}

func TestReverseInvalid(t *testing.T) {
	g, h := net.Pipe()

	go func() {
		errListen := ListenAndServe(g)
		if errListen != nil {
			t.Fatalf("ListenAndServe: %v", errListen)
		}
	}()

	tun, errDial := Dial(h)
	if errDial != nil {
		t.Fatalf("Dial: %v", errDial)
	}

	err := tun.Reverse(-1, "localhost", 450)
	if err == nil {
		t.Fatalf("nil error on Forward")
	}
}
