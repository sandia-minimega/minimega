// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
)

func TestHostTapRecoveryKeepsNamespace(t *testing.T) {
	original := vlans.Default
	defer func() { vlans.Default = original }()

	vlans.Default = vlans.NewVLANs()
	if err := vlans.AddAlias("", "other//lan", 101); err != nil {
		t.Fatal(err)
	}

	info := formatHostTaps([]bridge.Tap{
		{Bridge: "mega_bridge", Name: "mega_tap1", VLAN: 101, Namespace: "owner"},
	})

	lines := strings.Split(strings.TrimSpace(info), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %q, want a header and one tap", info)
	}

	tests := []struct {
		line string
		want hostTapRecord
	}{
		{lines[1], hostTapRecord{"mega_bridge", "mega_tap1", "other//lan", "owner"}},
		{"mega_bridge mega_tap2 other//lan", hostTapRecord{"mega_bridge", "mega_tap2", "other//lan", "other"}},
		{"mega_bridge mega_tap3 lan", hostTapRecord{"mega_bridge", "mega_tap3", "lan", ""}},
	}

	for _, tc := range tests {
		got, err := parseHostTap(tc.line)
		if err != nil {
			t.Errorf("parseHostTap(%q): %v", tc.line, err)
		} else if got != tc.want {
			t.Errorf("parseHostTap(%q) = %+v, want %+v", tc.line, got, tc.want)
		}
	}

	if _, err := parseHostTap("mega_bridge mega_tap1"); err == nil {
		t.Error("parseHostTap accepted a line with two columns")
	}
}

// TestMirrorDeleteAllContinues checks that deleting all the mirrors of a
// namespace tries each mirror after an error.
func TestMirrorDeleteAllContinues(t *testing.T) {
	ns := &Namespace{Mirrors: map[string]bool{"mm_test_gone1": true, "mm_test_gone2": true}}

	err := mirrorDelete(ns, Wildcard)
	if err == nil || !strings.Contains(err.Error(), "mm_test_gone1") || !strings.Contains(err.Error(), "mm_test_gone2") {
		t.Errorf("mirrorDelete(all) = %v, want an error for each mirror", err)
	}
}

// TestCleanKeep checks that the clean-up at start keeps the tap mirrors only
// with -recover, which takes them back.
func TestCleanKeep(t *testing.T) {
	if got := cleanKeep(false); len(got) != 0 {
		t.Errorf("cleanKeep(false) = %v, want nothing", got)
	}
	if got := cleanKeep(true); !slices.Equal(got, []string{bridge.MirrorKindTap}) {
		t.Errorf("cleanKeep(true) = %v, want %v", got, bridge.MirrorKindTap)
	}
}

// TestAdoptMirrors checks that a recovered tap mirror is a mirror of its
// namespace, so that mirror delete removes it.
func TestAdoptMirrors(t *testing.T) {
	nss := map[string]*Namespace{"exp": {Name: "exp", Mirrors: map[string]bool{}}}
	get := func(name string) *Namespace {
		if nss[name] == nil {
			nss[name] = &Namespace{Name: name, Mirrors: map[string]bool{}}
		}
		return nss[name]
	}

	adoptMirrors([]bridge.MirrorInfo{
		{Bridge: "mega_bridge", Mirror: "mirror-mega_tap2", Output: "mega_tap2", Namespace: "exp", Kind: bridge.MirrorKindTap},
		{Bridge: "mega_bridge", Mirror: "mirror-mega_tap3", Output: "mega_tap3", Namespace: "other", Kind: bridge.MirrorKindTap},
	}, get)

	if !nss["exp"].Mirrors["mega_tap2"] || !nss["other"].Mirrors["mega_tap3"] || len(nss["exp"].Mirrors) != 1 {
		t.Errorf("namespace mirrors: exp %v, other %v", nss["exp"].Mirrors, nss["other"].Mirrors)
	}
}
