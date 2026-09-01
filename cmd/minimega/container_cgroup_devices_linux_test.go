// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
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

func TestContainerDeviceFilterProgram(t *testing.T) {
	rules := make([]containerDeviceRule, 0, len(containerDevices))
	for _, value := range containerDevices {
		rule, err := parseContainerDeviceRule(value)
		if err != nil {
			t.Fatalf("parse %q: %v", value, err)
		}
		rules = append(rules, rule)
	}

	program := containerDeviceFilterProgram(rules)
	tests := []struct {
		name       string
		deviceType int32
		major      int32
		minor      int32
		access     int32
		want       bool
	}{
		{"read null", bpfDeviceChar, 1, 3, bpfAccessRead, true},
		{"write null", bpfDeviceChar, 1, 3, bpfAccessWrite, true},
		{"create char device", bpfDeviceChar, 8, 0, bpfAccessMknod, true},
		{"read unlisted char device", bpfDeviceChar, 8, 0, bpfAccessRead, false},
		{"create block device", bpfDeviceBlock, 8, 0, bpfAccessMknod, true},
		{"read block device", bpfDeviceBlock, 8, 0, bpfAccessRead, false},
		{"read unlisted device", bpfDeviceChar, 10, 229, bpfAccessRead, false},
		{"read and create unlisted device", bpfDeviceChar, 8, 0, bpfAccessRead | bpfAccessMknod, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := evaluateContainerDeviceFilter(t, program, test.deviceType, test.major, test.minor, test.access)
			if got != test.want {
				t.Fatalf("device access = %v, want %v", got, test.want)
			}
		})
	}
}

func TestParseContainerDeviceRuleErrors(t *testing.T) {
	for _, value := range []string{"", "x 1:2 r", "c 1 r", "c -1:2 r", "c 1:2 x"} {
		if _, err := parseContainerDeviceRule(value); err == nil {
			t.Errorf("parseContainerDeviceRule(%q) succeeded", value)
		}
	}
}

func TestContainerCgroupV2Integration(t *testing.T) {
	if os.Getenv("MINIMEGA_CGROUP_TEST") == "" {
		t.Skip("set MINIMEGA_CGROUP_TEST to test against the host cgroup hierarchy")
	}

	root := os.Getenv("MINIMEGA_CGROUP_ROOT")
	if root == "" {
		root = "/sys/fs/cgroup"
	}
	if !containerCgroupV2(root) {
		t.Skip("requires cgroup v2")
	}
	if err := containerInitV2(root); err != nil {
		t.Fatalf("initialize cgroup v2: %v", err)
	}

	parent := containerCgroupParent(root, "")
	cgroup := filepath.Join(parent, "1625")
	cleanupCgroup := filepath.Join(root, "test-cleanup")
	if err := os.Mkdir(cleanupCgroup, 0755); err != nil && !os.IsExist(err) {
		t.Fatalf("create cleanup cgroup: %v", err)
	}
	defer func() {
		pid := []byte(strconv.Itoa(os.Getpid()))
		if err := os.WriteFile(filepath.Join(cleanupCgroup, "cgroup.procs"), pid, 0644); err != nil {
			t.Errorf("move test process to cleanup cgroup: %v", err)
			return
		}
		if err := os.Remove(cgroup); err != nil {
			t.Errorf("remove test cgroup: %v", err)
		}
		if err := os.Remove(parent); err != nil {
			t.Errorf("remove minimega cgroup: %v", err)
		}
	}()

	if err := containerPopulateCgroupsV2(root, 1625, 2, 512); err != nil {
		t.Fatalf("populate cgroup v2: %v", err)
	}
	assertCgroupFile(t, filepath.Join(cgroup, "cpu.max"), "2000000 1000000")
	assertCgroupFile(t, filepath.Join(cgroup, "memory.max"), "536870912")

	allowed, err := os.Open("/dev/null")
	if err != nil {
		t.Fatalf("open allowed device: %v", err)
	}
	allowed.Close()

	blockedPath := filepath.Join(t.TempDir(), "blocked-device")
	if err := unix.Mknod(blockedPath, unix.S_IFCHR|0600, int(unix.Mkdev(1, 1))); err != nil {
		t.Fatalf("create test device: %v", err)
	}
	if blocked, err := os.Open(blockedPath); err == nil {
		blocked.Close()
		t.Fatal("opened device absent from allow list")
	}
}

func evaluateContainerDeviceFilter(t *testing.T, program []bpfInstruction, deviceType, major, minor, access int32) bool {
	t.Helper()

	var registers [6]int32
	context := [3]int32{access<<16 | deviceType, major, minor}
	for pc := 0; pc < len(program); pc++ {
		instruction := program[pc]
		destination := instruction.dstSource & 0xf
		source := instruction.dstSource >> 4

		switch instruction.code {
		case bpfLoadWordCode:
			registers[destination] = context[instruction.offset/4]
		case bpfMoveRegisterCode:
			registers[destination] = registers[source]
		case bpfRightShiftCode:
			registers[destination] = int32(uint32(registers[destination]) >> instruction.immediate)
		case bpfAndCode:
			registers[destination] &= instruction.immediate
		case bpfJumpNotEqualCode:
			if registers[destination] != instruction.immediate {
				pc += int(instruction.offset)
			}
		case bpfJumpSetCode:
			if registers[destination]&instruction.immediate != 0 {
				pc += int(instruction.offset)
			}
		case bpfMoveImmediateCode:
			registers[destination] = instruction.immediate
		case bpfExitCode:
			return registers[0] != 0
		default:
			t.Fatalf("unsupported BPF instruction %#x", instruction.code)
		}
	}

	t.Fatal("BPF program did not exit")
	return false
}
