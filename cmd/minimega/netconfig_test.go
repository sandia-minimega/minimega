// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"testing"
)

func TestParseNetConfig(t *testing.T) {
	nics := map[string]bool{
		"e1000":          true,
		"virtio-net-pci": true,
	}

	examples := []string{
		"foo",
		"foo,virtio-net-pci",
		"foo,de:ad:be:ef:ca:fe",
		"foo,de:ad:be:ef:ca:fe,virtio-net-pci",
		"foo,qinq",
		"foo,virtio-net-pci,qinq",
		"foo,de:ad:be:ef:ca:fe,qinq",
		"foo,de:ad:be:ef:ca:fe,virtio-net-pci,qinq",

		"my_bridge,foo",
		"my_bridge,foo,virtio-net-pci",
		"my_bridge,foo,de:ad:be:ef:ca:fe",
		"my_bridge,foo,de:ad:be:ef:ca:fe,virtio-net-pci",
		"my_bridge,foo,qinq",
		"my_bridge,foo,virtio-net-pci,qinq",
		"my_bridge,foo,de:ad:be:ef:ca:fe,qinq",
		"my_bridge,foo,de:ad:be:ef:ca:fe,virtio-net-pci,qinq",
	}

	for _, s := range examples {
		r, err := ParseNetConfig(s, nics)
		if err != nil {
			t.Fatalf("unable to parse `%v`: %v", s, err)
		}

		got := r.String()
		if got != s {
			t.Fatalf("unequal: `%v` != `%v`", s, got)
		}
	}
}

// TestParseNetConfigDefaults ensures that ParseNetConfig fills in the
// default bridge and driver when they are not specified in the netspec,
// since "vm net add" and "vm config net" both rely on these defaults.
func TestParseNetConfigDefaults(t *testing.T) {
	nics := map[string]bool{
		"e1000":          true,
		"virtio-net-pci": true,
	}

	r, err := ParseNetConfig("foo", nics)
	if err != nil {
		t.Fatalf("unable to parse `foo`: %v", err)
	}

	if r.Bridge != DefaultBridge {
		t.Fatalf("expected default bridge %q, got %q", DefaultBridge, r.Bridge)
	}
	if r.Driver != DefaultKVMDriver {
		t.Fatalf("expected default driver %q, got %q", DefaultKVMDriver, r.Driver)
	}
	if r.MAC != "" {
		t.Fatalf("expected no mac, got %q", r.MAC)
	}
	if r.QinQ {
		t.Fatalf("expected qinq to be false")
	}
}

// TestParseNetConfigErrors covers netspecs that should be rejected as
// malformed, e.g. too many fields or fields that can't be unambiguously
// disambiguated into bridge/vlan/mac/driver/qinq.
func TestParseNetConfigErrors(t *testing.T) {
	nics := map[string]bool{
		"e1000":          true,
		"virtio-net-pci": true,
	}

	examples := []string{
		// 3 fields: neither qinq, mac, nor driver in the disambiguating
		// positions
		"my_bridge,foo,bar",
		// 4 fields: last two fields aren't a recognized (mac,driver) or
		// (driver,qinq) or (mac,qinq) combination
		"my_bridge,foo,bar,baz",
		"foo,bar,baz,qux",
		// 5 fields: doesn't match bridge,vlan,mac,driver,qinq
		"my_bridge,foo,bar,virtio-net-pci,qinq",
		"my_bridge,foo,de:ad:be:ef:ca:fe,bar,qinq",
		"my_bridge,foo,de:ad:be:ef:ca:fe,virtio-net-pci,bar",
		// 6+ fields are always malformed
		"a,b,c,d,e,f",
	}

	for _, s := range examples {
		if _, err := ParseNetConfig(s, nics); err == nil {
			t.Fatalf("expected error parsing malformed netspec `%v`", s)
		}
	}
}

func TestParseBondConfig(t *testing.T) {
	examples := []string{
		"0,1,active-backup",
		"0,1,active-backup,foo-bond",
		"0,1,active-backup,qinq",
		"1,3,balance-tcp,qinq,foo-bond",
		"1,3,balance-tcp,active,no-lacp-fallback",
		"1,3,balance-tcp,active,no-lacp-fallback,qinq",
		"1,3,balance-tcp,active,no-lacp-fallback,foo-bond",
		"1,3,balance-tcp,active,no-lacp-fallback,qinq,foo-bond",
	}

	for _, s := range examples {
		r, err := ParseBondConfig(s)
		if err != nil {
			t.Fatalf("unable to parse `%v`: %v", s, err)
		}

		got := r.String()
		if got != s {
			t.Fatalf("unequal: `%v` != `%v`", s, got)
		}
	}
}
