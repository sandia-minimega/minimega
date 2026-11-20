// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestIOMBackingFile(t *testing.T) {
	oldInfo, oldCache := iomDiskInfo, iomBackings
	t.Cleanup(func() { iomDiskInfo, iomBackings = oldInfo, oldCache })
	iomBackings = newStatCache[string]()

	var calls int
	var fail bool
	iomDiskInfo = func(image string) (DiskInfo, error) {
		calls++
		if fail {
			return DiskInfo{}, errors.New("qemu-img failed")
		}
		return DiskInfo{Name: image, BackingFile: "base.qcow2"}, nil
	}

	dir := t.TempDir()
	img := fcWriteFile(t, filepath.Join(dir, "a.qcow2"), []byte("overlay"))

	check := func(wantBacking string, wantOK bool, wantCalls int) {
		t.Helper()
		backing, ok := iomBackingFile(img)
		if backing != wantBacking || ok != wantOK {
			t.Errorf("iomBackingFile() = %q, %v, want %q, %v", backing, ok, wantBacking, wantOK)
		}
		if calls != wantCalls {
			t.Errorf("qemu-img calls = %d, want %d", calls, wantCalls)
		}
	}

	check("base.qcow2", true, 1)
	check("base.qcow2", true, 1)

	// A changed file runs qemu-img again, and a failure is not kept.
	fcWriteFile(t, img, []byte("new overlay"))
	fail = true
	check("", false, 2)
	check("", false, 3)
	fail = false
	check("base.qcow2", true, 4)
	check("base.qcow2", true, 4)
}
