// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// newTestFCVM returns an unlocked Firecracker VM named fc1 with an instance
// path in a temporary -base.
func newTestFCVM(t *testing.T, cfg VMConfig) *FirecrackerVM {
	t.Helper()

	old := *f_base
	t.Cleanup(func() { *f_base = old })
	*f_base = t.TempDir()

	vm, err := NewFirecracker("fc1", DefaultNamespace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	vm.lock.Unlock()

	return vm
}

func cmdTestFCConfig() VMConfig {
	cfg := NewVMConfig()
	cfg.VCPUs = 2
	cfg.KernelPath = "/images/vmlinux"
	cfg.InitrdPath = "/images/initrd"
	cfg.KVMConfig.Append = []string{"console=ttyS0", "quiet"}
	cfg.Disks = DiskConfigs{{Path: "/images/rootfs.raw", SnapshotPath: "/tmp/minimega/1/disk-0.raw"}}
	cfg.FirecrackerPCI = true
	cfg.VsockCID = 7
	cfg.Injects = FCInjects{{Partition: 1, Src: "motd", Dst: "/etc/motd"}}
	return cfg
}

func cmdArgs(s map[string]string, b ...string) *minicli.Command {
	c := &minicli.Command{StringArgs: s, BoolArgs: map[string]bool{}}
	for _, k := range b {
		c.BoolArgs[k] = true
	}
	return c
}

func TestCloneVMConfigFirecracker(t *testing.T) {
	vm := newTestFCVM(t, cmdTestFCConfig())

	cfg := NewVMConfig()
	if err := cloneVMConfig(&cfg, vm); err != nil {
		t.Fatalf("cloneVMConfig() = %v", err)
	}

	// Copy gives the same empty slices that the VM and the clone have.
	src := cmdTestFCConfig()
	src.Disks[0].SnapshotPath = ""
	want := VMConfig{
		BaseConfig:        src.BaseConfig.Copy(),
		KVMConfig:         src.KVMConfig.Copy(),
		FirecrackerConfig: src.FirecrackerConfig.Copy(),
	}

	if !reflect.DeepEqual(cfg.FirecrackerConfig, want.FirecrackerConfig) {
		t.Errorf("FirecrackerConfig = %+v, want %+v", cfg.FirecrackerConfig, want.FirecrackerConfig)
	}
	if !reflect.DeepEqual(cfg.KVMConfig, want.KVMConfig) {
		t.Errorf("KVMConfig = %+v, want %+v", cfg.KVMConfig, want.KVMConfig)
	}

	base := cfg.BaseConfig
	base.UUID = ""
	if len(base.Tags) == 0 && len(want.BaseConfig.Tags) == 0 {
		base.Tags = want.BaseConfig.Tags
	}
	if !reflect.DeepEqual(base, want.BaseConfig) {
		t.Errorf("BaseConfig = %+v, want %+v", base, want.BaseConfig)
	}

	// The clone does not share slices with the VM.
	vm.Injects[0].Dst = "/changed"
	if cfg.Injects[0].Dst != "/etc/motd" {
		t.Errorf("clone inject dst = %v after a change to the VM", cfg.Injects[0].Dst)
	}
	if vm.Disks[0].SnapshotPath == "" {
		t.Errorf("clone cleared the snapshot path of the VM disk")
	}
}

// testUnknownVM is a VM type that no command knows.
type testUnknownVM struct {
	FirecrackerVM
}

func TestCloneVMConfigUnknownType(t *testing.T) {
	cfg := NewVMConfig()
	vm := &testUnknownVM{FirecrackerVM{BaseVM: &BaseVM{Name: "x", Type: KVM}}}
	if err := cloneVMConfig(&cfg, vm); err == nil {
		t.Errorf("cloneVMConfig(unknown type) = nil, want error")
	}
}

func TestSaveVMConfigOnlyFirecracker(t *testing.T) {
	vm := newTestFCVM(t, cmdTestFCConfig())

	var buf bytes.Buffer
	if err := saveVMConfigOnly(&buf, vm); err != nil {
		t.Fatalf("saveVMConfigOnly() = %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "# Firecracker VM \"fc1\" is saved as configuration only.\nclear vm config\n") {
		t.Errorf("output does not start with the comment and clear vm config:\n%v", out)
	}
	for _, line := range []string{
		"vm config kernel /images/vmlinux\n",
		"vm config initrd /images/initrd\n",
		"vm config firecracker-pci true\n",
		"vm config vsock-cid 7\n",
	} {
		if !strings.Contains(out, line) {
			t.Errorf("output does not contain %q:\n%v", line, out)
		}
	}
	if !strings.HasSuffix(out, "vm launch firecracker \"fc1\"\n\n") {
		t.Errorf("output does not end with the launch command:\n%v", out)
	}

	if err := saveVMConfigOnly(&buf, testConflictVM(KVM, "k")); err == nil {
		t.Errorf("saveVMConfigOnly(kvm) = nil, want error")
	}
}

func TestFCIgnoredLaunchSettings(t *testing.T) {
	if got := fcIgnoredLaunchSettings(NewVMConfig(), ""); len(got) != 0 {
		t.Errorf("fcIgnoredLaunchSettings(defaults) = %q, want none", got)
	}

	cfg := NewVMConfig()
	cfg.ContainerConfig.FilesystemPath = "/rootfs"
	cfg.ContainerConfig.Init = []string{"/sbin/init"}
	cfg.ContainerConfig.VolumePaths = map[string]string{"/a": "/b"}
	cfg.AndroidConfig.AVDName = "pixel"

	want := []string{
		"firecracker VMs ignore optimize hugepages /mnt/huge: huge pages for Firecracker VMs are not supported",
		"firecracker VMs ignore vm config filesystem",
		"firecracker VMs ignore vm config init",
		"firecracker VMs ignore vm config volume",
		"firecracker VMs ignore the vm config android settings",
	}
	if got := fcIgnoredLaunchSettings(cfg, "/mnt/huge"); !reflect.DeepEqual(got, want) {
		t.Errorf("fcIgnoredLaunchSettings() =\n%q\nwant\n%q", got, want)
	}
}

func TestVMSerialFirecracker(t *testing.T) {
	vm := newTestFCVM(t, cmdTestFCConfig())
	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{vm.ID: vm}

	if err := os.MkdirAll(vm.instancePath, 0700); err != nil {
		t.Fatal(err)
	}
	log := bytes.Repeat([]byte("a"), 70000)
	log = append(log, []byte("login: ")...)
	if err := os.WriteFile(filepath.Join(vm.instancePath, "console.log"), log, 0600); err != nil {
		t.Fatal(err)
	}

	resp := &minicli.Response{}
	if err := cliVMSerial(ns, cmdArgs(map[string]string{"vm": "fc1"}), resp); err != nil {
		t.Fatalf("cliVMSerial() = %v", err)
	}
	if want := string(log[len(log)-65536:]); resp.Response != want {
		t.Errorf("cliVMSerial() returned %d bytes, want the last 65536 bytes of the console log", len(resp.Response))
	}

	resp = &minicli.Response{}
	args := map[string]string{"vm": "fc1", "port": "0", "milliseconds": "100", "bytes": "7"}
	if err := cliVMSerial(ns, cmdArgs(args), resp); err != nil || resp.Response != "login: " {
		t.Errorf("cliVMSerial(7 bytes) = %q, %v, want %q", resp.Response, err, "login: ")
	}

	for _, tc := range []struct {
		args map[string]string
		want string
	}{
		{map[string]string{"vm": "fc1", "port": "1"}, "firecracker VMs have one serial port: 0"},
		{map[string]string{"vm": "fc1", "port": "0", "milliseconds": "750", "bytes": "0"}, "serial output limit must be between 1 and 1048576 bytes"},
		{map[string]string{"vm": "fc1", "port": "x"}, `invalid serial port: "x"`},
	} {
		if err := cliVMSerial(ns, cmdArgs(tc.args), &minicli.Response{}); err == nil || err.Error() != tc.want {
			t.Errorf("cliVMSerial(%v) = %v, want %v", tc.args, err, tc.want)
		}
	}
}

func TestFCUnsupportedCommands(t *testing.T) {
	vm := newTestFCVM(t, cmdTestFCConfig())
	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{vm.ID: vm}

	file := filepath.Join(t.TempDir(), "media.iso")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		fn   func(*Namespace, *minicli.Command, *minicli.Response) error
		cmd  *minicli.Command
		want string
	}{
		{"vm cdrom eject", cliVMCdrom, cmdArgs(map[string]string{"vm": "fc1"}, "eject"), "vm cdrom is not supported for firecracker VMs"},
		{"vm cdrom change", cliVMCdrom, cmdArgs(map[string]string{"vm": "fc1", "path": file}, "change"), "vm cdrom is not supported for firecracker VMs"},
		{"vm hotplug add", cliVMHotplug, cmdArgs(map[string]string{"vm": "fc1", "filename": file}, "add"), "vm hotplug is not supported for firecracker VMs"},
		{"vm hotplug remove", cliVMHotplug, cmdArgs(map[string]string{"vm": "fc1", "disk": "all"}, "remove"), "vm hotplug is not supported for firecracker VMs"},
		{"vm net add", cliVMNetMod, &minicli.Command{
			StringArgs: map[string]string{"vm": "fc1"},
			BoolArgs:   map[string]bool{"add": true},
			ListArgs:   map[string][]string{"netspec": {"100"}},
		}, "adding NICs to a launched firecracker VM is not supported"},
		{"vm net bond", cliVMNetMod, cmdArgs(map[string]string{"vm": "fc1", "interface": "0,1"}, "bond", "active-backup", "active"), "firecracker VMs do not support bonds"},
		{"vnc record", cliVNCRecord, cmdArgs(map[string]string{"vm": "fc1", "filename": "/tmp/x.kb"}, "record", "kb"), "vnc is not supported for firecracker VMs"},
		{"vnc play", cliVNCPlay, cmdArgs(map[string]string{"vm": "fc1", "filename": "/tmp/x.kb"}, "play"), "vnc is not supported for firecracker VMs"},
	} {
		if err := tc.fn(ns, tc.cmd, &minicli.Response{}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v = %v, want %v", tc.name, err, tc.want)
		}
	}

	// A wildcard target skips Firecracker VMs.
	for _, tc := range []struct {
		name string
		fn   func(*Namespace, *minicli.Command, *minicli.Response) error
		cmd  *minicli.Command
	}{
		{"vm cdrom eject all", cliVMCdrom, cmdArgs(map[string]string{"vm": Wildcard}, "eject")},
		{"vm hotplug remove all", cliVMHotplug, cmdArgs(map[string]string{"vm": Wildcard, "disk": "all"}, "remove")},
		{"vnc play all", cliVNCPlay, cmdArgs(map[string]string{"vm": Wildcard, "filename": "/tmp/x.kb"}, "play")},
		{"vm net add all", cliVMNetMod, &minicli.Command{
			StringArgs: map[string]string{"vm": Wildcard},
			BoolArgs:   map[string]bool{"add": true},
			ListArgs:   map[string][]string{"netspec": {"100"}},
		}},
		{"vm net bond all", cliVMNetMod, cmdArgs(map[string]string{"vm": Wildcard, "interface": "0,1"}, "bond", "active-backup", "active")},
	} {
		if err := tc.fn(ns, tc.cmd, &minicli.Response{}); err != nil {
			t.Errorf("%v = %v, want nil", tc.name, err)
		}
	}
}

func TestVMSaveUnsupported(t *testing.T) {
	vm := &ContainerVM{BaseVM: &BaseVM{ID: 9, Name: "c1", Type: CONTAINER, Namespace: DefaultNamespace}}
	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{vm.ID: vm}

	cmd := &minicli.Command{StringArgs: map[string]string{"vm": "c1"}, BoolArgs: map[string]bool{}}
	want := `vm save is not supported for "c1" (type container)`
	if err := cliVMSave(ns, cmd, &minicli.Response{}); err == nil || err.Error() != want {
		t.Errorf("cliVMSave() = %v, want %v", err, want)
	}
}

func TestSetAffinityArgs(t *testing.T) {
	want := []string{"taskset", "-a", "-p", "3", "42"}
	if got := setAffinityArgs("3", 42); !reflect.DeepEqual(got, want) {
		t.Errorf("setAffinityArgs() = %q, want %q", got, want)
	}
}

func TestFCRecoveredID(t *testing.T) {
	fcDir := fcResolve(t.TempDir())
	setFCJailGlobals(t, fcDir)

	base := fcResolve(t.TempDir())
	inst := filepath.Join(fcDir, "vm", "3")
	if err := os.MkdirAll(inst, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(inst, filepath.Join(base, "3")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "4"), 0700); err != nil {
		t.Fatal(err)
	}

	rel, err := filepath.Rel(base, inst)
	if err != nil {
		t.Fatal(err)
	}

	for _, vmid := range []string{"3", rel} {
		if id, ok := fcRecoveredID(base, vmid); !ok || id != "3" {
			t.Errorf("fcRecoveredID(%q) = %q, %v, want 3, true", vmid, id, ok)
		}
	}
	if id, ok := fcRecoveredID(base, "4"); ok {
		t.Errorf("fcRecoveredID(KVM dir) = %q, true, want false", id)
	}
}
