// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"testing"
)

func TestParseNetConfig(t *testing.T) {
	examples := []string{
		"foo",
		"foo,virtio-net-pci",
		"foo,de:ad:be:ef:ca:fe",
		"foo,de:ad:be:ef:ca:fe,virtio-net-pci",

		"my_bridge,foo",
		"my_bridge,foo,virtio-net-pci",
		"my_bridge,foo,de:ad:be:ef:ca:fe",
		"my_bridge,foo,de:ad:be:ef:ca:fe,virtio-net-pci",
	}

	for _, s := range examples {
		r, err := ParseNetConfig(s)
		if err != nil {
			t.Fatalf("unable to parse `%v`: %v", s, err)
		}

		got := r.String()
		if got != s {
			t.Fatalf("unequal: `%v` != `%v`", s, got)
		}
	}
}
