// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import (
	"fmt"
	"math"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// vsockAddr is the net.Addr of one end of a vsock connection.
type vsockAddr struct {
	CID  uint32
	Port uint32
}

func (a vsockAddr) Network() string {
	return "vsock"
}

func (a vsockAddr) String() string {
	return fmt.Sprintf("vsock:%d:%d", a.CID, a.Port)
}

// vsockConn is a connected AF_VSOCK stream socket. The net package cannot
// wrap AF_VSOCK sockets with net.FileConn, so it uses a pollable *os.File.
type vsockConn struct {
	*os.File
	local, remote vsockAddr
}

func (c *vsockConn) LocalAddr() net.Addr {
	return c.local
}

func (c *vsockConn) RemoteAddr() net.Addr {
	return c.remote
}

var _ net.Conn = (*vsockConn)(nil)

// vsockSockaddr returns the address to dial for the given CID and port.
func vsockSockaddr(cid uint, port int) (*unix.SockaddrVM, error) {
	if cid > math.MaxUint32 {
		return nil, fmt.Errorf("invalid vsock CID: %v", cid)
	}
	if port <= 0 || int64(port) > math.MaxUint32 {
		return nil, fmt.Errorf("invalid vsock port: %v", port)
	}

	return &unix.SockaddrVM{CID: uint32(cid), Port: uint32(port)}, nil
}

// dialVsock connects to the -vsock-cid CID (the host by default) on port.
func dialVsock(port int) (net.Conn, error) {
	sa, err := vsockSockaddr(*f_vsockCID, port)
	if err != nil {
		return nil, err
	}

	fd, err := unix.Socket(unix.AF_VSOCK, unix.SOCK_STREAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, os.NewSyscallError("socket", err)
	}

	if err := unix.Connect(fd, sa); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("dial vsock CID %v port %v: %w", sa.CID, sa.Port, err)
	}

	// a non-blocking descriptor makes the *os.File use the runtime poller,
	// which supports deadlines
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, os.NewSyscallError("setnonblock", err)
	}

	c := &vsockConn{
		File:   os.NewFile(uintptr(fd), fmt.Sprintf("vsock:%d:%d", sa.CID, sa.Port)),
		remote: vsockAddr{CID: sa.CID, Port: sa.Port},
	}

	if lsa, err := unix.Getsockname(fd); err == nil {
		if vm, ok := lsa.(*unix.SockaddrVM); ok {
			c.local = vsockAddr{CID: vm.CID, Port: vm.Port}
		}
	}

	return c, nil
}
