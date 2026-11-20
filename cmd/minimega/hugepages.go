// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// hugetlbfsMagic is the filesystem type that statfs reports for hugetlbfs.
const hugetlbfsMagic = 0x958458f6

const (
	mebibyte = 1 << 20
	gibibyte = 1 << 30
)

// hugepageSizeFn is the function that KvmVM.launch uses to find the hugepage
// size. Tests replace it.
var hugepageSizeFn = hugepageSize

// memPathArg returns the value that follows the last -mem-path or --mem-path in
// a QEMU argument list, or "" when there is no such value.
func memPathArg(args []string) string {
	var res string

	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-mem-path" || args[i] == "--mem-path" {
			res = args[i+1]
		}
	}

	return res
}

// memArgMB returns the VM memory in MiB that QEMU uses for the -m options in
// args. QEMU merges repeated -m options and the last size wins. It returns
// false when there is no size or the size has a form that it cannot read.
func memArgMB(args []string) (uint64, bool) {
	var (
		res uint64
		ok  bool
	)

	for i := 0; i+1 < len(args); i++ {
		if args[i] != "-m" && args[i] != "--m" {
			continue
		}

		size, found, valid := parseMemSize(args[i+1])
		switch {
		case !valid:
			return 0, false
		case found:
			res, ok = size, true
		}
	}

	return res, ok
}

// parseMemSize reads the size from the value of one -m option. The last size
// in the value wins, as in QEMU. found is false when the value has no size,
// and valid is false when a size is not a whole number of MiB.
func parseMemSize(value string) (mb uint64, found, valid bool) {
	for i, field := range strings.Split(value, ",") {
		key, val, hasKey := strings.Cut(field, "=")
		if !hasKey {
			key, val = "size", field
		}

		if key != "size" || (!hasKey && i != 0) {
			continue
		}

		unit := uint64(1)
		switch {
		case val == "":
			return 0, false, false
		case strings.HasSuffix(val, "G") || strings.HasSuffix(val, "g"):
			unit, val = 1024, val[:len(val)-1]
		case strings.HasSuffix(val, "M") || strings.HasSuffix(val, "m"):
			val = val[:len(val)-1]
		}

		n, err := strconv.ParseUint(val, 10, 64)
		if err != nil {
			return 0, false, false
		}

		mb, found = n*unit, true
	}

	return mb, found, true
}

// hugepageSize returns the hugepage size in bytes of the hugetlbfs filesystem
// that contains path. When path does not exist, it uses the directory of path.
// It returns 0 when path is not on hugetlbfs.
func hugepageSize(path string) (int64, error) {
	var st syscall.Statfs_t

	err := syscall.Statfs(path, &st)
	if errors.Is(err, syscall.ENOENT) {
		// QEMU creates a missing -mem-path file in its directory
		err = syscall.Statfs(filepath.Dir(path), &st)
	}
	if err != nil {
		return 0, fmt.Errorf("unable to stat filesystem of %v: %v", path, err)
	}

	if uint32(st.Type) != hugetlbfsMagic {
		return 0, nil
	}

	// hugetlbfs reports its hugepage size as the block size
	return int64(st.Bsize), nil
}

// checkMemAlignment returns an error when memMB megabytes of VM memory is not
// a multiple of pageSize bytes. A pageSize of 0 means that no rule applies.
func checkMemAlignment(memMB uint64, pageSize int64, path string) error {
	if pageSize <= 0 || (memMB*mebibyte)%uint64(pageSize) == 0 {
		return nil
	}

	return fmt.Errorf("vm memory %v MB is not a multiple of the %v hugepage size of %v", memMB, formatPageSize(pageSize), path)
}

// formatPageSize formats a page size in bytes as GiB, MiB, or KiB when the
// size is a whole number of that unit.
func formatPageSize(size int64) string {
	switch {
	case size%gibibyte == 0:
		return fmt.Sprintf("%d GiB", size/gibibyte)
	case size%mebibyte == 0:
		return fmt.Sprintf("%d MiB", size/mebibyte)
	case size%1024 == 0:
		return fmt.Sprintf("%d KiB", size/1024)
	}

	return fmt.Sprintf("%d bytes", size)
}
