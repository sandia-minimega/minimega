// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"strings"
	"testing"
)

func hasArgSequence(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		match := true
		for j := range want {
			if args[i+j] != want[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestBaremetalQemuArgs(t *testing.T) {
	c := NewVMConfig()
	c.Baremetal = true
	c.Backchannel = false
	c.Memory = 16
	c.VCPUs = 1
	c.Machine = "mps2-an385"
	c.CPU = "cortex-m3"
	c.KernelPath = "/firmware/RTOSDemo.out"
	c.SerialPorts = 1
	c.UUID = "11111111-2222-3333-4444-555555555555"

	args := c.qemuArgs(7, "/tmp/minimega/7")
	for _, seq := range [][]string{
		{"-display", "none"},
		{"-M", "mps2-an385"},
		{"-cpu", "cortex-m3"},
		{"-kernel", "/firmware/RTOSDemo.out"},
		{"-serial", "chardev:charserial0"},
		{"-qmp", "unix:/tmp/minimega/7/qmp,server=on"},
		{"-net", "none"},
	} {
		if !hasArgSequence(args, seq...) {
			t.Errorf("missing argument sequence %q in %q", seq, args)
		}
	}

	joined := strings.Join(args, " ")
	for _, forbidden := range []string{"-vnc", "-vga", "-usb", "usb-tablet", "pci-bridge", "media=cdrom", "isa-serial"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("bare-metal arguments contain %q: %s", forbidden, joined)
		}
	}
}

func TestBaremetalNetworkUsesBoardNIC(t *testing.T) {
	c := NewVMConfig()
	c.Baremetal = true
	c.Backchannel = false
	c.BaremetalNetworkDriver = "lan9118"
	c.KernelPath = "/firmware/RTOSDemo.out"
	c.Networks = NetConfigs{{
		Tap:    "mega_tap0",
		MAC:    "52:54:00:12:34:ad",
		Driver: "lan9118",
	}}

	args := c.qemuArgs(8, "/tmp/minimega/8")
	if !hasArgSequence(args, "-netdev", "tap,id=mega_tap0,script=no,ifname=mega_tap0") {
		t.Fatalf("missing bare-metal tap arguments: %q", args)
	}
	if !hasArgSequence(args, "-net", "nic,model=lan9118,netdev=mega_tap0,macaddr=52:54:00:12:34:ad") {
		t.Fatalf("missing bare-metal NIC arguments: %q", args)
	}
	if hasArgSequence(args, "-net", "none") {
		t.Fatalf("networked bare-metal VM unexpectedly disables networking: %q", args)
	}
	if strings.Contains(strings.Join(args, " "), "pci-bridge") {
		t.Fatalf("bare-metal network unexpectedly uses PCI: %q", args)
	}
}

func TestBaremetalValidation(t *testing.T) {
	c := NewVMConfig()
	c.Baremetal = true
	c.Backchannel = false
	if _, err := NewKVM("freertos", "test", c); err == nil || !strings.Contains(err.Error(), "firmware") {
		t.Fatalf("expected missing firmware error, got %v", err)
	}

	c.KernelPath = "/firmware/RTOSDemo.out"
	c.Backchannel = true
	if _, err := NewKVM("freertos", "test", c); err == nil || !strings.Contains(err.Error(), "backchannel") {
		t.Fatalf("expected backchannel error, got %v", err)
	}

	c.Backchannel = false
	c.Networks = NetConfigs{{Driver: "lan9118"}}
	if _, err := NewKVM("freertos", "test", c); err == nil || !strings.Contains(err.Error(), "NIC driver") {
		t.Fatalf("expected missing bare-metal NIC driver error, got %v", err)
	}
}

func TestBaremetalSkipsVNC(t *testing.T) {
	vm := &KvmVM{KVMConfig: KVMConfig{Baremetal: true}}
	if err := vm.connectVNC(); err != nil {
		t.Fatal(err)
	}
	if vm.vncShim != nil || vm.VNCPort != 0 {
		t.Fatalf("bare-metal VM created a VNC shim: listener=%v port=%d", vm.vncShim, vm.VNCPort)
	}
}

func TestBaremetalConfigReadPreservesBoardNIC(t *testing.T) {
	const saved = `vm config qemu minimega-test-qemu-not-found
vm config machine mps2-an385
vm config kernel /firmware/RTOSDemo.out
vm config baremetal true
vm config baremetal-network-driver lan9118
vm config backchannel false
vm config networks 378,52:54:00:12:34:ad,lan9118
`

	c := NewVMConfig()
	if err := c.ReadConfig(strings.NewReader(saved), "test-baremetal-config-read"); err != nil {
		t.Fatal(err)
	}
	if len(c.Networks) != 1 {
		t.Fatalf("expected one restored network, got %d", len(c.Networks))
	}
	if got := c.Networks[0].Driver; got != "lan9118" {
		t.Fatalf("unexpected restored network driver: %q", got)
	}
}
