// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"net"
	"strings"
	"testing"
)

func TestValidVNCPort(t *testing.T) {
	for _, port := range []uint64{0, 1, 5901, 65535} {
		if err := validVNCPort(VMConfig{}, port); err != nil {
			t.Errorf("validVNCPort(%v) = %v, want nil", port, err)
		}
	}

	if err := validVNCPort(VMConfig{}, 65536); err == nil {
		t.Errorf("validVNCPort(65536) = nil, want error")
	}
}

func TestVNCPortConfigRoundTrip(t *testing.T) {
	c := NewVMConfig()
	c.VNCListenPort = 5901

	var b strings.Builder
	if err := c.WriteConfig(&b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "vm config vnc-port 5901\n") {
		t.Fatalf("saved config missing vnc-port:\n%v", b.String())
	}

	c2 := NewVMConfig()
	if err := c2.ReadConfig(strings.NewReader(b.String()), "test-vnc-port-config"); err != nil {
		t.Fatal(err)
	}
	if c2.VNCListenPort != 5901 {
		t.Fatalf("restored vnc-port = %v, want 5901", c2.VNCListenPort)
	}

	if v, err := c2.KVMConfig.Info("vnc-port"); err != nil || v != "5901" {
		t.Fatalf("Info(vnc-port) = %q, %v", v, err)
	}

	c2.Clear("vnc-port")
	if c2.VNCListenPort != 0 {
		t.Fatalf("clear vm config vnc-port left %v", c2.VNCListenPort)
	}
}

// freePort returns a TCP port that is free on all interfaces right now.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestReserveVNCPortAcrossNamespaces(t *testing.T) {
	port := freePort(t)
	newVM := func(name, namespace string) *KvmVM {
		return &KvmVM{
			BaseVM:    &BaseVM{Name: name, Namespace: namespace},
			KVMConfig: KVMConfig{VNCListenPort: uint64(port)},
		}
	}

	a := newVM("a", "ns-a")
	if err := reserveVNCPort(a); err != nil {
		t.Fatal(err)
	}
	defer releaseVNCPort(a)

	// same host, different namespace
	b := newVM("b", "ns-b")
	if err := reserveVNCPort(b); err == nil || !strings.Contains(err.Error(), "already reserved by vm a in namespace ns-a") {
		t.Fatalf("second reservation: got %v, want already reserved", err)
	}

	// releasing a VM that does not hold the port must not free it
	releaseVNCPort(b)
	if err := reserveVNCPort(b); err == nil {
		t.Fatalf("port freed by a VM that did not hold it")
	}

	releaseVNCPort(a)
	if err := reserveVNCPort(b); err != nil {
		t.Fatalf("reservation after release: %v", err)
	}
	releaseVNCPort(b)

	if err := reserveVNCPort(newVM("unset", "ns-a")); err != nil {
		t.Fatal(err)
	}
}

func TestVNCPortAndAndroidPortsDoNotOverlap(t *testing.T) {
	// a fixed VNC port inside the Android console range is skipped by the Android allocator
	port := -1
	for p := MinAndroidConsolePort; p <= MaxAndroidConsolePort; p += 2 {
		if !androidReservedPort[p] && !androidReservedPort[p+1] && tcpPortAvailable(p) && tcpPortAvailable(p+1) {
			port = p
			break
		}
	}
	if port < 0 {
		t.Skip("no free Android console port pair on this host")
	}

	vm := &KvmVM{BaseVM: &BaseVM{Name: "vnc"}, KVMConfig: KVMConfig{VNCListenPort: uint64(port)}}
	if err := reserveVNCPort(vm); err != nil {
		t.Fatal(err)
	}
	defer releaseVNCPort(vm)

	console, _, err := reserveAndroidPortPair(uint64(port))
	if err == nil {
		defer releaseAndroidPortPair(console)
		if console == port {
			t.Fatalf("android allocator handed out reserved vnc port %v", port)
		}
	}

	// and a fixed VNC port cannot take a port an Android VM holds
	releaseVNCPort(vm)
	console, adb, err := reserveAndroidPortPair(uint64(port))
	if err != nil {
		t.Skipf("android port reservation: %v", err)
	}
	defer releaseAndroidPortPair(console)
	vm2 := &KvmVM{BaseVM: &BaseVM{Name: "vnc2"}, KVMConfig: KVMConfig{VNCListenPort: uint64(adb)}}
	if err := reserveVNCPort(vm2); err == nil || !strings.Contains(err.Error(), "android") {
		releaseVNCPort(vm2)
		t.Fatalf("vnc reservation of android adb port %v: got %v, want android error", adb, err)
	}
}

func TestAndroidRejectsVNCPort(t *testing.T) {
	c := NewVMConfig()
	c.AVDName = "test-avd"
	c.VNCListenPort = 5901
	if _, err := NewAndroid("droid", "test", c); err != errAndroidVNCPort {
		t.Fatalf("NewAndroid with vnc-port: got %v, want %v", err, errAndroidVNCPort)
	}

	n := &Namespace{Name: "test-vnc-port-android"}
	if err := n.Queue("droid", ANDROID, c); err != errAndroidVNCPort {
		t.Fatalf("Queue android with vnc-port: got %v, want %v", err, errAndroidVNCPort)
	}
}

func TestQueueRejectsMultipleVMsWithVNCPort(t *testing.T) {
	c := NewVMConfig()
	c.VNCListenPort = 5901

	n := &Namespace{Name: "test-vnc-port-queue"}
	if err := n.Queue("vm[1-2]", KVM, c); err == nil || !strings.Contains(err.Error(), "VNC port") {
		t.Fatalf("Queue of two VMs with a vnc-port = %v, want error", err)
	}
}

func TestBaremetalRejectsVNCPort(t *testing.T) {
	c := NewVMConfig()
	c.Baremetal = true
	c.Backchannel = false
	c.KernelPath = "/firmware/RTOSDemo.out"
	c.VNCListenPort = 5901
	if _, err := NewKVM("freertos", "test", c); err == nil || !strings.Contains(err.Error(), "VNC") {
		t.Fatalf("expected VNC error, got %v", err)
	}
}

func TestConnectVNCFixedPort(t *testing.T) {
	port := freePort(t)

	// the shim goroutine looks up the namespace, which needs a running mesh to create
	registerNamespace(t, "test-vnc-port-shim")

	vm := &KvmVM{
		BaseVM:    &BaseVM{Name: "vnc", Namespace: "test-vnc-port-shim"},
		KVMConfig: KVMConfig{VNCListenPort: uint64(port)},
	}
	if err := vm.connectVNC(); err != nil {
		t.Fatal(err)
	}
	defer vm.vncShim.Close()

	if vm.VNCPort != port {
		t.Fatalf("VNCPort = %v, want %v", vm.VNCPort, port)
	}

	// a second VM asking for the same port must fail to bind
	vm2 := &KvmVM{
		BaseVM:    &BaseVM{Name: "vnc2", Namespace: "test-vnc-port-shim"},
		KVMConfig: KVMConfig{VNCListenPort: uint64(port)},
	}
	if err := vm2.connectVNC(); err == nil {
		vm2.vncShim.Close()
		t.Fatalf("second shim bound in-use port %v", port)
	}
}
