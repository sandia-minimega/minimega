// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
)

func TestInfoReturnsLocalListError(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "one.txt"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(base, "two.txt")); err != nil {
		t.Fatal(err)
	}

	iom, err := New(base, &meshage.Node{}, "", false)
	if err != nil {
		t.Fatal(err)
	}

	if files, err := iom.Info("*.txt"); err == nil {
		t.Fatalf("Info(*.txt) = %v, nil; want the error for the dangling symlink", files)
	}
}
