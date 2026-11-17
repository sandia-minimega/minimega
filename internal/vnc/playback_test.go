// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import (
	"path/filepath"
	"testing"
	"time"
)

// Pausing and continuing partway through a delay must leave only the
// remaining delay, not add the elapsed time on top of it.
func TestPauseKeepsRemainingDelay(t *testing.T) {
	srv := newFakeServer(t)
	dir := t.TempDir()
	writeKbr(t, filepath.Join(dir, "a.kbr"), "1s:KeyEvent,true,a\n")

	p := NewPlayer()
	start := time.Now()
	if err := p.Playback("vm", srv.addr, filepath.Join(dir, "a.kbr")); err != nil {
		t.Fatal(err)
	}
	defer p.Stop("vm")

	time.Sleep(600 * time.Millisecond)
	within(t, 5*time.Second, "Pause", func() error { return p.Pause("vm") })
	within(t, 5*time.Second, "Continue", func() error { return p.Continue("vm") })

	if !srv.gotKey('a') {
		t.Fatal("event never fired")
	}
	// ~1s expected; the bug re-waited the elapsed 0.6s too (~1.6s)
	if got := time.Since(start); got > 1400*time.Millisecond {
		t.Fatalf("event fired after %v, want about 1s", got)
	}
}
