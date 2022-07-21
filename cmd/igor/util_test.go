package main

import (
	"net"
	"testing"
)

func TestToPXE(t *testing.T) {
	ip := net.IPv4(192, 168, 2, 91)

	got := toPXE(ip)
	want := "C0A8025B"

	if got != want {
		t.Errorf("%v != %v", got, want)
	}
}
