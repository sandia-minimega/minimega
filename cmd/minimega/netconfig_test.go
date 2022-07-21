// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"testing"
)

func TestParseNetConfig(t *testing.T) {
	nics := map[string]bool{
		"e1000":          true,
		"virtio-net-pci": true,
	}

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
		r, err := ParseNetConfig(s, nics)
		if err != nil {
			t.Fatalf("unable to parse `%v`: %v", s, err)
		}

		got := r.String()
		if got != s {
			t.Fatalf("unequal: `%v` != `%v`", s, got)
		}
	}
}
