// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"testing"
)

func TestParseVmTags(t *testing.T) {
	want := map[string]string{
		"foo":       "bar", // easy case
		"foo bar":   "baz", // key with spaces
		`"foo"`:     "bar", // key with quotes
		`"foo bar"`: "baz", // key with quotes and spaces
		"foo:bar":   "baz", // key with colon
		`':"foo'\"`: "baz", // all the crazy
	}

	raw := fmt.Sprintf("%q", want)
	got, err := ParseVmTags(raw)
	if err != nil {
		t.Fatalf("unable to parse tags -- %v", err)
	}

	// Check all the keys
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key: %s, got: `%v` != want: `%v`", k, got[k], v)
		}
	}
}
