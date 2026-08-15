// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"net"
	"path/filepath"
	"testing"
	"time"
)

func TestReadUnixSocketSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serial0")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		_, err = conn.Write([]byte("FreeRTOS scheduler running\n"))
		done <- err
	}()

	out, err := readUnixSocketSnapshot(path, time.Second, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "FreeRTOS scheduler running\n" {
		t.Fatalf("unexpected serial output: %q", out)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReadUnixSocketSnapshotHonorsLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serial0")
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.Write([]byte("0123456789"))
	}()

	out, err := readUnixSocketSnapshot(path, time.Second, 4)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "0123" {
		t.Fatalf("unexpected limited output: %q", out)
	}
}
