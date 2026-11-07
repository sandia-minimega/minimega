// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package minicli

import "testing"

// TestInputItemsString checks that a lexed input written back as a string
// lexes to the same items (issue #1685).
func TestInputItemsString(t *testing.T) {
	inputs := []string{
		`capture pcap filter ""`,
		`capture pcap filter "tcp port 80"`,
		`foo "a \"quoted\" word" 'it\'s' "back\\slash" "#hash" ''`,
		`capture pcap vm foo 0 foo.pcap filter="tcp port 80" snaplen=128`,
		"foo zero\u200bwidth \"no\u00a0break\" \"bell\a\vtab\"",
	}

	for _, v := range inputs {
		in, err := lexInput(v)
		if err != nil {
			t.Fatalf("lexInput(%q): %v", v, err)
		}

		again, err := lexInput(in.items.String())
		if err != nil {
			t.Fatalf("lexInput(%q): %v", in.items.String(), err)
		}

		if len(again.items) != len(in.items) {
			t.Fatalf("%q: got %d items from %q, want %d", v, len(again.items), in.items.String(), len(in.items))
		}

		for i := range in.items {
			if again.items[i] != in.items[i] {
				t.Errorf("%q: item %d is %q, want %q", v, i, again.items[i].Value, in.items[i].Value)
			}
		}
	}
}
