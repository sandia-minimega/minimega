// Copyright 2025 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDetectCgroupsV1(t *testing.T) {
	root := t.TempDir()

	for _, ctrl := range cgroupV1Controllers {
		if err := os.Mkdir(filepath.Join(root, ctrl), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cg, err := detectCgroups(root)
	if err != nil {
		t.Fatalf("detectCgroups: %v", err)
	}

	if cg.Mode() != cgroupV1 {
		t.Errorf("got %v, want v1", cg.Mode())
	}
}

func TestDetectCgroupsMissingController(t *testing.T) {
	root := t.TempDir()

	// every controller but the last one
	for _, ctrl := range cgroupV1Controllers[:len(cgroupV1Controllers)-1] {
		if err := os.Mkdir(filepath.Join(root, ctrl), 0755); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := detectCgroups(root); err == nil {
		t.Error("expected an error for an incomplete hierarchy")
	}
}

func TestDetectCgroupsV2(t *testing.T) {
	const root = "/sys/fs/cgroup"

	var buf unix.Statfs_t
	if err := unix.Statfs(root, &buf); err != nil {
		t.Skipf("statfs %v: %v", root, err)
	}
	if buf.Type != unix.CGROUP2_SUPER_MAGIC {
		t.Skipf("%v is not a unified hierarchy", root)
	}

	cg, err := detectCgroups(root)
	if err != nil {
		t.Fatalf("detectCgroups: %v", err)
	}

	if cg.Mode() != cgroupV2 {
		t.Errorf("got %v, want v2", cg.Mode())
	}
}

func TestCgroupCPUQuota(t *testing.T) {
	// one second period, one second of run-time per VCPU
	period, quota := cgroupCPUQuota(4)
	if period != 1000000 || quota != 4000000 {
		t.Errorf("got period %v quota %v, want 1000000 and 4000000", period, quota)
	}
}

func TestCgroupRemoveTree(t *testing.T) {
	root := t.TempDir()

	// a container running its own init nests cgroups below the VM's, and
	// those have to be removed before it
	nested := filepath.Join(root, "minimega", "0", "init.scope")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}

	if err := cgroupRemoveTree(filepath.Join(root, "minimega")); err != nil {
		t.Fatalf("cgroupRemoveTree: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "minimega")); !os.IsNotExist(err) {
		t.Errorf("tree not removed: %v", err)
	}
}

func TestCgroupRemoveTreeMissing(t *testing.T) {
	if err := cgroupRemoveTree(filepath.Join(t.TempDir(), "nope")); err != nil {
		t.Errorf("expected no error for a missing tree, got %v", err)
	}
}

// read is a helper for asserting on control file contents.
func readCgroupFile(t *testing.T, elem ...string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join(elem...))
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

// The hierarchies are ordinary directory trees, so the layout and the values
// written into it can be checked without a real cgroup mount. Note that the
// devices controller is not covered: it takes one write per device, which a
// real cgroupfs accumulates but a regular file does not.
func TestCgroupV1Layout(t *testing.T) {
	root := t.TempDir()

	for _, ctrl := range cgroupV1Controllers {
		if err := os.Mkdir(filepath.Join(root, ctrl), 0755); err != nil {
			t.Fatal(err)
		}
	}

	cg := &cgroupsV1{root: root}
	if err := cg.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	for _, ctrl := range cgroupV1Controllers {
		if got := readCgroupFile(t, root, ctrl, cgroupName, "cgroup.clone_children"); got != "1" {
			t.Errorf("%v clone_children = %q, want 1", ctrl, got)
		}
	}
	if got := readCgroupFile(t, root, "memory", cgroupName, "memory.use_hierarchy"); got != "1" {
		t.Errorf("use_hierarchy = %q, want 1", got)
	}

	vm := cg.VM(7)
	if err := vm.Populate(2, 512); err != nil {
		t.Fatalf("populate: %v", err)
	}

	for _, tc := range []struct{ ctrl, name, want string }{
		{"cpu", "cpu.cfs_period_us", "1000000"},
		{"cpu", "cpu.cfs_quota_us", "2000000"},
		{"memory", "memory.limit_in_bytes", "512M"},
		{"devices", "devices.deny", "a"},
	} {
		if got := readCgroupFile(t, root, tc.ctrl, cgroupName, "7", tc.name); got != tc.want {
			t.Errorf("%v = %q, want %q", tc.name, got, tc.want)
		}
	}

	pids, err := vm.Pids()
	if err != nil {
		t.Fatalf("pids: %v", err)
	}
	if len(pids) != 1 || pids[0] != os.Getpid() {
		t.Errorf("pids = %v, want [%v]", pids, os.Getpid())
	}

	if err := vm.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if got := readCgroupFile(t, root, "freezer", cgroupName, "7", "freezer.state"); got != "FROZEN" {
		t.Errorf("freezer.state = %q, want FROZEN", got)
	}
	if err := vm.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	if got := readCgroupFile(t, root, "freezer", cgroupName, "7", "freezer.state"); got != "THAWED" {
		t.Errorf("freezer.state = %q, want THAWED", got)
	}
}

func TestCgroupV2Layout(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset cpu io memory pids\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// stand in for the control files a real cgroupfs creates. Init tears down
	// leftovers first, but a directory holding files cannot be removed, so the
	// fixture survives -- the teardown error it logs here is expected.
	if err := os.MkdirAll(filepath.Join(root, cgroupName), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, cgroupName, "cgroup.freeze"), []byte("0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cg := &cgroupsV2{root: root}
	if err := cg.Init(); err != nil {
		t.Fatalf("init: %v", err)
	}

	want := "+cpu +memory"
	if got := readCgroupFile(t, root, "cgroup.subtree_control"); got != want {
		t.Errorf("root subtree_control = %q, want %q", got, want)
	}
	if got := readCgroupFile(t, root, cgroupName, "cgroup.subtree_control"); got != want {
		t.Errorf("minimega subtree_control = %q, want %q", got, want)
	}

	vm := cg.VM(7)
	if err := vm.Populate(2, 512); err != nil {
		t.Fatalf("populate: %v", err)
	}

	for _, tc := range []struct{ name, want string }{
		{"cpu.max", "2000000 1000000"},
		{"memory.max", "536870912"},
		{"cgroup.procs", strconv.Itoa(os.Getpid())},
	} {
		if got := readCgroupFile(t, root, cgroupName, "7", tc.name); got != tc.want {
			t.Errorf("%v = %q, want %q", tc.name, got, tc.want)
		}
	}

	if err := vm.Freeze(); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if got := readCgroupFile(t, root, cgroupName, "7", "cgroup.freeze"); got != "1" {
		t.Errorf("cgroup.freeze = %q, want 1", got)
	}
	if err := vm.Thaw(); err != nil {
		t.Fatalf("thaw: %v", err)
	}
	if got := readCgroupFile(t, root, cgroupName, "7", "cgroup.freeze"); got != "0" {
		t.Errorf("cgroup.freeze = %q, want 0", got)
	}
}

// the freezer is how containers are paused, and it is what a kernel older
// than 5.2 is missing
func TestCgroupV2MissingFreezer(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset cpu io memory pids\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cg := &cgroupsV2{root: root}
	if err := cg.Init(); err == nil {
		t.Error("expected an error when the freezer is missing")
	}
}

func TestCgroupV2MissingController(t *testing.T) {
	root := t.TempDir()

	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpuset io pids\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cg := &cgroupsV2{root: root}
	if err := cg.Init(); err == nil {
		t.Error("expected an error when cpu and memory are not delegated")
	}
}
