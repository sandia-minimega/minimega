// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func debugCPU(t *testing.T, action, file string) error {
	t.Helper()

	c := &minicli.Command{
		BoolArgs:   map[string]bool{"cpu": true, action: true},
		StringArgs: map[string]string{},
	}
	if file != "" {
		c.StringArgs["file"] = file
	}

	return cliDebug(nil, c, &minicli.Response{})
}

func TestDebugCPUStartWhileRunning(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.prof"), filepath.Join(dir, "b.prof")

	if err := debugCPU(t, "start", a); err != nil {
		t.Fatal(err)
	}
	defer debugCPU(t, "stop", "")

	if err := debugCPU(t, "start", b); err == nil {
		t.Fatal("expected error starting a second profile")
	}

	// the rejected start must not touch the filesystem
	if _, err := os.Stat(b); err == nil {
		t.Error("rejected start created its output file")
	}
}
