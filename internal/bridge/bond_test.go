package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeOVS puts a fake ovs-vsctl on PATH that records its arguments, and
// returns a func reading the recorded commands.
func fakeOVS(t *testing.T) func() []string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "ovs-vsctl"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return func() []string {
		b, _ := os.ReadFile(log)
		return strings.Split(strings.TrimSpace(string(b)), "\n")
	}
}

func TestAddBondInvalidKeepsTaps(t *testing.T) {
	for _, tc := range []struct{ mode, lacp string }{
		{"balance-tcp", "off"},
		{"Balance-TCP", "active"},
		{"bogus", "off"},
	} {
		cmds := fakeOVS(t)
		b := &Bridge{Name: "br0", bonds: map[string]map[string]int{}, taps: map[string]*Tap{}}

		_, err := b.AddBond("bond0", tc.mode, tc.lacp, false, map[string]int{"tap0": 1, "tap1": 1}, 1)
		if err == nil {
			t.Fatalf("%v/%v: expected error", tc.mode, tc.lacp)
		}

		// the VM's taps must still be on the bridge: nothing removed them
		for _, c := range cmds() {
			if strings.Contains(c, "del-port") {
				t.Errorf("%v/%v: taps removed from OVS despite error: %q", tc.mode, tc.lacp, c)
			}
		}
	}
}
