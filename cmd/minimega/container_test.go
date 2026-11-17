// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
)

// TestContainerMountsSkipMissingPaths checks that masked and read-only /proc
// paths missing from the kernel, such as /proc/kcore without CONFIG_PROC_KCORE
// or /proc/sysrq-trigger without CONFIG_MAGIC_SYSRQ, do not fail the launch.
func TestContainerMountsSkipMissingPaths(t *testing.T) {
	fsPath := t.TempDir()

	// without CAP_SYS_ADMIN (e.g. a Docker build), mount fails before the lookup
	err := syscall.Mount("/dev/null", filepath.Join(fsPath, "missing"), "", syscall.MS_BIND, "")
	if !errors.Is(err, syscall.ENOENT) {
		t.Skipf("mount on a missing path returned %v, not ENOENT", err)
	}

	if err := containerMaskPaths(fsPath); err != nil {
		t.Errorf("containerMaskPaths: %v", err)
	}

	if err := containerRemountReadOnly(fsPath); err != nil {
		t.Errorf("containerRemountReadOnly: %v", err)
	}
}
