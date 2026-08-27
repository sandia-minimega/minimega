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

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// cgroupV2Controllers are the controllers minimega needs delegated to it in
// the unified hierarchy.
var cgroupV2Controllers = []string{"cpu", "memory"}

// cgroupsV2 implements cgroups for the unified hierarchy, where a VM has a
// single cgroup providing every controller.
type cgroupsV2 struct {
	root string
}

func (c *cgroupsV2) Mode() cgroupMode { return cgroupV2 }

func (c *cgroupsV2) VM(id int) vmCgroup {
	return &cgroupV2VM{path: filepath.Join(c.path(), strconv.Itoa(id))}
}

// path returns minimega's cgroup.
func (c *cgroupsV2) path() string {
	return filepath.Join(c.root, cgroupName)
}

func (c *cgroupsV2) Init() error {
	log.Debug("cgroup v2 init: %v", c.root)

	c.Teardown()

	if err := os.MkdirAll(c.path(), 0755); err != nil {
		return fmt.Errorf("cgroup mkdir: %v", err)
	}

	if err := c.delegate(); err != nil {
		return err
	}

	// containers are paused by freezing them, which the kernel only supports
	// in the unified hierarchy from 5.2 onwards
	freezer := filepath.Join(c.path(), "cgroup.freeze")
	if _, err := os.Stat(freezer); err != nil {
		return fmt.Errorf("no cgroup freezer, linux 5.2 or newer is required: %v", err)
	}

	// there is no devices controller in the unified hierarchy -- it was
	// replaced by cgroup device programs, which minimega does not attach
	log.Warn("cgroup v2: container device access is unrestricted")

	return nil
}

// delegate makes the cpu and memory controllers available to minimega's
// cgroup and to the per-VM cgroups below it.
func (c *cgroupsV2) delegate() error {
	b, err := os.ReadFile(filepath.Join(c.root, "cgroup.controllers"))
	if err != nil {
		return fmt.Errorf("read cgroup.controllers: %v", err)
	}

	avail := strings.Fields(string(b))

	for _, ctrl := range cgroupV2Controllers {
		var found bool
		for _, v := range avail {
			if v == ctrl {
				found = true
				break
			}
		}

		if !found {
			return fmt.Errorf("%v controller not delegated to %v, only have %v -- if minimega runs under a systemd unit, it needs `Delegate=%v`", ctrl, c.root, avail, strings.Join(cgroupV2Controllers, " "))
		}
	}

	enable := "+" + strings.Join(cgroupV2Controllers, " +")

	// enabling a controller in a cgroup's subtree_control makes it available
	// to that cgroup's children, so it has to be done at every level above the
	// VMs. Re-enabling an already enabled controller is a no-op.
	for _, path := range []string{c.root, c.path()} {
		if err := cgroupWrite(path, "cgroup.subtree_control", enable); err != nil {
			return err
		}
	}

	return nil
}

func (c *cgroupsV2) Teardown() {
	if err := cgroupRemoveTree(c.path()); err != nil {
		log.Errorln(err)
	}
}

func (c *cgroupsV2) Nuke() {
	// containers cannot exit while frozen, so thaw every VM first. Freezing is
	// inherited, so this has to happen at the VM level rather than wherever
	// the processes turn up.
	ents, err := os.ReadDir(c.path())
	if err != nil && !os.IsNotExist(err) {
		log.Errorln(err)
	}

	for _, ent := range ents {
		if !ent.IsDir() {
			continue
		}

		if err := cgroupWrite(filepath.Join(c.path(), ent.Name()), "cgroup.freeze", "0"); err != nil {
			log.Debugln(err)
		}
	}

	cgroupWalkPids(c.path(), "cgroup.procs", func(_ string, pids []int) {
		cgroupKill(pids)
	})
}

// cgroupV2VM implements vmCgroup for the unified hierarchy.
type cgroupV2VM struct {
	path string
}

func (c *cgroupV2VM) Populate(vcpus, memory int) error {
	if err := os.MkdirAll(c.path, 0755); err != nil {
		return err
	}

	// cpu
	period, quota := cgroupCPUQuota(vcpus)
	if err := cgroupWrite(c.path, "cpu.max", fmt.Sprintf("%v %v", quota, period)); err != nil {
		return err
	}

	// memory, which unlike the legacy hierarchy only accepts a byte count
	if err := cgroupWrite(c.path, "memory.max", strconv.FormatInt(int64(memory)<<20, 10)); err != nil {
		return err
	}

	return cgroupWrite(c.path, "cgroup.procs", strconv.Itoa(os.Getpid()))
}

func (c *cgroupV2VM) Freeze() error {
	return cgroupWrite(c.path, "cgroup.freeze", "1")
}

func (c *cgroupV2VM) Thaw() error {
	return cgroupWrite(c.path, "cgroup.freeze", "0")
}

func (c *cgroupV2VM) Pids() ([]int, error) {
	return cgroupTreePids(c.path, "cgroup.procs")
}

func (c *cgroupV2VM) Remove() error {
	return cgroupRemoveTree(c.path)
}
