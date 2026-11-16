// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package vnc

import "testing"

// keysymInverse gives one fixed name for a keysym that has several names.
func TestKeysymInverse(t *testing.T) {
	for k, v := range map[uint32]string{
		0x27: "apostrophe", 0x60: "grave", 0x41: "A", 0x61: "a",
		0xff55: "Page_Up", 0xff56: "Page_Down", 0xff9b: "KP_Page_Down",
		0xff7e: "Mode_switch", 0xffc8: "F11",
	} {
		if got := keysymInverse[k]; got != v {
			t.Errorf("keysym %#x is %q, want %q", k, got, v)
		}
	}
}
