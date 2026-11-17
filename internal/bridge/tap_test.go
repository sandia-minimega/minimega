package bridge

import "testing"

// Destroying a tap that is a mirror destination must also destroy the tap.
func TestDestroyTapMirrorDestination(t *testing.T) {
	fakeOVS(t)
	b := &Bridge{
		Name:    "br0",
		mirrors: map[string]bool{"tap1": true},
		taps:    map[string]*Tap{"tap1": {Name: "tap1", Host: true}},
	}

	if err := b.DestroyTap("tap1"); err != nil {
		t.Fatal(err)
	}
	if b.mirrors["tap1"] {
		t.Error("mirror not removed")
	}
	if !b.taps["tap1"].Defunct {
		t.Error("tap was not destroyed (not marked defunct)")
	}
}
