// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
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
