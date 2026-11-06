// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/hex"
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestVMTypeRoundTrip(t *testing.T) {
	for _, want := range []VMType{KVM, CONTAINER, ANDROID, FIRECRACKER} {
		got, err := ParseVMType(want.String())
		if err != nil {
			t.Errorf("ParseVMType(%q): %v", want.String(), err)
			continue
		}
		if got != want {
			t.Errorf("ParseVMType(%q) = %v, want %v", want.String(), got, want)
		}
	}

	if got := FIRECRACKER.String(); got != "firecracker" {
		t.Errorf("FIRECRACKER.String() = %q, want %q", got, "firecracker")
	}
}

func TestNewVMFirecracker(t *testing.T) {
	vm, err := NewVM("fc0", DefaultNamespace, FIRECRACKER, NewVMConfig())
	if err != nil {
		t.Fatal(err)
	}

	fc, ok := vm.(*FirecrackerVM)
	if !ok {
		t.Fatalf("NewVM returned %T, want *FirecrackerVM", vm)
	}
	defer fc.lock.Unlock()

	if got := fc.GetType(); got != FIRECRACKER {
		t.Errorf("GetType() = %v, want %v", got, FIRECRACKER)
	}
}

func testConflictVM(vmType VMType, name string) VM {
	base := &BaseVM{
		Name: name,
		Type: vmType,
	}
	base.UUID = name + "-uuid"

	switch vmType {
	case KVM:
		return &KvmVM{BaseVM: base}
	case CONTAINER:
		return &ContainerVM{BaseVM: base}
	case ANDROID:
		return &AndroidVM{BaseVM: base}
	case FIRECRACKER:
		return &FirecrackerVM{BaseVM: base}
	default:
		panic("unknown VM type")
	}
}

func TestFirecrackerConflicts(t *testing.T) {
	for _, vmType := range []VMType{KVM, CONTAINER, ANDROID, FIRECRACKER} {
		t.Run(vmType.String(), func(t *testing.T) {
			other := testConflictVM(vmType, "other")
			fc := testConflictVM(FIRECRACKER, "fc")

			if err := other.Conflicts(fc); err != nil {
				t.Errorf("%v.Conflicts(firecracker) with distinct names: %v", vmType, err)
			}
			if err := fc.Conflicts(other); err != nil {
				t.Errorf("firecracker.Conflicts(%v) with distinct names: %v", vmType, err)
			}

			dup := testConflictVM(vmType, "fc")

			if err := dup.Conflicts(fc); err == nil {
				t.Errorf("%v.Conflicts(firecracker) with duplicate name: want error", vmType)
			}
			if err := fc.Conflicts(dup); err == nil {
				t.Errorf("firecracker.Conflicts(%v) with duplicate name: want error", vmType)
			}
		})
	}
}

func TestDetectCgroupVersion(t *testing.T) {
	statfs := func(magic int64, err error) func(string, *unix.Statfs_t) error {
		return func(path string, st *unix.Statfs_t) error {
			if path != cgroupRoot {
				t.Errorf("statfs path = %q, want %q", path, cgroupRoot)
			}
			st.Type = magic
			return err
		}
	}

	tests := []struct {
		name  string
		magic int64
		err   error
		want  int
	}{
		{name: "cgroup2", magic: unix.CGROUP2_SUPER_MAGIC, want: 2},
		{name: "tmpfs", magic: unix.TMPFS_MAGIC, want: 1},
		{name: "cgroup1", magic: unix.CGROUP_SUPER_MAGIC, want: 1},
		{name: "error", err: errors.New("no such file"), want: 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectCgroupVersion(statfs(test.magic, test.err)); got != test.want {
				t.Errorf("detectCgroupVersion() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestInstanceName(t *testing.T) {
	name := instanceName("/tmp/minimega")

	if len(name) != 6 {
		t.Fatalf("instanceName() = %q, want 6 characters", name)
	}
	if _, err := hex.DecodeString(name); err != nil {
		t.Errorf("instanceName() = %q, want hex: %v", name, err)
	}

	for _, base := range []string{"/tmp/minimega/", "/tmp//minimega", "/tmp/x/../minimega"} {
		if got := instanceName(base); got != name {
			t.Errorf("instanceName(%q) = %q, want %q", base, got, name)
		}
	}

	if got := instanceName("/tmp/other"); got == name {
		t.Errorf("instanceName(%q) = %q, same as /tmp/minimega", "/tmp/other", got)
	}
}

func TestFirecrackerHostNames(t *testing.T) {
	old := fcInstance
	defer func() { fcInstance = old }()

	fcInstance = "a1b2c3"

	if got, want := fcNetnsName(7), "mm-fc-a1b2c3-7"; got != want {
		t.Errorf("fcNetnsName(7) = %q, want %q", got, want)
	}
	if got, want := fcParentCgroup(), "minimega-fc-a1b2c3"; got != want {
		t.Errorf("fcParentCgroup() = %q, want %q", got, want)
	}
}

func TestFirecrackerConfigDefaults(t *testing.T) {
	cfg := NewVMConfig()

	tests := map[string]string{
		"firecracker-entropy":  "true",
		"firecracker-pci":      "false",
		"firecracker-path":     "firecracker",
		"jailer-path":          "jailer",
		"vsock-cid":            "3",
		"firecracker-snapshot": "",
	}

	for field, want := range tests {
		got, err := cfg.FirecrackerConfig.Info(field)
		if err != nil {
			t.Errorf("Info(%q): %v", field, err)
			continue
		}
		if got != want {
			t.Errorf("Info(%q) = %q, want %q", field, got, want)
		}
	}

	if got, err := cfg.BaseConfig.Info("cc-policy"); err != nil || got != "" {
		t.Errorf("Info(%q) = %q, %v, want empty", "cc-policy", got, err)
	}
}

func TestFirecrackerConfigValidation(t *testing.T) {
	cfg := NewVMConfig()

	for _, v := range []string{"", "strict", "legacy"} {
		if err := validCCPolicy(cfg, v); err != nil {
			t.Errorf("validCCPolicy(%q): %v", v, err)
		}
	}
	if err := validCCPolicy(cfg, "loose"); err == nil {
		t.Error("validCCPolicy(loose): want error")
	}

	for _, v := range []uint64{MinVsockCID, MaxVsockCID} {
		if err := validVsockCID(cfg, v); err != nil {
			t.Errorf("validVsockCID(%v): %v", v, err)
		}
	}
	for _, v := range []uint64{0, 2, MaxVsockCID + 1} {
		if err := validVsockCID(cfg, v); err == nil {
			t.Errorf("validVsockCID(%v): want error", v)
		}
	}
}
