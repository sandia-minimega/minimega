// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeTool logs its invocation and fails when it matches $FAKE_TOOLS_FAIL.
const fakeTool = `#!/bin/sh
cmd="${0##*/} $*"
echo "$cmd" >> "$FAKE_TOOLS_LOG"
if [ -n "$FAKE_TOOLS_FAIL" ]; then
	case "$cmd" in *"$FAKE_TOOLS_FAIL"*) echo "fake failure: $cmd"; exit 1 ;; esac
fi
`

// fakeTools replaces ip and Open vSwitch with stand-ins and returns their call log.
func fakeTools(t *testing.T) func() string {
	t.Helper()

	dir := t.TempDir()
	for _, name := range []string{"ip", "ovs-vsctl", "ovs-ofctl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fakeTool), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	log := filepath.Join(dir, "calls")
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_TOOLS_LOG", log)
	t.Setenv("FAKE_TOOLS_FAIL", "")

	return func() string {
		t.Helper()

		data, err := os.ReadFile(log)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		return string(data)
	}
}

// newTestBridges returns Bridges holding one bridge that was never set up in Open vSwitch.
func newTestBridges() (Bridges, *Bridge) {
	br := &Bridge{
		Name:    "mmtest_bridge",
		taps:    make(map[string]*Tap),
		mirrors: make(map[string]bool),
		bonds:   make(map[string]map[string]int),
		tapChan: make(chan string, 1),
	}

	return Bridges{Default: br.Name, bridges: map[string]*Bridge{br.Name: br}}, br
}

func TestCreateHostTap(t *testing.T) {
	calls := fakeTools(t)
	bridges, br := newTestBridges()

	tap, err := br.CreateHostTap("mmtest_tap", 5)
	if err != nil {
		t.Fatal(err)
	}
	if tap != "mmtest_tap" {
		t.Errorf("got tap %q, want mmtest_tap", tap)
	}

	for _, want := range []string{
		"ovs-vsctl add-port mmtest_bridge mmtest_tap tag=5 -- set Interface mmtest_tap type=internal",
		"ip link set mmtest_tap up promisc on",
	} {
		if !strings.Contains(calls(), want+"\n") {
			t.Errorf("missing call %q in:\n%s", want, calls())
		}
	}

	got, err := bridges.FindTap(tap)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Host || got.VLAN != 5 || got.Bridge != br.Name {
		t.Errorf("unexpected tap: %+v", got)
	}
	if taps := bridges.HostTaps(); len(taps) != 1 || taps[0].Name != tap {
		t.Errorf("got host taps %+v, want only %v", taps, tap)
	}
}

func TestCreateHostTapGeneratesName(t *testing.T) {
	fakeTools(t)
	_, br := newTestBridges()
	br.tapChan <- "mmtest_tap7"

	tap, err := br.CreateHostTap("", 5)
	if err != nil {
		t.Fatal(err)
	}
	if tap != "mmtest_tap7" {
		t.Errorf("got tap %q, want mmtest_tap7", tap)
	}
}

func TestCreateHostTapDuplicate(t *testing.T) {
	fakeTools(t)
	bridges, br := newTestBridges()

	if _, err := br.CreateHostTap("mmtest_tap", 5); err != nil {
		t.Fatal(err)
	}
	if _, err := br.CreateHostTap("mmtest_tap", 6); err == nil {
		t.Fatal("creating a tap twice succeeded")
	}

	// the first tap is untouched
	if got, err := bridges.FindTap("mmtest_tap"); err != nil || got.VLAN != 5 {
		t.Errorf("got tap %+v, %v; want VLAN 5", got, err)
	}
}

func TestCreateHostTapAddPortFailure(t *testing.T) {
	fakeTools(t)
	bridges, br := newTestBridges()
	t.Setenv("FAKE_TOOLS_FAIL", "add-port")

	if _, err := br.CreateHostTap("mmtest_tap", 5); err == nil {
		t.Fatal("expected an error when ovs-vsctl fails")
	}
	if taps := bridges.HostTaps(); len(taps) != 0 {
		t.Errorf("got host taps %+v, want none", taps)
	}
}

// A tap that cannot be brought up is reported and removed from the bridge.
func TestCreateHostTapUpFailure(t *testing.T) {
	calls := fakeTools(t)
	bridges, br := newTestBridges()
	t.Setenv("FAKE_TOOLS_FAIL", "ip link set")

	_, err := br.CreateHostTap("mmtest_tap", 5)
	if err == nil {
		t.Fatal("expected an error when the tap cannot be brought up")
	}
	if !strings.Contains(err.Error(), "fake failure") {
		t.Errorf("error does not carry the command output: %v", err)
	}

	if taps := bridges.HostTaps(); len(taps) != 0 {
		t.Errorf("got host taps %+v, want none", taps)
	}
	if _, err := bridges.FindTap("mmtest_tap"); err == nil {
		t.Error("failed tap is still findable")
	}

	if err := bridges.ReapTaps(); err != nil {
		t.Fatal(err)
	}
	if want := "ovs-vsctl -- del-port mmtest_bridge mmtest_tap\n"; !strings.Contains(calls(), want) {
		t.Errorf("missing call %q in:\n%s", want, calls())
	}
}

func TestDestroyTapIsReaped(t *testing.T) {
	calls := fakeTools(t)
	bridges, br := newTestBridges()

	for _, tap := range []string{"mmtest_tap0", "mmtest_tap1"} {
		if _, err := br.CreateHostTap(tap, 5); err != nil {
			t.Fatal(err)
		}
	}

	if err := br.DestroyTap("mmtest_tap0"); err != nil {
		t.Fatal(err)
	}
	if err := br.DestroyTap("mmtest_missing"); err == nil {
		t.Error("destroying an unknown tap succeeded")
	}

	// destroyed taps disappear right away, before the reaper runs
	if taps := bridges.HostTaps(); len(taps) != 1 || taps[0].Name != "mmtest_tap1" {
		t.Errorf("got host taps %+v, want only mmtest_tap1", taps)
	}
	if strings.Contains(calls(), "del-port") {
		t.Errorf("port deleted before the reaper ran:\n%s", calls())
	}

	if err := br.ReapTaps(); err != nil {
		t.Fatal(err)
	}
	if want := "ovs-vsctl -- del-port mmtest_bridge mmtest_tap0\n"; !strings.Contains(calls(), want) {
		t.Errorf("missing call %q in:\n%s", want, calls())
	}
	if strings.Contains(calls(), "del-port mmtest_bridge mmtest_tap1") {
		t.Errorf("live tap was reaped:\n%s", calls())
	}

	// the name is free again once reaped
	if _, err := br.CreateHostTap("mmtest_tap0", 6); err != nil {
		t.Errorf("recreating a reaped tap: %v", err)
	}
}

// A failed reap keeps the taps so that the next pass retries them.
func TestReapTapsFailureRetries(t *testing.T) {
	fakeTools(t)
	_, br := newTestBridges()

	if _, err := br.CreateHostTap("mmtest_tap", 5); err != nil {
		t.Fatal(err)
	}
	if err := br.DestroyTap("mmtest_tap"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("FAKE_TOOLS_FAIL", "del-port")
	if err := br.ReapTaps(); err == nil {
		t.Fatal("expected an error when ovs-vsctl fails")
	}

	t.Setenv("FAKE_TOOLS_FAIL", "")
	if err := br.ReapTaps(); err != nil {
		t.Fatal(err)
	}
	if len(br.taps) != 0 {
		t.Errorf("got taps %v after reaping, want none", br.taps)
	}
}
