// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

func TestCCFileSendToKeepsDestination(t *testing.T) {
	server, err := ron.NewServer(t.TempDir(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Destroy()

	ns := &Namespace{ccServer: server}

	// Namespace.NewCommand logs the command after ron shares it, which races.
	log.SetLevelAll(log.INFO)
	defer log.SetLevelAll(log.LevelFlag)

	// The client uses its own path rules, so Windows paths must not change.
	for _, destination := range []string{`C:/`, `//server/share/drop`, `/opt/foo/`} {
		c := &minicli.Command{
			StringArgs: map[string]string{"destination": destination},
			ListArgs:   map[string][]string{},
		}
		resp := &minicli.Response{}

		if err := cliCCFileSendTo(ns, c, resp); err != nil {
			t.Fatal(err)
		}

		cmd := server.GetCommand(resp.Data.(int))
		if cmd.FilesSendDir != destination {
			t.Errorf("destination = %q, want %q", cmd.FilesSendDir, destination)
		}
	}
}
