// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func TestCommandSocketSkipsInvalidCommand(t *testing.T) {
	defer func() {
		dnsmasqData = &Dnsmasq{DHCP: map[string]*Dhcp{}, DNS: map[string][]string{}, RAD: map[string]bool{}}
	}()

	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		commandSocketHandle(server)
		close(done)
	}()

	client.SetDeadline(time.Now().Add(5 * time.Second))

	// a command this minirouter does not know must not stop the ones after it
	for _, cmd := range []string{"dnsmasq flush", "no such command", `dnsmasq custom "x=1"`} {
		if _, err := fmt.Fprintln(client, cmd); err != nil {
			t.Fatalf("write %q: %v", cmd, err)
		}
	}
	client.Close()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("command socket handler blocked after an invalid command")
	}
	if len(dnsmasqData.Custom) != 1 || dnsmasqData.Custom[0] != "x=1" {
		t.Errorf("command after the invalid one not applied: %q", dnsmasqData.Custom)
	}
}
