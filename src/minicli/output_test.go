// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"fmt"
	"testing"
)

func TestCompressHosts(t *testing.T) {
	hosts := []string{}

	for i := 0; i < 10; i++ {
		hosts = append(hosts, fmt.Sprintf("node%d", i))
		hosts = append(hosts, fmt.Sprintf("n%d", i))
		hosts = append(hosts, fmt.Sprintf("foo%d", i))
	}

	want := "foo[0-9],n[0-9],node[0-9]"
	got := compressHosts(hosts)

	if want != got {
		t.Errorf("got: `%s`, want `%s`", got, want)
	}
}

func TestCompressHostsSkip(t *testing.T) {
	hosts := []string{}

	for i := 0; i < 10; i++ {
		if i != 5 {
			hosts = append(hosts, fmt.Sprintf("n%d", i))
		}
		if i != 9 {
			hosts = append(hosts, fmt.Sprintf("node%d", i))
		}
	}

	want := "n[0-4,6-9],node[0-8]"
	got := compressHosts(hosts)

	if want != got {
		t.Errorf("got: `%s`, want `%s`", got, want)
	}
}
