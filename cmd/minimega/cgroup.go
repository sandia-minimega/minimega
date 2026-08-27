// Copyright 2025 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"

	"golang.org/x/sys/unix"
)

// cgroupName is the name of minimega's cgroup within the host hierarchy.
const cgroupName = "minimega"

// CgroupRemoveRetries is the maximum number of times to try to remove a busy
// cgroup.
const CgroupRemoveRetries = 10

type cgroupMode int

const (
	cgroupV1 cgroupMode = iota // legacy hierarchy, one mount per controller
	cgroupV2                   // unified hierarchy
)

func (m cgroupMode) String() string {
	if m == cgroupV2 {
		return "v2"
	}
	return "v1"
}

// cgroups manages minimega's cgroups on the host. Implementations exist for
// the legacy (v1) and unified (v2) hierarchies.
type cgroups interface {
	// Init creates minimega's cgroups, discarding any left behind by a
	// previous, unclean run.
	Init() error

	// Teardown removes minimega's cgroups, along with any VM cgroups still
	// inside them.
	Teardown()

	// Nuke thaws and kills every process in minimega's cgroups.
	Nuke()

	// VM returns the cgroup for a VM, which need not exist yet.
	VM(id int) vmCgroup

	Mode() cgroupMode
}

// vmCgroup is a single VM's cgroup. It is used both by minimega and by the
// container shim, which builds its own cgroups after the clone.
type vmCgroup interface {
	// Populate creates the cgroup, applies the VM's resource limits, and
	// moves the calling process into it.
	Populate(vcpus, memory int) error

	Freeze() error
	Thaw() error

	// Pids returns the processes in the cgroup and any below it.
	Pids() ([]int, error)

	// Remove deletes the cgroup, which must have no processes left in it.
	Remove() error
}

// theCgroups is minimega's cgroup hierarchy, set by containerInit. The
// container shim builds its own -- it runs in a separate process.
var theCgroups cgroups

// detectCgroups classifies the hierarchy mounted at root and returns the
// matching implementation. Hybrid hosts are treated as v1 since the legacy
// controllers minimega uses are still mounted individually there.
func detectCgroups(root string) (cgroups, error) {
	var buf unix.Statfs_t
	if err := unix.Statfs(root, &buf); err != nil {
		return nil, fmt.Errorf("statfs %v: %v", root, err)
	}

	if buf.Type == unix.CGROUP2_SUPER_MAGIC {
		return &cgroupsV2{root: root}, nil
	}

	for _, ctrl := range cgroupV1Controllers {
		p := filepath.Join(root, ctrl)
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("no cgroup hierarchy at %v: %v", p, err)
		}
	}

	return &cgroupsV1{root: root}, nil
}

// cgroupWrite writes a value to a control file, reporting the full path on
// failure since the errors are otherwise indistinguishable.
func cgroupWrite(dir, name, value string) error {
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(value), 0644); err != nil {
		return fmt.Errorf("write %v: %v", p, err)
	}

	return nil
}

// cgroupPids reads the pids from a cgroup.procs or tasks file.
func cgroupPids(path string) ([]int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pids []int

	for _, f := range strings.Fields(string(b)) {
		pid, err := strconv.Atoi(f)
		if err != nil {
			return nil, fmt.Errorf("malformed pid in %v: %v", path, f)
		}

		pids = append(pids, pid)
	}

	return pids, nil
}

// cgroupTreePids returns the pids in the cgroup rooted at dir and every cgroup
// below it. Containers running their own init create nested cgroups, so the
// VM's own procs file is not the whole story.
func cgroupTreePids(dir, procs string) ([]int, error) {
	var pids []int

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != procs {
			return nil
		}

		p, err := cgroupPids(path)
		if err != nil {
			return err
		}

		pids = append(pids, p...)
		return nil
	})

	return pids, err
}

// cgroupWalkPids calls fn for each cgroup at or below dir that has processes
// in it, passing the cgroup's path and its pids.
func cgroupWalkPids(dir, procs string, fn func(path string, pids []int)) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || info.Name() != procs {
			return nil
		}

		pids, err := cgroupPids(path)
		if err != nil {
			log.Debugln(err)
			return nil
		}

		if len(pids) > 0 {
			fn(filepath.Dir(path), pids)
		}

		return nil
	})
}

func cgroupKill(pids []int) {
	for _, pid := range pids {
		log.Info("killing process: %v", pid)

		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
			log.Error("unable to kill %v: %v", pid, err)
		}
	}
}

// cgroupRemoveTree removes dir and every cgroup below it. Children have to go
// before their parent, so the walk is depth-first, and only directories are
// removed -- control files cannot be unlinked.
func cgroupRemoveTree(dir string) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}

		if err := cgroupRemoveTree(filepath.Join(dir, ent.Name())); err != nil {
			return err
		}
	}

	return cgroupRemove(dir)
}

// cgroupRemove removes a single, empty cgroup. Processes leave a cgroup as
// they exit, but it stays busy until they have been reaped, so a cgroup that
// has just been emptied may need a moment.
func cgroupRemove(dir string) error {
	for i := 0; i < CgroupRemoveRetries; i++ {
		err := os.Remove(dir)
		if err == nil {
			return nil
		}

		if err, ok := err.(*os.PathError); ok && err.Err == syscall.EBUSY {
			log.Debug("cgroup %v busy, sleeping", dir)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return err
	}

	return fmt.Errorf("cgroup %v still busy", dir)
}

// cgroupCPUQuota converts a VCPU count into a CFS bandwidth period and quota,
// both in microseconds. Limiting a container to that much run-time per period
// emulates the requested number of CPUs. A one second period allows a high
// burst capacity. Based on:
//
// https://www.kernel.org/doc/Documentation/scheduler/sched-bwc.txt
func cgroupCPUQuota(vcpus int) (period, quota int64) {
	period = time.Second.Nanoseconds() / 1000
	return period, int64(vcpus) * period
}
