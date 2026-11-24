// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// fakeHostTool logs its invocation and fails when it matches $FAKE_TOOLS_FAIL.
const fakeHostTool = `#!/bin/sh
cmd="${0##*/} $*"
echo "$cmd" >> "$FAKE_TOOLS_LOG"
if [ -n "$FAKE_TOOLS_FAIL" ]; then
	case "$cmd" in *"$FAKE_TOOLS_FAIL"*) echo "fake failure: $cmd"; exit 1 ;; esac
fi
`

// fakeHostTaps replaces ip, dhclient, and Open vSwitch with stand-ins, points
// minimega at unprivileged bridges, and returns the stand-ins' call log.
func fakeHostTaps(t *testing.T) func() string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"ip", "dhclient", "ovs-vsctl", "ovs-ofctl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fakeHostTool), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_TOOLS_LOG", log)
	t.Setenv("FAKE_TOOLS_FAIL", "")

	var (
		originalBridges = bridges
		originalVLANs   = vlans.Default
		originalBase    = *f_base
	)

	t.Cleanup(func() {
		bridges = originalBridges
		vlans.Default = originalVLANs
		*f_base = originalBase
	})

	bridges = bridge.NewBridges(DefaultBridge, "mmtest_tap%v", "mmtest_bond%v")
	bridges.NoSnoop = true
	vlans.Default = vlans.NewVLANs()
	*f_base = t.TempDir()

	return func() string {
		t.Helper()

		data, err := os.ReadFile(log)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return string(data)
	}
}

func testTapNamespace() *Namespace {
	return &Namespace{
		Name:    "test",
		Taps:    make(map[string]bool),
		Mirrors: make(map[string]bool),
	}
}

// tapCreate runs "tap create" on VLAN 10 with the given extra arguments.
func tapCreate(ns *Namespace, stringArgs map[string]string, boolArgs map[string]bool) (string, error) {
	c := &minicli.Command{
		StringArgs: map[string]string{"vlan": "10"},
		BoolArgs:   map[string]bool{"create": true},
	}
	for k, v := range stringArgs {
		c.StringArgs[k] = v
	}
	for k, v := range boolArgs {
		c.BoolArgs[k] = v
	}

	resp := &minicli.Response{}
	err := cliTap(ns, c, resp)

	return resp.Response, err
}

// tapsFile returns the taps listed in the state file used for recovery.
func tapsFile(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(*f_base, "taps"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return string(data)
}

func TestTapCreate(t *testing.T) {
	tests := []struct {
		name       string
		stringArgs map[string]string
		boolArgs   map[string]bool
		wantTap    string
		wantCall   string
	}{
		{name: "plain", wantTap: "mmtest_tap0"},
		{
			name:       "named",
			stringArgs: map[string]string{"tap": "mmtest_named"},
			wantTap:    "mmtest_named",
		},
		{
			name:       "ip",
			stringArgs: map[string]string{"ip": "10.0.0.1/24"},
			wantTap:    "mmtest_tap0",
			wantCall:   "ip addr add dev mmtest_tap0 10.0.0.1/24",
		},
		{
			name:     "dhcp",
			boolArgs: map[string]bool{"dhcp": true},
			wantTap:  "mmtest_tap0",
			wantCall: "dhclient mmtest_tap0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeHostTaps(t)
			ns := testTapNamespace()

			tap, err := tapCreate(ns, tt.stringArgs, tt.boolArgs)
			if err != nil {
				t.Fatal(err)
			}
			if tap != tt.wantTap {
				t.Errorf("got tap %q, want %q", tap, tt.wantTap)
			}
			if !ns.Taps[tap] {
				t.Errorf("tap %v not recorded in the namespace", tap)
			}
			if !strings.Contains(calls(), tt.wantCall+"\n") {
				t.Errorf("missing call %q in:\n%s", tt.wantCall, calls())
			}
			if !strings.Contains(tapsFile(t), tap) {
				t.Errorf("tap %v missing from the taps file:\n%s", tap, tapsFile(t))
			}

			resp := &minicli.Response{}
			if err := cliTap(ns, &minicli.Command{}, resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.Tabular) != 1 || resp.Tabular[0][0] != DefaultBridge || resp.Tabular[0][1] != tap {
				t.Errorf("got tap list %v, want only %v on %v", resp.Tabular, tap, DefaultBridge)
			}
		})
	}
}

