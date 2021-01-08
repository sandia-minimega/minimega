// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package qemu

import (
	"strings"
	"testing"
)

func TestParseCPUs(t *testing.T) {
	res, err := parseCPUs(strings.NewReader(cpusOut))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	t.Logf("parsed %v cpus", len(res))
}

func TestParseCPUsARM(t *testing.T) {
	res, err := parseCPUs(strings.NewReader(cpusOutARM))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	t.Logf("parsed %v cpus", len(res))
}

func TestParseMachines(t *testing.T) {
	res, err := parseMachines(strings.NewReader(machinesOut))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	t.Logf("parsed %v machines", len(res))
}

func TestParseNICs(t *testing.T) {
	res, err := parseNICs(strings.NewReader(deviceOut))
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	t.Logf("parsed %v nics", len(res))
}
