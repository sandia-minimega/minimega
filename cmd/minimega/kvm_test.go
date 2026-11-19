// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/qmp"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// fakeQMP returns a QMP connection to a server that sends reply for each
// command after the handshake.
func fakeQMP(t *testing.T, reply string) qmp.Conn {
	t.Helper()

	path := filepath.Join(t.TempDir(), "qmp")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			close(accepted)
			return
		}
		accepted <- conn

		fmt.Fprintln(conn, `{"QMP":{}}`)
		dec := json.NewDecoder(conn)
		for {
			var req map[string]interface{}
			if err := dec.Decode(&req); err != nil {
				return
			}
			if req["execute"] == "qmp_capabilities" {
				fmt.Fprintln(conn, `{"return":{}}`)
				continue
			}
			fmt.Fprintln(conn, reply)
		}
	}()

	q, err := qmp.Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	if conn := <-accepted; conn != nil {
		t.Cleanup(func() { conn.Close() })
	}

	return q
}

// TestVMSaveStatusKeepsOtherVMs checks that a VM that cannot answer a QMP
// query, such as a VM that quit, does not hide the status of the other VMs.
func TestVMSaveStatusKeepsOtherVMs(t *testing.T) {
	quit := testVM(KVM, VM_QUIT).(*KvmVM)
	quit.ID, quit.Name = 1, "quit"

	saved := testVM(KVM, VM_RUNNING).(*KvmVM)
	saved.ID, saved.Name = 2, "saved"
	saved.q = fakeQMP(t, `{"return":{"status":"completed"}}`)

	ns := &Namespace{Name: "test"}
	ns.VMs.m = map[int]VM{1: quit, 2: saved}

	resp := &minicli.Response{}
	if err := cliVMSave(ns, minicli.MustCompile("vm save"), resp); err != nil {
		t.Fatalf("vm save: %v", err)
	}

	want := [][]string{{"2", "saved", "completed", "100.00"}}
	if !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("got rows %v, want %v", resp.Tabular, want)
	}
}

func TestParseMigrateStatus(t *testing.T) {
	ram := map[string]interface{}{"total": 200.0, "transferred": 50.0}

	tests := []struct {
		name      string
		reply     map[string]interface{}
		status    string
		completed float64
	}{
		{"no status", map[string]interface{}{}, "", 0},
		{"nil reply", nil, "", 0},
		{"non-string status", map[string]interface{}{"status": 1.0}, "", 0},
		{"none", map[string]interface{}{"status": "none"}, "", 0},
		{"setup", map[string]interface{}{"status": "setup"}, "setup", 0},
		{"cancelling", map[string]interface{}{"status": "cancelling", "ram": ram}, "cancelling", 25},
		{"cancelled", map[string]interface{}{"status": "cancelled"}, "cancelled", 0},
		{"active", map[string]interface{}{"status": "active", "ram": ram}, "active", 25},
		{"postcopy-device", map[string]interface{}{"status": "postcopy-device", "ram": ram}, "postcopy-device", 25},
		{"postcopy-active", map[string]interface{}{"status": "postcopy-active", "ram": ram}, "postcopy-active", 25},
		{"postcopy-paused", map[string]interface{}{"status": "postcopy-paused", "ram": ram}, "postcopy-paused", 25},
		{"postcopy-recover-setup", map[string]interface{}{"status": "postcopy-recover-setup", "ram": ram}, "postcopy-recover-setup", 25},
		{"postcopy-recover", map[string]interface{}{"status": "postcopy-recover", "ram": ram}, "postcopy-recover", 25},
		{"completed", map[string]interface{}{"status": "completed", "ram": ram}, "completed", 100},
		{"completed without ram", map[string]interface{}{"status": "completed"}, "completed", 100},
		{"failing", map[string]interface{}{"status": "failing", "ram": ram}, "failing", 25},
		{"failing without ram", map[string]interface{}{"status": "failing"}, "failing", 0},
		{"failed", map[string]interface{}{"status": "failed"}, "failed", 0},
		{"failed with ram", map[string]interface{}{"status": "failed", "ram": ram}, "failed", 0},
		{"colo", map[string]interface{}{"status": "colo"}, "colo", 0},
		{"pre-switchover", map[string]interface{}{"status": "pre-switchover", "ram": ram}, "pre-switchover", 25},
		{"device", map[string]interface{}{"status": "device", "ram": ram}, "device", 25},
		{"wait-unplug", map[string]interface{}{"status": "wait-unplug"}, "wait-unplug", 0},
		{"unknown status", map[string]interface{}{"status": "future-status", "ram": ram}, "future-status", 25},
		{"active without ram", map[string]interface{}{"status": "active"}, "active", 0},
		{"ram wrong type", map[string]interface{}{"status": "active", "ram": "lots"}, "active", 0},
		{"ram nil", map[string]interface{}{"status": "active", "ram": nil}, "active", 0},
		{
			"total zero",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": 0.0, "transferred": 10.0}},
			"active", 0,
		},
		{
			"total negative",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": -1.0, "transferred": 10.0}},
			"active", 0,
		},
		{
			"total missing",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"transferred": 10.0}},
			"active", 0,
		},
		{
			"total wrong type",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": "100", "transferred": 10.0}},
			"active", 0,
		},
		{
			"transferred missing",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": 100.0}},
			"active", 0,
		},
		{
			"transferred wrong type",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": 100.0, "transferred": int64(10)}},
			"active", 0,
		},
		{
			"transferred complete",
			map[string]interface{}{"status": "active", "ram": map[string]interface{}{"total": 100.0, "transferred": 100.0}},
			"active", 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, completed, err := parseMigrateStatus(tt.reply)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.status {
				t.Errorf("status = %q, want %q", status, tt.status)
			}
			if completed != tt.completed {
				t.Errorf("completed = %v, want %v", completed, tt.completed)
			}
		})
	}
}

func TestKVMQemuArgsDefault(t *testing.T) {
	c := NewVMConfig()
	c.UUID = "11111111-2222-3333-4444-555555555555"

	args := c.qemuArgs(3, "/tmp/minimega/3")
	for _, seq := range [][]string{
		{"-name", "3"},
		{"-m", "2048"},
		{"-nographic"},
		{"-vnc", "unix:/tmp/minimega/3/vnc"},
		{"-smp", "1"},
		{"-qmp", "unix:/tmp/minimega/3/qmp,server=on"},
		{"-vga", "std"},
		{"-rtc", "clock=vm,base=utc"},
		{"-usb"},
		{"-device", "qemu-xhci,id=xhci"},
		{"-device", "usb-tablet,bus=usb-bus.0"},
		{"-pidfile", "/tmp/minimega/3/qemu.pid"},
		{"-cpu", "host"},
		{"-net", "none"},
		{"-S"},
		{"-drive", "media=cdrom"},
		{"-device", "pci-bridge,id=pci.1,chassis_nr=1"},
	} {
		if !hasArgSequence(args, seq...) {
			t.Errorf("missing argument sequence %q in %q", seq, args)
		}
	}

	if n := len(args); n < 2 || args[n-2] != "-uuid" || args[n-1] != c.UUID {
		t.Errorf("arguments do not end with -uuid %v: %q", c.UUID, args)
	}

	joined := strings.Join(args, " ")
	for _, forbidden := range []string{"-mem-path", "-display", "-incoming", "-M ", "-kernel"} {
		if strings.Contains(joined, forbidden) {
			t.Errorf("default arguments contain %q: %s", forbidden, joined)
		}
	}
}
