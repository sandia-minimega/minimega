// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestVLANsAddMeshageSourceDoesNotBroadcast(t *testing.T) {
	var (
		originalVLANs       = vlans.Default
		originalBase        = *f_base
		originalMeshageNode = meshageNode
	)

	defer func() {
		vlans.Default = originalVLANs
		*f_base = originalBase
		meshageNode = originalMeshageNode
	}()

	vlans.Default = vlans.NewVLANs()
	*f_base = t.TempDir()
	meshageNode = nil

	c := &minicli.Command{
		StringArgs: map[string]string{
			"alias": "mesh-alias",
			"vlan":  "101",
		},
		Source: "meshage",
	}

	if err := cliVLANsAdd(&Namespace{Name: "test"}, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if vlan, err := vlans.ParseVLAN("test", "mesh-alias"); err != nil || vlan != 101 {
		t.Fatalf("got VLAN %d, %v; want 101, nil", vlan, err)
	}

	if _, err := os.Stat(filepath.Join(*f_base, "vlans")); err != nil {
		t.Fatal(err)
	}
}
