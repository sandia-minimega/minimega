// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minitunnel_test

import (
	. "minitunnel"
	"net"
	"testing"
)

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

	ln, err := net.Listen("tcp", ":4445")
	if err != nil {
		t.Fatalf("%v", err)
	}

	go func() {
		rconn, err := ln.Accept()
		if err != nil {
			t.Fatalf("%v", err)
		}
		var buf = make([]byte, 10)
		n, err := rconn.Read(buf)
		if err != nil {
			t.Fatalf("%v %v %v", err, n, string(buf[:n]))
		}
		if string(buf[:n]) != "hello" {
			t.Fatalf("invalid message: %v", string(buf[:n]))
		}
		_, err = rconn.Write([]byte("world"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		rconn.Close()
	}()

	err = tun.Forward(4444, "localhost", 4445)
	if err != nil {
		t.Fatalf("%v", err)
	}

	conn, err := net.Dial("tcp", ":4444")
	if err != nil {
		t.Fatalf("%v", err)
	}
	_, err = conn.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("%v", err)
	}
	var buf = make([]byte, 10)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("%v %v %v", err, n, string(buf[:n]))
	}
	if string(buf[:n]) != "world" {
		t.Fatalf("invalid message: %v", string(buf[:n]))
	}
	conn.Close()
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

	ln, err := net.Listen("tcp", ":4447")
	if err != nil {
		t.Fatalf("%v", err)
	}

	ln2, err := net.Listen("tcp", ":4449")
	if err != nil {
		t.Fatalf("%v", err)
	}

	go func() {
		rconn, err := ln.Accept()
		if err != nil {
			t.Fatalf("%v", err)
		}
		var buf = make([]byte, 10)
		n, err := rconn.Read(buf)
		if err != nil {
			t.Fatalf("%v %v %v", err, n, string(buf[:n]))
		}
		if string(buf[:n]) != "hello" {
			t.Fatalf("invalid message: %v", string(buf[:n]))
		}
		_, err = rconn.Write([]byte("world"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		rconn.Close()
	}()

	go func() {
		rconn, err := ln2.Accept()
		if err != nil {
			t.Fatalf("%v", err)
		}
		var buf = make([]byte, 10)
		n, err := rconn.Read(buf)
		if err != nil {
			t.Fatalf("%v %v %v", err, n, string(buf[:n]))
		}
		if string(buf[:n]) != "yellow" {
			t.Fatalf("invalid message: %v", string(buf[:n]))
		}
		_, err = rconn.Write([]byte("mellow"))
		if err != nil {
			t.Fatalf("%v", err)
		}
		rconn.Close()
	}()

	err = tun.Forward(4446, "localhost", 4447)
	if err != nil {
		t.Fatalf("%v", err)
	}

	err = tun.Forward(4448, "localhost", 4449)
	if err != nil {
		t.Fatalf("%v", err)
	}

	conn, err := net.Dial("tcp", ":4446")
	if err != nil {
		t.Fatalf("%v", err)
	}
	conn2, err := net.Dial("tcp", ":4448")
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = conn.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("%v", err)
	}

	_, err = conn2.Write([]byte("yellow"))
	if err != nil {
		t.Fatalf("%v", err)
	}

	var buf = make([]byte, 10)
	var buf2 = make([]byte, 10)

	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("%v %v %v", err, n, string(buf[:n]))
	}

	if string(buf[:n]) != "world" {
		t.Fatalf("invalid message: %v", string(buf[:n]))
	}

	n, err = conn2.Read(buf2)
	if err != nil {
		t.Fatalf("%v %v %v", err, n, string(buf2[:n]))
	}

	if string(buf2[:n]) != "mellow" {
		t.Fatalf("invalid message: %v", string(buf2[:n]))
	}
	conn.Close()
	conn2.Close()
}
