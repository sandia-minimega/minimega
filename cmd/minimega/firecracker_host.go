// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"golang.org/x/sys/unix"
)

const cgroupRoot = "/sys/fs/cgroup"

var (
	// hostCgroupVersion is the cgroup version of the host, 1 or 2. It is set
	// once at startup because it cannot change while minimega runs.
	hostCgroupVersion int

	// fcInstance is a short name for this minimega instance, derived from
	// -base. It keeps the names of host objects from two instances apart.
	fcInstance string
)

// firecrackerHostInit caches the host facts that Firecracker VMs use. It does
// not create any file, directory, or cgroup.
func firecrackerHostInit() {
	hostCgroupVersion = detectCgroupVersion(unix.Statfs)
	log.Debug("host cgroup version: %v", hostCgroupVersion)

	fcInstance = instanceName(*f_base)
	log.Debug("firecracker instance name: %v", fcInstance)
}

// detectCgroupVersion returns 2 when the cgroup root is a cgroup2 filesystem,
// else 1. Hybrid hosts use version 1.
func detectCgroupVersion(statfs func(string, *unix.Statfs_t) error) int {
	var st unix.Statfs_t
	if err := statfs(cgroupRoot, &st); err != nil {
		log.Warn("unable to statfs %v, using cgroup version 1: %v", cgroupRoot, err)
		return 1
	}

	if uint64(st.Type) == unix.CGROUP2_SUPER_MAGIC {
		return 2
	}

	return 1
}

// instanceName returns a short, stable name for the minimega instance that
// uses base: the first 6 hex characters of the SHA-256 of the clean absolute
// path.
func instanceName(base string) string {
	if abs, err := filepath.Abs(base); err == nil {
		base = abs
	}

	sum := sha256.Sum256([]byte(filepath.Clean(base)))
	return hex.EncodeToString(sum[:])[:6]
}

// fcNetnsName returns the name of the network namespace for a Firecracker VM.
func fcNetnsName(id int) string {
	return fmt.Sprintf("mm-fc-%s-%d", fcInstance, id)
}

// fcParentCgroup returns the name of the parent cgroup for Firecracker VMs.
func fcParentCgroup() string {
	return "minimega-fc-" + fcInstance
}