// A tap whose address cannot be configured used to be deleted while the
// command reported success (sandia-minimega/minimega#1270).
func TestTapCreateConfigFailure(t *testing.T) {
	tests := []struct {
		name       string
		stringArgs map[string]string
		boolArgs   map[string]bool
		fail       string
		wantErr    string
	}{
		{
			name:       "ip",
			stringArgs: map[string]string{"ip": "10.0.0.1/24"},
			fail:       "ip addr add",
			wantErr:    "ip error",
		},
		{
			name:     "dhcp",
			boolArgs: map[string]bool{"dhcp": true},
			fail:     "dhclient",
			wantErr:  "dhcp error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := fakeHostTaps(t)
			ns := testTapNamespace()
			t.Setenv("FAKE_TOOLS_FAIL", tt.fail)

			tap, err := tapCreate(ns, tt.stringArgs, tt.boolArgs)
			if err == nil {
				t.Fatalf("tap create reported success with tap %q", tap)
			}
			for _, want := range []string{tt.wantErr, "fake failure"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}

			if len(ns.Taps) != 0 {
				t.Errorf("failed tap recorded in the namespace: %v", ns.Taps)
			}
			if taps := bridges.HostTaps(); len(taps) != 0 {
				t.Errorf("got host taps %+v, want none", taps)
			}
			if got := tapsFile(t); got != "" {
				t.Errorf("failed tap left in the taps file:\n%s", got)
			}

			t.Setenv("FAKE_TOOLS_FAIL", "")
			if err := bridges.ReapTaps(); err != nil {
				t.Fatal(err)
			}
			if want := "del-port " + DefaultBridge + " mmtest_tap0\n"; !strings.Contains(calls(), want) {
				t.Errorf("missing call %q in:\n%s", want, calls())
			}
		})
	}
}

func TestTapCreateRejectsReservedNames(t *testing.T) {
	fakeHostTaps(t)
	ns := testTapNamespace()

	if _, err := tapCreate(ns, map[string]string{"tap": Wildcard}, nil); err == nil {
		t.Error("reserved tap name accepted")
	}
	if _, err := tapCreate(ns, map[string]string{"bridge": Wildcard}, nil); err == nil {
		t.Error("reserved bridge name accepted")
	}
	if taps := bridges.HostTaps(); len(taps) != 0 {
		t.Errorf("got host taps %+v, want none", taps)
	}
}

func TestTapDelete(t *testing.T) {
	fakeHostTaps(t)
	ns, other := testTapNamespace(), testTapNamespace()

	var taps []string
	for i := 0; i < 3; i++ {
		tap, err := tapCreate(ns, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		taps = append(taps, tap)
	}

	if err := hostTapDelete(other, taps[0]); err == nil {
		t.Error("deleted a tap that belongs to another namespace")
	}
	if err := hostTapDelete(ns, "mmtest_missing"); err == nil {
		t.Error("deleted a tap that does not exist")
	}

	c := &minicli.Command{
		StringArgs: map[string]string{"tap": taps[0]},
		BoolArgs:   map[string]bool{"delete": true},
	}
	if err := cliTap(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}
	if ns.Taps[taps[0]] || len(ns.Taps) != 2 {
		t.Errorf("got namespace taps %v after deleting %v", ns.Taps, taps[0])
	}
	if got := tapsFile(t); strings.Contains(got, taps[0]) || !strings.Contains(got, taps[1]) {
		t.Errorf("unexpected taps file after deleting %v:\n%s", taps[0], got)
	}

	// deleting all taps also has to empty the file used for recovery
	if err := hostTapDelete(ns, Wildcard); err != nil {
		t.Fatal(err)
	}
	if len(ns.Taps) != 0 {
		t.Errorf("got namespace taps %v after deleting all", ns.Taps)
	}
	if left := bridges.HostTaps(); len(left) != 0 {
		t.Errorf("got host taps %+v after deleting all", left)
	}
	if got := tapsFile(t); got != "" {
		t.Errorf("taps file not emptied after deleting all:\n%s", got)
	}
}

// Destroying a bridge takes its taps and mirrors with it, which must not stop
// a namespace from cleaning up afterwards.
func TestTapDeleteAfterBridgeDestroy(t *testing.T) {
	fakeHostTaps(t)
	ns := testTapNamespace()

	tap, err := tapCreate(ns, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	kept, err := tapCreate(ns, map[string]string{"bridge": "mmtest_bridge2"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ns.Mirrors[tap] = true

	if err := bridges.DestroyBridge(DefaultBridge); err != nil {
		t.Fatal(err)
	}

	if err := mirrorDelete(ns, Wildcard); err != nil {
		t.Errorf("clearing mirrors: %v", err)
	}
	if len(ns.Mirrors) != 0 {
		t.Errorf("got namespace mirrors %v, want none", ns.Mirrors)
	}

	if err := hostTapDelete(ns, Wildcard); err != nil {
		t.Errorf("deleting taps: %v", err)
	}
	if len(ns.Taps) != 0 {
		t.Errorf("got namespace taps %v, want none", ns.Taps)
	}
	if _, err := bridges.FindTap(kept); err == nil {
		t.Errorf("tap %v on the surviving bridge was not deleted", kept)
	}
}
