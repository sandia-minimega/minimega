// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import "testing"

func TestIsValidName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		// acceptable names
		{"foo", true},
		{"ns1", true},
		{"foo-bar_baz.1", true},
		{".hidden", true},
		{"a.", true},
		{"...", true},
		// empty
		{"", false},
		// filesystem-special names that would escape or alias a directory
		{".", false},
		{"..", false},
		// disallowed characters
		{"foo bar", false},
		{"foo/bar", false},
		{"../foo", false},
		{"ns/../../etc", false},
		{"foo\x00", false},
	}

	for _, tt := range tests {
		if got := isValidName(tt.name); got != tt.want {
			t.Errorf("isValidName(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
