// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestMemPathArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"none", []string{"-m", "512", "-smp", "1"}, ""},
		{"empty", nil, ""},
		{"one", []string{"-m", "512", "-mem-path", "/dev/hugepages", "-uuid", "x"}, "/dev/hugepages"},
		{"two", []string{"-mem-path", "/dev/hugepages", "-mem-path", "/mnt/huge1G"}, "/mnt/huge1G"},
		{"last without value", []string{"-m", "512", "-mem-path"}, ""},
		{"value then flag without value", []string{"-mem-path", "/dev/hugepages", "-mem-path"}, "/dev/hugepages"},
		{"two dashes", []string{"-m", "512", "--mem-path", "/dev/hugepages"}, "/dev/hugepages"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := memPathArg(test.args); got != test.want {
				t.Errorf("memPathArg(%q) = %q, want %q", test.args, got, test.want)
			}
		})
	}
}

func TestMemArgMB(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		wantMB uint64
		wantOK bool
	}{
		{"one", []string{"-m", "512"}, 512, true},
		{"last wins", []string{"-m", "512", "-m", "1024"}, 1024, true},
		{"G suffix", []string{"-m", "1G"}, 1024, true},
		{"two dashes", []string{"--m", "2G"}, 2048, true},
		{"size first", []string{"-m", "size=1G,slots=2,maxmem=4G"}, 1024, true},
		{"size last", []string{"-m", "slots=2,maxmem=4G,size=1024"}, 1024, true},
		{"later without size", []string{"-m", "512", "-m", "maxmem=4G"}, 512, true},
		{"two sizes in one value", []string{"-m", "size=1G,size=2G"}, 2048, true},
		{"size after an implied size", []string{"-m", "512,size=1G"}, 1024, true},
		{"bad size before a size", []string{"-m", "bogus,size=1G"}, 0, false},
		{"fraction", []string{"-m", "1.5G"}, 0, false},
		{"bogus", []string{"-m", "bogus"}, 0, false},
		{"last without value", []string{"-m"}, 0, false},
		{"empty", nil, 0, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mb, ok := memArgMB(test.args)
			if mb != test.wantMB || ok != test.wantOK {
				t.Errorf("memArgMB(%q) = %v, %v, want %v, %v", test.args, mb, ok, test.wantMB, test.wantOK)
			}
		})
	}
}

func TestCheckMemAlignment(t *testing.T) {
	const (
		page2M = 2 * mebibyte
		page1G = gibibyte
	)

	tests := []struct {
		memMB    uint64
		pageSize int64
		wantErr  string
	}{
		{512, page2M, ""},
		{513, page2M, "vm memory 513 MB is not a multiple of the 2 MiB hugepage size of /dev/hugepages"},
		{1024, page1G, ""},
		{2048, page1G, ""},
		{512, page1G, "vm memory 512 MB is not a multiple of the 1 GiB hugepage size of /dev/hugepages"},
		{1536, page1G, "vm memory 1536 MB is not a multiple of the 1 GiB hugepage size of /dev/hugepages"},
		{513, 0, ""},
		{1, 0, ""},
	}

	for _, test := range tests {
		err := checkMemAlignment(test.memMB, test.pageSize, "/dev/hugepages")

		switch {
		case test.wantErr == "" && err != nil:
			t.Errorf("checkMemAlignment(%v, %v) = %v, want nil", test.memMB, test.pageSize, err)
		case test.wantErr != "" && (err == nil || err.Error() != test.wantErr):
			t.Errorf("checkMemAlignment(%v, %v) = %v, want %q", test.memMB, test.pageSize, err, test.wantErr)
		}
	}
}

func TestHugepageSizeNotHugetlbfs(t *testing.T) {
	size, err := hugepageSize(t.TempDir())
	if err != nil {
		t.Fatalf("hugepageSize: %v", err)
	}

	if size != 0 {
		t.Errorf("hugepageSize of a temporary directory = %v, want 0", size)
	}
}

func TestHugepageSizeMissingPath(t *testing.T) {
	if _, err := hugepageSize(filepath.Join(t.TempDir(), "nodir", "missing")); err == nil {
		t.Error("hugepageSize of a missing path returned no error")
	}
}

// TestHugepageSizeMissingFile checks that hugepageSize uses the directory of a
// missing file, because QEMU creates a missing -mem-path file.
func TestHugepageSizeMissingFile(t *testing.T) {
	size, err := hugepageSize(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("hugepageSize: %v", err)
	}

	if size != 0 {
		t.Errorf("hugepageSize of a missing file in a temporary directory = %v, want 0", size)
	}
}

