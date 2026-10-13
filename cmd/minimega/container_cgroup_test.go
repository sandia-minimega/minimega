// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestContainerCgroupV1(t *testing.T) {
	root := t.TempDir()

	if containerCgroupV2(root) {
		t.Fatal("empty cgroup root detected as v2")
	}
	if err := containerInitV1(root); err != nil {
		t.Fatalf("initialize cgroup v1: %v", err)
	}
	if err := containerPopulateCgroupsV1(root, 7, 2, 512); err != nil {
		t.Fatalf("populate cgroup v1: %v", err)
	}

	assertCgroupFile(t, filepath.Join(root, "cpu", "minimega", "7", "cpu.cfs_period_us"), "1000000")
	assertCgroupFile(t, filepath.Join(root, "cpu", "minimega", "7", "cpu.cfs_quota_us"), "2000000")
	assertCgroupFile(t, filepath.Join(root, "memory", "minimega", "7", "memory.limit_in_bytes"), "512M")

	for _, cgroup := range containerCgroupPaths(root, "7") {
		assertCgroupFile(t, filepath.Join(cgroup, "cgroup.procs"), strconv.Itoa(os.Getpid()))
	}

	freezer := filepath.Join(root, "freezer", "minimega", "7")
	path, value := containerCgroupFreezeSetting(root, freezer, true)
	if path != filepath.Join(freezer, "freezer.state") || value != "FROZEN" {
		t.Fatalf("unexpected v1 freeze setting: %q %q", path, value)
	}
	if got := containerCgroupProcessFile(root, freezer); got != filepath.Join(freezer, "tasks") {
		t.Fatalf("unexpected v1 process file: %q", got)
	}
}

func TestContainerCgroupV2(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), []byte("cpu memory"), 0644); err != nil {
		t.Fatal(err)
	}

	if !containerCgroupV2(root) {
		t.Fatal("unified cgroup root not detected as v2")
	}
	if err := containerInitV2(root); err != nil {
		t.Fatalf("initialize cgroup v2: %v", err)
	}

	parent := filepath.Join(root, "minimega")
	assertCgroupFile(t, filepath.Join(root, "cgroup.subtree_control"), "+cpu +memory")
	assertCgroupFile(t, filepath.Join(parent, "cgroup.subtree_control"), "+cpu +memory")

	var filterCgroup string
	var filterDevices []string
	attachDeviceFilter := func(cgroup string, devices []string) error {
		filterCgroup = cgroup
		filterDevices = append([]string(nil), devices...)
		return nil
	}
	if err := containerPopulateCgroupsV2WithDeviceFilter(root, 7, 2, 512, attachDeviceFilter); err != nil {
		t.Fatalf("populate cgroup v2: %v", err)
	}

	cgroup := filepath.Join(parent, "7")
	if filterCgroup != cgroup {
		t.Fatalf("device filter cgroup = %q, want %q", filterCgroup, cgroup)
	}
	if len(filterDevices) != len(containerDevices) {
		t.Fatalf("device filter has %d rules, want %d", len(filterDevices), len(containerDevices))
	}

	assertCgroupFile(t, filepath.Join(cgroup, "cpu.max"), "2000000 1000000")
	assertCgroupFile(t, filepath.Join(cgroup, "memory.max"), "536870912")
	assertCgroupFile(t, filepath.Join(cgroup, "cgroup.procs"), strconv.Itoa(os.Getpid()))

	paths := containerCgroupPaths(root, "7")
	if len(paths) != 1 || paths[0] != cgroup {
		t.Fatalf("unexpected v2 cgroup paths: %v", paths)
	}

	path, value := containerCgroupFreezeSetting(root, cgroup, true)
	if path != filepath.Join(cgroup, "cgroup.freeze") || value != "1" {
		t.Fatalf("unexpected v2 freeze setting: %q %q", path, value)
	}
	path, value = containerCgroupFreezeSetting(root, cgroup, false)
	if path != filepath.Join(cgroup, "cgroup.freeze") || value != "0" {
		t.Fatalf("unexpected v2 thaw setting: %q %q", path, value)
	}
	if got := containerCgroupProcessFile(root, cgroup); got != filepath.Join(cgroup, "cgroup.procs") {
		t.Fatalf("unexpected v2 process file: %q", got)
	}
}

func TestContainerNukeWalker(t *testing.T) {
	tests := []struct {
		name         string
		v2           bool
		processFile  string
		freezerFile  string
		freezerValue string
	}{
		{"v1", false, "tasks", "freezer.state", "THAWED"},
		{"v2", true, "cgroup.procs", "cgroup.freeze", "0"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if test.v2 {
				if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			setContainerCgroupRoot(t, root)

			cgroup := filepath.Join(containerCgroupParent(root, "freezer"), "7")
			if err := os.MkdirAll(cgroup, 0755); err != nil {
				t.Fatal(err)
			}
			freezer := filepath.Join(cgroup, test.freezerFile)
			if err := os.WriteFile(freezer, nil, 0644); err != nil {
				t.Fatal(err)
			}
			processes := filepath.Join(cgroup, test.processFile)
			if err := os.WriteFile(processes, []byte("99999999"), 0644); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(processes)
			if err != nil {
				t.Fatal(err)
			}

			if err := containerNukeWalker(processes, info, nil); err != nil {
				t.Fatalf("walk process file: %v", err)
			}
			assertCgroupFile(t, freezer, test.freezerValue)
		})
	}
}

func TestContainerCleanCgroupDirs(t *testing.T) {
	for _, v2 := range []bool{false, true} {
		name := "v1"
		if v2 {
			name = "v2"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if v2 {
				if err := os.WriteFile(filepath.Join(root, "cgroup.controllers"), nil, 0644); err != nil {
					t.Fatal(err)
				}
			}
			setContainerCgroupRoot(t, root)

			for _, parent := range containerCgroupParents(root) {
				if err := os.MkdirAll(filepath.Join(parent, "7"), 0755); err != nil {
					t.Fatal(err)
				}
			}

			containerCleanCgroupDirs()
			for _, parent := range containerCgroupParents(root) {
				if _, err := os.Stat(parent); !os.IsNotExist(err) {
					t.Errorf("cgroup parent still exists: %q", parent)
				}
			}
		})
	}
}

func setContainerCgroupRoot(t *testing.T, root string) {
	t.Helper()

	original := *f_cgroup
	*f_cgroup = root
	t.Cleanup(func() {
		*f_cgroup = original
	})
}

func assertCgroupFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	if strings.TrimSpace(string(got)) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
