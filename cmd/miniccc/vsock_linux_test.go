// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux
// +build linux

package main

import "testing"

func TestVsockSockaddr(t *testing.T) {
	sa, err := vsockSockaddr(2, 9002)
	if err != nil {
		t.Fatal(err)
	}
	if sa.CID != 2 || sa.Port != 9002 {
		t.Fatalf("sockaddr = %+v, want CID 2 port 9002", sa)
	}

	for _, tt := range []struct {
		cid  uint
		port int
	}{
		{2, 0},
		{2, -1},
		{2, 1 << 33},
		{1 << 33, 9002},
	} {
		if _, err := vsockSockaddr(tt.cid, tt.port); err == nil {
			t.Errorf("vsockSockaddr(%v, %v): want error", tt.cid, tt.port)
		}
	}
}

func TestHostBoundChannel(t *testing.T) {
	family, serial := *f_family, *f_serial
	defer func() { *f_family, *f_serial = family, serial }()

	for _, tt := range []struct {
		family, serial string
		want           bool
	}{
		{"tcp", "", false},
		{"unix", "", false},
		{"vsock", "", true},
		{"tcp", "/dev/virtio-ports/cc", true},
	} {
		*f_family, *f_serial = tt.family, tt.serial
		if got := hostBoundChannel(); got != tt.want {
			t.Errorf("hostBoundChannel(%v, %q) = %v, want %v", tt.family, tt.serial, got, tt.want)
		}
	}
}