func TestHugepagesQemuArgs(t *testing.T) {
	c := NewVMConfig()
	c.Memory = 512
	c.UUID = "11111111-2222-3333-4444-555555555555"

	if args := c.qemuArgs(7, "/tmp/minimega/7"); memPathArg(args) != "" {
		t.Errorf("qemu arguments without hugepages contain -mem-path: %q", args)
	}

	c.hugepagesMountPath = "/dev/hugepages"

	args := c.qemuArgs(7, "/tmp/minimega/7")
	if !hasArgSequence(args, "-mem-path", "/dev/hugepages") {
		t.Errorf("missing -mem-path /dev/hugepages in %q", args)
	}
}

// TestHugepagesLaunchVMs checks that the hugepages mount path of the namespace
// reaches the configuration of the KVM VM that the namespace launches.
func TestHugepagesLaunchVMs(t *testing.T) {
	ns := &Namespace{Name: "test"}
	ns.setHugepagesMountPath("/dev/hugepages")

	q := &QueuedVMs{Names: []string{"huge"}, VMType: KVM, VMConfig: NewVMConfig()}

	got := ns.launchVMs(q)
	if q.hugepagesMountPath != "" {
		t.Errorf("launchVMs changed the queued VMs: %q", q.hugepagesMountPath)
	}

	vm, err := NewKVM("huge", ns.Name, got.VMConfig)
	if err != nil {
		t.Fatal(err)
	}

	if vm.hugepagesMountPath != "/dev/hugepages" {
		t.Errorf("launched VM hugepages mount path = %q, want /dev/hugepages", vm.hugepagesMountPath)
	}

	args := VMConfig{BaseConfig: vm.BaseConfig, KVMConfig: vm.KVMConfig}.qemuArgs(vm.ID, vm.instancePath)
	if !hasArgSequence(args, "-mem-path", "/dev/hugepages") {
		t.Errorf("missing -mem-path /dev/hugepages in %q", args)
	}

	ns.setHugepagesMountPath("")
	if got := ns.launchVMs(q); got.hugepagesMountPath != "" {
		t.Errorf("hugepages mount path after clear = %q, want none", got.hugepagesMountPath)
	}
}

// fakeHugepageSize makes KvmVM.launch use size as the hugepage size of each
// path. It returns the paths that launch checks.
func fakeHugepageSize(t *testing.T, size int64) *[]string {
	orig := hugepageSizeFn
	t.Cleanup(func() { hugepageSizeFn = orig })

	var paths []string
	hugepageSizeFn = func(p string) (int64, error) {
		paths = append(paths, p)
		return size, nil
	}

	return &paths
}

// launchHugepagesVM launches a KVM VM with 512 MB of memory through
// launchHugepagesVMMemory.
func launchHugepagesVM(t *testing.T, path string, appendArgs ...string) (*KvmVM, error) {
	t.Helper()

	return launchHugepagesVMMemory(t, path, 512, appendArgs...)
}

// launchHugepagesVMMemory launches a KVM VM through Namespace.Launch with the
// hugepages mount path set to path, memory MB of memory, and appendArgs as the
// QEMU append arguments. The QEMU path does not exist, so the launch fails
// before QEMU starts. It returns the VM and the launch error.
func launchHugepagesVMMemory(t *testing.T, path string, memory uint64, appendArgs ...string) (*KvmVM, error) {
	t.Helper()

	originalBase := *f_base
	t.Cleanup(func() { *f_base = originalBase })
	*f_base = t.TempDir()

	ns := &Namespace{Name: "test", VMs: VMs{m: map[int]VM{}}}
	ns.setHugepagesMountPath(path)

	config := NewVMConfig()
	config.QemuPath = filepath.Join(*f_base, "no-qemu")
	config.Memory = memory
	config.QemuAppend = appendArgs

	errs := ns.Launch("", &QueuedVMs{Names: []string{"huge"}, VMType: KVM, VMConfig: config})
	if len(errs) != 1 {
		t.Fatalf("launch errors = %v, want one", errs)
	}

	vm, err := ns.FindKvmVM("huge")
	if err != nil {
		t.Fatal(err)
	}

	return vm, errs[0]
}

// TestHugepagesNamespaceLaunch checks that Namespace.Launch gives the hugepages
// mount path to the KVM VM and that the launch check accepts a path that is
// not on hugetlbfs.
func TestHugepagesNamespaceLaunch(t *testing.T) {
	path := t.TempDir()

	vm, err := launchHugepagesVM(t, path)
	if vm.hugepagesMountPath != path {
		t.Errorf("launched VM hugepages mount path = %q, want %q", vm.hugepagesMountPath, path)
	}

	if !strings.Contains(err.Error(), "unable to start qemu") {
		t.Errorf("launch error = %v, want the error from the start of QEMU", err)
	}
}

// TestHugepagesNamespaceLaunchMissingPath checks that the launch fails when the
// hugepages mount path does not exist.
func TestHugepagesNamespaceLaunchMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nodir", "missing")

	vm, err := launchHugepagesVM(t, path)
	if !strings.Contains(err.Error(), "unable to stat filesystem of "+path) {
		t.Errorf("launch error = %v, want an error about %v", err, path)
	}

	if vm.GetState() != VM_ERROR || !strings.Contains(vm.Tag("error"), path) {
		t.Errorf("VM state = %v, error tag = %q; want ERROR with the path", vm.GetState(), vm.Tag("error"))
	}
}

