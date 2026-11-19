// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// TestCapturePath checks that relative capture filenames land in the files
// directory and absolute ones are kept, for every capture to a file.
func TestCapturePath(t *testing.T) {
	base := *f_iomBase

	for in, want := range map[string]string{
		"sandwich.pcap":      filepath.Join(base, "sandwich.pcap"),
		"dir/sandwich.pcap":  filepath.Join(base, "dir/sandwich.pcap"),
		"/tmp/sandwich.pcap": "/tmp/sandwich.pcap",
	} {
		if got := capturePath(in); got != want {
			t.Errorf("capturePath(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCaptureHandlerPaths runs the VM, pcap and netflow-file handlers with the
// capture start stubbed out (the real one needs a bridge and root) and checks
// the path each hands over.
func TestCaptureHandlerPaths(t *testing.T) {
	var got string
	record := func(p string) error { got = p; return nil }

	origVM, origBr, origNF := captureVM, captureBridge, captureNetflowFile
	t.Cleanup(func() { captureVM, captureBridge, captureNetflowFile = origVM, origBr, origNF })
	captureVM = func(_ *captures, _ VM, _ int, p string, _ bridge.CaptureConfig) (int, error) { return 0, record(p) }
	captureBridge = func(_ *captures, _, p string, _ bridge.CaptureConfig) (int, error) { return 0, record(p) }
	captureNetflowFile = func(_ *captures, _, p string) error { return record(p) }

	vm := &KvmVM{BaseVM: &BaseVM{Name: "vm0"}}
	ns := &Namespace{VMs: VMs{m: map[int]VM{0: vm}}}

	handlers := map[string]struct {
		fn   func(*Namespace, *minicli.Command, *minicli.Response) error
		args map[string]string
		bool map[string]bool
	}{
		"vm":      {cliCaptureVM, map[string]string{"vm": "vm0", "interface": "0"}, nil},
		"pcap":    {cliCapturePcap, map[string]string{"bridge": "br0"}, nil},
		"netflow": {cliCaptureNetflow, map[string]string{"bridge": "br0"}, map[string]bool{"bridge": true}},
	}

	for name, h := range handlers {
		for in, want := range map[string]string{
			"sandwich.pcap":      filepath.Join(*f_iomBase, "sandwich.pcap"),
			"/tmp/sandwich.pcap": "/tmp/sandwich.pcap",
		} {
			c := &minicli.Command{StringArgs: map[string]string{"filename": in}, BoolArgs: h.bool}
			for k, v := range h.args {
				c.StringArgs[k] = v
			}

			got = ""
			if err := h.fn(ns, c, &minicli.Response{}); err != nil {
				t.Fatalf("%v %q: %v", name, in, err)
			}
			if got != want {
				t.Errorf("%v %q wrote to %q, want %q", name, in, got, want)
			}
		}
	}
}

// TestCaptureConfigSnapLen checks that snaplen 0 sets the default and that a
// snaplen out of range fails.
func TestCaptureConfigSnapLen(t *testing.T) {
	ns := &Namespace{}

	for _, tc := range []struct {
		size string
		want uint32
		fail bool
	}{
		{"128", 128, false},
		{"0", 0, false},
		{"13", 0, true},
		{"262145", 0, true},
	} {
		ns.captures.SnapLen = 99
		c := &minicli.Command{StringArgs: map[string]string{"size": tc.size}, BoolArgs: map[string]bool{"snaplen": true}}

		err := cliCaptureConfig(ns, c, &minicli.Response{})
		if tc.fail {
			if err == nil {
				t.Errorf("snaplen %v did not fail", tc.size)
			}
			continue
		}
		if err != nil || ns.captures.SnapLen != tc.want {
			t.Errorf("snaplen %v: got %v, %v", tc.size, ns.captures.SnapLen, err)
		}
	}
}

// TestCaptureBridgeVMs checks that a bridge capture refuses vms= names that
// are not VMs of the namespace.
func TestCaptureBridgeVMs(t *testing.T) {
	var started bool

	orig := captureBridge
	t.Cleanup(func() { captureBridge = orig })
	captureBridge = func(_ *captures, _, _ string, _ bridge.CaptureConfig) (int, error) { started = true; return 0, nil }

	vm := &KvmVM{BaseVM: &BaseVM{ID: 0, Name: "vm0"}}
	ns := &Namespace{Name: "exp", VMs: VMs{m: map[int]VM{0: vm}}}

	for vms, ok := range map[string]bool{"vm0": true, "vm0,vm1": false, "0": false} {
		c := &minicli.Command{
			StringArgs: map[string]string{"bridge": "br0", "filename": "a.pcap"},
			ListArgs:   map[string][]string{"option": {"ports=edge", "vms=" + vms}},
		}

		started = false
		err := cliCapturePcap(ns, c, &minicli.Response{})
		if ok != (err == nil) || ok != started {
			t.Errorf("vms=%v: started %v, err %v", vms, started, err)
		}
	}
}
