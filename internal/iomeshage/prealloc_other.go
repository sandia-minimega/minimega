// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build !linux

package iomeshage

import "os"

// preallocateFile sizes f without reserving space, so a full disk shows up on write.
func preallocateFile(f *os.File, size int64) error {
	return f.Truncate(size)
}