// TestHugepagesLaunchRejectsUnalignedMemory checks that the launch fails and
// puts the VM in the ERROR state when the memory does not fit the hugepage size.
func TestHugepagesLaunchRejectsUnalignedMemory(t *testing.T) {
	path := t.TempDir()
	fakeHugepageSize(t, gibibyte)

	vm, err := launchHugepagesVM(t, path)

	want := "vm memory 512 MB is not a multiple of the 1 GiB hugepage size of " + path
	if !strings.Contains(err.Error(), want) {
		t.Errorf("launch error = %v, want %q", err, want)
	}

	if vm.GetState() != VM_ERROR || !strings.Contains(vm.Tag("error"), want) {
		t.Errorf("VM state = %v, error tag = %q; want ERROR with %q", vm.GetState(), vm.Tag("error"), want)
	}
}

// TestHugepagesLaunchChecksQemuAppendPath checks that the launch checks a
// -mem-path from the QEMU append arguments when no hugepages path is set.
func TestHugepagesLaunchChecksQemuAppendPath(t *testing.T) {
	p := t.TempDir()
	paths := fakeHugepageSize(t, gibibyte)

	_, err := launchHugepagesVM(t, "", "-mem-path", p)

	if len(*paths) != 1 || (*paths)[0] != p {
		t.Errorf("checked paths = %q, want [%q]", *paths, p)
	}

	want := "not a multiple of the 1 GiB hugepage size of " + p
	if !strings.Contains(err.Error(), want) {
		t.Errorf("launch error = %v, want %q", err, want)
	}
}

// TestHugepagesLaunchUsesQemuAppendMemory checks that the memory check uses the
// last -m in the QEMU arguments and not the configured VM memory.
func TestHugepagesLaunchUsesQemuAppendMemory(t *testing.T) {
	fakeHugepageSize(t, gibibyte)

	_, err := launchHugepagesVMMemory(t, t.TempDir(), 1000, "-m", "1024")
	if strings.Contains(err.Error(), "is not a multiple of") {
		t.Errorf("launch error = %v, want no memory alignment error for -m 1024", err)
	}
}

// TestHugepagesLaunchVMsOnlyKVM checks that VMs of other types do not get the
// hugepages mount path.
func TestHugepagesLaunchVMsOnlyKVM(t *testing.T) {
	ns := &Namespace{Name: "test"}
	ns.setHugepagesMountPath("/dev/hugepages")

	for _, vmType := range []VMType{ANDROID, CONTAINER} {
		q := &QueuedVMs{Names: []string{"other"}, VMType: vmType, VMConfig: NewVMConfig()}
		// a path in the queued config must not reach the VM
		q.hugepagesMountPath = "/stale"

		if got := ns.launchVMs(q); got.hugepagesMountPath != "" {
			t.Errorf("%v VM hugepages mount path = %q, want none", vmType, got.hugepagesMountPath)
		}
	}
}

func TestFormatPageSize(t *testing.T) {
	for size, want := range map[int64]string{
		64 * 1024:     "64 KiB",
		2 * mebibyte:  "2 MiB",
		32 * mebibyte: "32 MiB",
		gibibyte:      "1 GiB",
		16 * gibibyte: "16 GiB",
		1000:          "1000 bytes",
	} {
		if got := formatPageSize(size); got != want {
			t.Errorf("formatPageSize(%v) = %q, want %q", size, got, want)
		}
	}

	if !strings.Contains(checkMemAlignment(1, 64*1024*3, "/x").Error(), "192 KiB") {
		t.Error("checkMemAlignment does not format a KiB page size")
	}
}

// TestHugepagesCloneClearsMountPath checks that "vm config clone" does not copy
// the hugepages mount path of a launched VM to the VM configuration.
func TestHugepagesCloneClearsMountPath(t *testing.T) {
	vm := testVM(KVM, VM_RUNNING)
	vm.(*KvmVM).ID = 1
	vm.(*KvmVM).Name = "vm1"
	vm.(*KvmVM).KVMConfig = NewVMConfig().KVMConfig
	vm.(*KvmVM).hugepagesMountPath = "/dev/hugepages"

	ns := &Namespace{
		Name:     "test",
		Hosts:    map[string]bool{hostname: true},
		vmConfig: NewVMConfig(),
	}
	ns.VMs.m = map[int]VM{1: vm}
	setupClearVLANsBlacklist(t, ns)

	c := minicli.MustCompile("vm config clone vm1")
	if err := cliVMConfig(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if got := ns.vmConfig.qemuOverrideString(); strings.Contains(got, "-mem-path") {
		t.Errorf("qemu-override preview after clone contains -mem-path: %q", got)
	}
}
