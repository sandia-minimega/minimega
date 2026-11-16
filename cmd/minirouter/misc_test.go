// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeSysClassNet builds a /sys/class/net with one directory per interface,
// writing its type file unless the type is "".
func fakeSysClassNet(t *testing.T, ifaces map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, typ := range ifaces {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if typ == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name, "type"), []byte(typ+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestListEths(t *testing.T) {
	tests := []struct {
		name   string
		ifaces map[string]string
		want   []string
	}{
		{
			// issue #1387: a container launched after ip_gre was loaded
			name: "container with GRE fallback devices",
			ifaces: map[string]string{
				"lo": "772", "veth0": "1", "veth1": "1",
				"gre0": "778", "gretap0": "1", "erspan0": "1",
			},
			want: []string{"veth0", "veth1"},
		},
		{
			name: "every kernel fallback tunnel",
			ifaces: map[string]string{
				"lo": "772", "eth0": "1",
				"gre0": "778", "gretap0": "1", "erspan0": "1", "tunl0": "768", "sit0": "776",
				"ip6tnl0": "769", "ip6gre0": "823", "ip_vti0": "768", "ip6_vti0": "769",
			},
			want: []string{"eth0"},
		},
		{
			name: "non-Ethernet device with an unknown name",
			ifaces: map[string]string{
				"eth0": "1", "wg0": "65534", "eth1": "1",
			},
			want: []string{"eth0", "eth1"},
		},
		{
			name: "numeric order, not lexical",
			ifaces: map[string]string{
				"eth0": "1", "eth1": "1", "eth2": "1", "eth10": "1", "eth11": "1",
			},
			want: []string{"eth0", "eth1", "eth2", "eth10", "eth11"},
		},
		{
			name: "predictable names",
			ifaces: map[string]string{
				"ens10": "1", "ens3": "1", "ens4": "1",
			},
			want: []string{"ens3", "ens4", "ens10"},
		},
		{
			name: "unreadable type is kept",
			ifaces: map[string]string{
				"eth0": "", "eth1": "1",
			},
			want: []string{"eth0", "eth1"},
		},
		{
			name:   "only loopback",
			ifaces: map[string]string{"lo": "772"},
			want:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := listEths(fakeSysClassNet(t, tt.ifaces))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("listEths() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestListEthsMissingDir(t *testing.T) {
	if _, err := listEths(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("listEths() of a missing directory returned no error")
	}
}

func TestFindEth(t *testing.T) {
	old := sysClassNet
	t.Cleanup(func() { sysClassNet = old })
	sysClassNet = fakeSysClassNet(t, map[string]string{
		"lo": "772", "erspan0": "1", "eth0": "1", "eth1": "1", "gre0": "778", "gretap0": "1",
	})

	for idx, want := range []string{"eth0", "eth1"} {
		got, err := findEth(idx)
		if err != nil {
			t.Fatalf("findEth(%d): %v", idx, err)
		}
		if got != want {
			t.Errorf("findEth(%d) = %q, want %q", idx, got, want)
		}
	}

	for _, idx := range []int{2, -1} {
		if got, err := findEth(idx); err == nil {
			t.Errorf("findEth(%d) = %q, want an error", idx, got)
		}
	}
}

func TestEthLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"eth0", "eth1", true},
		{"eth1", "eth0", false},
		{"eth2", "eth10", true},
		{"veth0", "eth1", true},
		// equal suffixes fall back to the name, so the order is total
		{"eth0", "veth0", true},
		{"veth0", "eth0", false},
		// names without a suffix sort last
		{"eth9", "bond", true},
		{"bond", "eth9", false},
		{"br", "bond", false},
		{"eth0", "eth0", false},
	}
	for _, tt := range tests {
		if got := ethLess(tt.a, tt.b); got != tt.want {
			t.Errorf("ethLess(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestEthIndex(t *testing.T) {
	tests := []struct {
		name string
		idx  int
		ok   bool
	}{
		{"eth0", 0, true},
		{"veth12", 12, true},
		{"ip6tnl0", 0, true},
		{"enp0s3", 3, true},
		{"bond", 0, false},
		{"", 0, false},
	}
	for _, tt := range tests {
		idx, ok := ethIndex(tt.name)
		if idx != tt.idx || ok != tt.ok {
			t.Errorf("ethIndex(%q) = %d, %v, want %d, %v", tt.name, idx, ok, tt.idx, tt.ok)
		}
	}
}
