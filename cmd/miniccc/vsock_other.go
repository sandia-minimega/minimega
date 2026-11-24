// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build !linux
// +build !linux

package main

import "net"

func dialVsock(port int) (net.Conn, error) {
	return nil, errVsockUnsupported
}
