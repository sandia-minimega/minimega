// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestClearAffinityFilterKeepsAffinityDisabled(t *testing.T) {
	ns := &Namespace{VMs: VMs{m: map[int]VM{}}, affinityFilter: []string{"1"}}

	c := &minicli.Command{BoolArgs: map[string]bool{"filter": true}}
	if err := cliOptimizeClear(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if len(ns.affinityFilter) != 0 {
		t.Errorf("filter not cleared: %v", ns.affinityFilter)
	}
	if ns.affinityEnabled {
		t.Error("clear optimize affinity filter enabled affinity")
	}
}
