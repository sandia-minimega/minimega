// Copyright 2025 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// cgroupV1Controllers are the legacy controllers minimega uses. Each is a
// separate mount under the cgroup root.
var cgroupV1Controllers = []string{"freezer", "memory", "devices", "cpu"}

// cgroupsV1 implements cgroups for the legacy hierarchy.
type cgroupsV1 struct {
	root string
}

func (c *cgroupsV1) Mode() cgroupMode { return cgroupV1 }

func (c *cgroupsV1) VM(id int) vmCgroup {
	return &cgroupV1VM{root: c.root, id: id}
}

// paths returns minimega's cgroup in each controller.
func (c *cgroupsV1) paths() []string {
	var paths []string

	for _, ctrl := range cgroupV1Controllers {
		paths = append(paths, filepath.Join(c.root, ctrl, cgroupName))
	}

	return paths
}

func (c *cgroupsV1) Init() error {
	log.Debug("cgroup v1 init: %v", c.root)

	c.Teardown()

	for _, path := range c.paths() {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("cgroup mkdir: %v", err)
		}

		// inherit cpusets
		if err := cgroupWrite(path, "cgroup.clone_children", "1"); err != nil {
			return err
		}
	}

	memory := filepath.Join(c.root, "memory", cgroupName)

	return cgroupWrite(memory, "memory.use_hierarchy", "1")
}

func (c *cgroupsV1) Teardown() {
	for _, path := range c.paths() {
		if err := cgroupRemoveTree(path); err != nil {
			log.Errorln(err)
		}
	}
}

func (c *cgroupsV1) Nuke() {
	for _, path := range c.paths() {
		cgroupWalkPids(path, "tasks", func(dir string, pids []int) {
			// a frozen container cannot be killed, and this walk may be in a
			// controller other than the freezer, so thaw by VM id
			freezer := filepath.Join(c.root, "freezer", cgroupName, filepath.Base(dir))
			if err := cgroupWrite(freezer, "freezer.state", "THAWED"); err != nil {
				log.Debugln(err)
			}

			cgroupKill(pids)
		})
	}
}

// cgroupV1VM implements vmCgroup for the legacy hierarchy, where a VM has a
// cgroup in each controller.
type cgroupV1VM struct {
	root string
	id   int
}

// path returns the VM's cgroup in a single controller.
func (c *cgroupV1VM) path(ctrl string) string {
	return filepath.Join(c.root, ctrl, cgroupName, strconv.Itoa(c.id))
}

func (c *cgroupV1VM) paths() []string {
	var paths []string

	for _, ctrl := range cgroupV1Controllers {
		paths = append(paths, c.path(ctrl))
	}

	return paths
}

func (c *cgroupV1VM) Populate(vcpus, memory int) error {
	for _, path := range c.paths() {
		if err := os.MkdirAll(path, 0755); err != nil {
			return err
		}
	}

	// devices
	devices := c.path("devices")
	if err := cgroupWrite(devices, "devices.deny", "a"); err != nil {
		return err
	}
	for _, a := range containerDevices {
		if err := cgroupWrite(devices, "devices.allow", a); err != nil {
			return err
		}
	}

	// cpu
	period, quota := cgroupCPUQuota(vcpus)
	cpu := c.path("cpu")
	if err := cgroupWrite(cpu, "cpu.cfs_period_us", strconv.FormatInt(period, 10)); err != nil {
		return err
	}
	if err := cgroupWrite(cpu, "cpu.cfs_quota_us", strconv.FormatInt(quota, 10)); err != nil {
		return err
	}

	// memory
	if err := cgroupWrite(c.path("memory"), "memory.limit_in_bytes", fmt.Sprintf("%vM", memory)); err != nil {
		return err
	}

	// associate the pid with these permissions
	for _, path := range c.paths() {
		if err := cgroupWrite(path, "cgroup.procs", strconv.Itoa(os.Getpid())); err != nil {
			return err
		}
	}

	return nil
}

func (c *cgroupV1VM) Freeze() error {
	return cgroupWrite(c.path("freezer"), "freezer.state", "FROZEN")
}

func (c *cgroupV1VM) Thaw() error {
	return cgroupWrite(c.path("freezer"), "freezer.state", "THAWED")
}

func (c *cgroupV1VM) Pids() ([]int, error) {
	return cgroupTreePids(c.path("freezer"), "cgroup.procs")
}

func (c *cgroupV1VM) Remove() error {
	var first error

	for _, path := range c.paths() {
		if err := cgroupRemoveTree(path); err != nil && first == nil {
			first = err
		}
	}

	return first
}
