// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestBridgeFileWriter(t *testing.T) {
	var mu sync.Mutex
	var writes int
	current := map[string]bool{}

	w := &bridgeFileWriter{
		delay: 50 * time.Millisecond,
		write: func() map[string]bool {
			mu.Lock()
			defer mu.Unlock()
			writes++
			names := map[string]bool{}
			for k := range current {
				names[k] = true
			}
			return names
		},
	}
	setBridges := func(names ...string) {
		mu.Lock()
		defer mu.Unlock()
		current = map[string]bool{}
		for _, n := range names {
			current[n] = true
		}
	}
	got := func() int {
		mu.Lock()
		defer mu.Unlock()
		return writes
	}
	waitWrites := func(n int) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for got() < n {
			if time.Now().After(deadline) {
				t.Fatalf("writes = %d, want %d", got(), n)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	// A new bridge writes the file before update returns.
	setBridges("a")
	w.update("a")
	if n := got(); n != 1 {
		t.Fatalf("writes after a new bridge = %d, want 1", n)
	}

	// Other uses write the file one time after the delay.
	for i := 0; i < 10; i++ {
		w.update("a")
	}
	if n := got(); n != 1 {
		t.Errorf("writes before the delay = %d, want 1", n)
	}
	waitWrites(2)
	time.Sleep(100 * time.Millisecond)
	if n := got(); n != 2 {
		t.Errorf("writes after the delay = %d, want 2", n)
	}

	// A second bridge writes at once, and cancels the pending write.
	w.update("a")
	setBridges("a", "b")
	w.update("b")
	if n := got(); n != 3 {
		t.Errorf("writes after a second bridge = %d, want 3", n)
	}
	time.Sleep(100 * time.Millisecond)
	if n := got(); n != 3 {
		t.Errorf("writes after the canceled delay = %d, want 3", n)
	}

	// A bridge that the last write did not include writes at once.
	setBridges("a")
	w.update("a")
	waitWrites(4)
	setBridges("a", "b")
	w.update("b")
	if n := got(); n != 5 {
		t.Errorf("writes after b came back = %d, want 5", n)
	}

	// reset cancels a pending write and forgets the bridges.
	w.update("a")
	var ran bool
	if err := w.reset(func() error { ran = true; return nil }); err != nil || !ran {
		t.Errorf("reset() = %v, ran %v", err, ran)
	}
	time.Sleep(100 * time.Millisecond)
	if n := got(); n != 5 {
		t.Errorf("writes after reset = %d, want 5", n)
	}
	w.update("a")
	if n := got(); n != 6 {
		t.Errorf("writes after reset and a use = %d, want 6", n)
	}
}
