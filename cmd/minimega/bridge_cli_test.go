package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/gopacket/pcap"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// `tap create ... ip` must report the ip failure to the user even though the
// tap clean-up succeeds.
func TestTapCreateIPFailureIsReported(t *testing.T) {
	// creating the bridge starts a packet capture on it, which needs
	// CAP_NET_RAW (unprivileged CI runners do not have it)
	h, err := pcap.OpenLive("lo", 1600, true, time.Second)
	if err != nil {
		t.Skipf("cannot capture on lo: %v", err)
	}
	h.Close()

	originalVLANs, originalBase := vlans.Default, *f_base
	defer func() { vlans.Default, *f_base = originalVLANs, originalBase }()
	vlans.Default = vlans.NewVLANs()
	*f_base = t.TempDir()

	// fake ovs-vsctl (always works) and ip (fails only for `addr add`)
	bin := t.TempDir()
	for name, body := range map[string]string{
		"ovs-vsctl": "exit 0",
		"ovs-ofctl": "exit 0",
		"ip":        `[ "$1" = addr ] && { echo bad address; exit 1; }; exit 0`,
	} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))

	ns := &Namespace{Name: "ns", Taps: map[string]bool{}}
	c := &minicli.Command{
		BoolArgs: map[string]bool{"create": true},
		StringArgs: map[string]string{
			"bridge": "lo", "tap": "mmtap0", "vlan": "100", "ip": "999.1.1.1/24",
		},
	}
	resp := &minicli.Response{}

	err = cliTapCreate(ns, c, resp)
	if err == nil || !strings.Contains(err.Error(), "ip error") {
		t.Fatalf("expected ip error, got %v", err)
	}
	if ns.Taps["mmtap0"] {
		t.Error("failed tap recorded in namespace")
	}
}
