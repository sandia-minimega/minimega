// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package iomeshage

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// preallocateFile reserves size bytes on disk for f, failing with ENOSPC when
// the file system is too small.
func preallocateFile(f *os.File, size int64) error {
	err := unix.Fallocate(int(f.Fd()), 0, 0, size)
	if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
		// the file system cannot reserve space, so a full disk shows up on write
		return f.Truncate(size)
	}

	return err
}
