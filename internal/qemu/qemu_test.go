// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package qemu

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testdata returns the content of a file in testdata; see testdata/README.md.
func testdata(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// headerWords are words from QEMU help headers that are never valid values.
var headerWords = []string{"x86", "Available", "Supported", "Recognized", "Network", "name"}

func checkParsed(t *testing.T, res map[string]bool, want, notWant []string) {
	t.Helper()

	for _, w := range want {
		if !res[w] {
			t.Errorf("missing %q in %v", w, res)
		}
	}

	for _, w := range append(notWant, headerWords...) {
		if res[w] {
			t.Errorf("unexpected %q in %v", w, res)
		}
	}
}

func TestParseCPUs(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		want    []string
		notWant []string
		count   int
	}{
		{
			name:    "legacy",
			out:     cpusOut,
			want:    []string{"486", "qemu64", "max", "host", "base", "Broadwell", "Broadwell-noTSX", "pentium"},
			notWant: []string{"fpu", "CPUID"},
			count:   34,
		},
		{
			name:    "QEMU 11.1.3",
			out:     cpusOutQEMU11,
			want:    []string{"486", "486-v1", "pentium-v1", "qemu64", "qemu64-v1", "max", "base", "Broadwell", "Broadwell-IBRS", "Cascadelake-Server-noTSX", "Skylake-Server-noTSX-IBRS", "kvm64"},
			notWant: []string{"3dnow", "x2apic", "zero-fcs-fds", "CPUID", "(alias", "Intel"},
			count:   30,
		},
		{
			name:    "QEMU 8.2.2",
			out:     testdata(t, "qemu-8.2.2-cpu-help.txt"),
			want:    []string{"486", "486-v1", "pentium-v1", "qemu64", "qemu64-v1", "max", "base", "Broadwell", "Broadwell-IBRS", "Cascadelake-Server-noTSX", "Skylake-Server-noTSX-IBRS", "kvm64", "EPYC-Genoa"},
			notWant: []string{"3dnow", "x2apic", "xtpr", "CPUID", "(alias", "Intel"},
			count:   144,
		},
		{
			name:  "ARM",
			out:   cpusOutARM,
			want:  []string{"cortex-a15", "cortex-m3", "arm1026", "ti925t"},
			count: 31,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseCPUs(strings.NewReader(tt.out))
			if err != nil {
				t.Fatalf("failed: %v", err)
			}

			checkParsed(t, res, tt.want, tt.notWant)
			if len(res) != tt.count {
				t.Errorf("parsed %v cpus, want %v: %v", len(res), tt.count, res)
			}
		})
	}
}

func TestParseMachines(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		want  []string
		count int
	}{
		{
			name:  "legacy",
			out:   machinesOut,
			want:  []string{"pc", "pc-i440fx-2.11", "q35", "pc-q35-2.11", "isapc", "none", "xenpv"},
			count: 40,
		},
		{
			name:  "QEMU 11.1.3",
			out:   machinesOutQEMU11,
			want:  []string{"pc", "pc-i440fx-11.1", "pc-i440fx-8.1", "pc-i440fx-5.2", "q35", "pc-q35-11.1", "microvm", "isapc", "none"},
			count: 20,
		},
		{
			name:  "QEMU 8.2.2",
			out:   testdata(t, "qemu-8.2.2-machine-help.txt"),
			want:  []string{"pc", "pc-i440fx-8.2", "pc-i440fx-noble", "ubuntu", "q35", "pc-q35-8.2", "pc-q35-noble", "microvm", "isapc", "none"},
			count: 134,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseMachines(strings.NewReader(tt.out))
			if err != nil {
				t.Fatalf("failed: %v", err)
			}

			checkParsed(t, res, tt.want, []string{"Standard", "(default)", "(deprecated)", "machines"})
			if len(res) != tt.count {
				t.Errorf("parsed %v machines, want %v: %v", len(res), tt.count, res)
			}
		})
	}
}

func TestParseNICs(t *testing.T) {
	tests := []struct {
		name  string
		out   string
		want  []string
		count int
	}{
		{
			name:  "legacy",
			out:   deviceOut,
			want:  []string{"e1000", "e1000e", "rtl8139", "virtio-net-pci", "vmxnet3"},
			count: 27,
		},
		{
			name:  "QEMU 11.1.3",
			out:   deviceOutQEMU11,
			want:  []string{"e1000", "e1000-82544gc", "e1000e", "igb", "ne2k_isa", "rtl8139", "tulip", "usb-net", "virtio-net-device", "virtio-net-pci", "virtio-net-pci-transitional", "vmxnet3"},
			count: 30,
		},
		{
			name:  "QEMU 8.2.2",
			out:   testdata(t, "qemu-8.2.2-device-help.txt"),
			want:  []string{"e1000", "e1000-82544gc", "e1000e", "igb", "ne2k_isa", "rtl8139", "tulip", "usb-net", "virtio-net-device", "virtio-net-pci", "virtio-net-pci-transitional", "vmxnet3"},
			count: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := parseNICs(strings.NewReader(tt.out))
			if err != nil {
				t.Fatalf("failed: %v", err)
			}

			checkParsed(t, res, tt.want, []string{"pci-bridge", "usb-ehci", "virtio-scsi-pci", "isa-serial", "ccid-card-emulated", "usb-tablet", "e1000-82540em", "virtio-net"})
			if len(res) != tt.count {
				t.Errorf("parsed %v nics, want %v: %v", len(res), tt.count, res)
			}
		})
	}
}

func TestProbeArgsUseHelp(t *testing.T) {
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{"cpus", cpuArgs(""), []string{"-cpu", "help"}},
		{"cpus with machine", cpuArgs("q35"), []string{"-M", "q35", "-cpu", "help"}},
		{"machines", machineArgs(), []string{"-M", "help"}},
		{"nics", nicArgs(""), []string{"-device", "help"}},
		{"nics with machine", nicArgs("pc"), []string{"-M", "pc", "-device", "help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Equal(tt.got, tt.want) {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
			if slices.Contains(tt.got, "?") {
				t.Errorf("probe uses deprecated \"?\": %q", tt.got)
			}
		})
	}
}

func TestCapsReturnsIndependentMaps(t *testing.T) {
	const name = "test-independent-capability-maps"

	mu.Lock()
	original, existed := cache[name]
	cache[name] = map[string]bool{"e1000": true}
	mu.Unlock()

	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		if existed {
			cache[name] = original
		} else {
			delete(cache, name)
		}
	})

	first, err := caps(name, []string{"unused"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first["lan9118"] = true

	second, err := caps(name, []string{"unused"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if second["lan9118"] {
		t.Fatal("mutating returned capabilities changed the cached map")
	}
}
