// Copyright 2015-2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"net"
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

func TestValidMACPrefix(t *testing.T) {
	for _, p := range validMACPrefix {
		if p[0]&macMulticastBit != 0 {
			t.Errorf("validMACPrefix has multicast OUI %02x:%02x:%02x", p[0], p[1], p[2])
		}

		if p[0]&macLocalBit != 0 {
			t.Errorf("validMACPrefix has locally administered OUI %02x:%02x:%02x", p[0], p[1], p[2])
		}

		if p == ianaMACPrefix {
			t.Errorf("validMACPrefix has IANA OUI %02x:%02x:%02x", p[0], p[1], p[2])
		}
	}
}

func TestIsUsableRandomMAC(t *testing.T) {
	examples := map[string]bool{
		// ordinary vendor addresses, including ones just outside each FHRP range
		"00:00:0c:12:34:56": true,
		"00:00:0c:07:ab:ff": true, // below HSRPv1
		"00:00:0c:9f:ef:ff": true, // below HSRPv2 IPv4
		"00:05:73:a0:10:00": true, // above HSRP IPv6
		"00:07:b4:01:00:00": true, // above GLBP
		"00:00:00:00:00:01": true, // all-zero prefix, non-zero address

		// all-zero address
		"00:00:00:00:00:00": false,

		// I/G (multicast) bit
		"11:00:aa:12:34:56": false,
		"ff:ff:ff:ff:ff:ff": false, // broadcast

		// U/L (locally administered) bit
		"02:00:00:00:00:01": false,
		"aa:00:04:00:00:01": false, // DECnet

		// IANA OUI
		"00:00:5e:00:01:01": false, // VRRP/CARP
		"00:00:5e:12:34:56": false,

		// first and last address of each FHRP range
		"00:00:0c:07:ac:00": false, // HSRPv1
		"00:00:0c:07:ac:ff": false,
		"00:00:0c:9f:f0:00": false, // HSRPv2 IPv4
		"00:00:0c:9f:ff:ff": false,
		"00:05:73:a0:00:00": false, // HSRP IPv6
		"00:05:73:a0:0f:ff": false,
		"00:07:b4:00:00:00": false, // GLBP
		"00:07:b4:00:ff:ff": false,
	}

	for mac, want := range examples {
		hw, err := net.ParseMAC(mac)
		if err != nil {
			t.Fatalf("bad example %q: %v", mac, err)
		}

		if got := isUsableRandomMAC([6]byte(hw)); got != want {
			t.Errorf("isUsableRandomMAC(%v) = %v, want %v", mac, got, want)
		}
	}
}

func TestRandomMac(t *testing.T) {
	for range 1000 {
		mac, err := randomMac()
		if err != nil {
			t.Fatalf("randomMac() error: %v", err)
		}

		hw, err := net.ParseMAC(mac)
		if err != nil || len(hw) != 6 || !isUsableRandomMAC([6]byte(hw)) {
			t.Fatalf("randomMac() = %q (err %v), want a usable address", mac, err)
		}
	}
}

func TestRandomMacNoPrefixes(t *testing.T) {
	saved := validMACPrefix
	validMACPrefix = nil
	defer func() { validMACPrefix = saved }()

	if mac, err := randomMac(); err == nil {
		t.Fatalf("randomMac() = %q, want an error with no prefixes", mac)
	}
}
