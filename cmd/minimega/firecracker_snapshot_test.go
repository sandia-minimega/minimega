// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

var fcTestHost = fcSnapshotHost{
	Arch:            "x86_64",
	CPUModel:        "Test CPU @ 3.00GHz",
	Kernel:          "6.8.0-test",
	Version:         fcVersion{1, 17, 0},
	SnapshotVersion: fcVersion{12, 0, 0},
}

// setFCSnapshotSeams gives the test its own save registry and host facts, and
// restores the snapshot seams.
func setFCSnapshotSeams(t *testing.T) {
	t.Helper()

	setFCOpsSeams(t)
	setMemDumpSeams(t)

	oldSaves, oldSnapshot, oldHost, oldRename := fcSaves, fcSaveSnapshot, fcSnapshotHostFn, fcSaveRename
	oldGroups, oldPrefixes, oldPoll := fcSnapGroups, fcPrefixes, fcSavePoll
	t.Cleanup(func() {
		fcSaves, fcSaveSnapshot, fcSnapshotHostFn, fcSaveRename = oldSaves, oldSnapshot, oldHost, oldRename
		fcSnapGroups, fcPrefixes, fcSavePoll = oldGroups, oldPrefixes, oldPoll
	})

	fcSaves = &fcExportRegistry{m: map[int]*fcExport{}}
	fcPrefixes = &fcPrefixTable{saving: map[string]bool{}, locks: map[string]*fcPrefixLock{}}
	fcSavePoll = time.Millisecond
	fcSnapGroups = newFCSnapGroups()
	fcSnapshotHostFn = func(string) fcSnapshotHost { return fcTestHost }
	fcReflinkOK = func() bool { return false }
}

// fcFakeSaveSnapshot returns a fcSaveSnapshot that sends the request to the
// fake API server, then writes the state and memory files in the jail.
func fcFakeSaveSnapshot(mem []byte) func(context.Context, *fcClient, string, string) error {
	return func(ctx context.Context, c *fcClient, statePath, memPath string) error {
		if err := c.CreateSnapshotSync(ctx, statePath, memPath); err != nil {
			return err
		}

		root := filepath.Dir(c.socket)
		if err := os.WriteFile(filepath.Join(root, statePath), []byte("state"), 0644); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, memPath), mem, 0644)
	}
}

func waitFCSave(t *testing.T, id int, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		fcSaves.mu.Lock()
		e := fcSaves.m[id]
		fcSaves.mu.Unlock()

		var got string
		if e != nil {
			got, _ = e.get()
		}
		if got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("vm save status = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// fcTestSnapshotInfo returns the sidecar of a snapshot that matches cfg and
// fcTestHost.
func fcTestSnapshotInfo(cfg VMConfig) fcSnapshotInfo {
	info := fcSnapshotInfo{
		VMType:             fcSnapshotType,
		FirecrackerVersion: "v1.17.0",
		SnapshotVersion:    "v12.0.0",
		Arch:               fcTestHost.Arch,
		HostCPUModel:       fcTestHost.CPUModel,
		HostKernel:         fcTestHost.Kernel,
		VCPUs:              cfg.VCPUs,
		MemoryMiB:          cfg.Memory,
		Devices:            fcSnapshotDevicesOf(cfg),
		Drives:             fcSnapshotDrives(cfg),
	}

	for _, nic := range cfg.Networks {
		info.NICMACs = append(info.NICMACs, nic.MAC)
	}

	return info
}

func TestFCCreateSnapshotSync(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	s := newFCTestServer(t, fcListen(t, socket))
	c := newFCClient(socket)

	if err := c.CreateSnapshotSync(context.Background(), "/out/a.state", "/out/a.mem"); err != nil {
		t.Fatalf("CreateSnapshotSync() = %v", err)
	}

	r := s.last(t)
	want := map[string]any{
		"snapshot_type":       "Full",
		"snapshot_path":       "/out/a.state",
		"mem_file_path":       "/out/a.mem",
		"sync_snapshot_files": true,
	}
	if r.Method != "PUT" || r.Path != "/snapshot/create" || !reflect.DeepEqual(r.Body, want) {
		t.Errorf("request = %v %v %v, want PUT /snapshot/create %v", r.Method, r.Path, r.Body, want)
	}
}

func TestFCSave(t *testing.T) {
	f := setupFCDriver(t)
	setFCSnapshotSeams(t)

	mem := bytes.Repeat([]byte("m"), 8192)
	fcSaveSnapshot = fcFakeSaveSnapshot(mem)

	var mu sync.Mutex
	var renames [][2]string
	fcSaveRename = func(src, dst string) error {
		mu.Lock()
		renames = append(renames, [2]string{filepath.Base(src), filepath.Base(dst)})
		mu.Unlock()
		return os.Rename(src, dst)
	}

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	cleanupFCVM(t, vm)

	dir := filepath.Join(f.dir, "saved")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "fc1snap")

	if err := vm.Save(dst); err == nil || !strings.Contains(err.Error(), "needs a vm in running or paused") {
		t.Errorf("Save() in BUILDING = %v, want state error", err)
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	disk := filepath.Join(fcInstanceDir(vm.ID), "disk-0.raw")
	if err := os.WriteFile(disk, append([]byte("disk data"), make([]byte, 4087)...), 0600); err != nil {
		t.Fatal(err)
	}

	// The sidecar of an earlier save goes away before the new files arrive.
	fcWriteFile(t, dst+".json", []byte(`{"vm_type": "old"}`))

	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{vm.ID: vm}

	before := len(f.requests(1))
	cmd := &minicli.Command{StringArgs: map[string]string{"vm": "fc1", "filename": dst}, BoolArgs: map[string]bool{}}
	if err := cliVMSave(ns, cmd, &minicli.Response{}); err != nil {
		t.Fatalf("cliVMSave() = %v", err)
	}
	if got := vm.GetState(); got != VM_PAUSED {
		t.Errorf("state after vm save = %v, want PAUSED", got)
	}
	waitFCSave(t, vm.ID, fcExportCompleted)

	if got, want := f.requests(1)[before:], []string{"PATCH /vm", "PUT /snapshot/create"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %q, want %q", got, want)
	}
	if got := f.reqAt(1, before).Body["state"]; got != "Paused" {
		t.Errorf("first request state = %v, want Paused", got)
	}
	wantBody := map[string]any{
		"snapshot_type":       "Full",
		"snapshot_path":       "/out/save.state",
		"mem_file_path":       "/out/save.mem",
		"sync_snapshot_files": true,
	}
	if got := f.reqAt(1, before+1).Body; !reflect.DeepEqual(got, wantBody) {
		t.Errorf("snapshot request = %v, want %v", got, wantBody)
	}

	diskData := append([]byte("disk data"), make([]byte, 4087)...)
	for ext, want := range map[string][]byte{".state": []byte("state"), ".mem": mem, ".hdd": diskData} {
		data, err := os.ReadFile(dst + ext)
		if err != nil || !bytes.Equal(data, want) {
			t.Errorf("%v = %d bytes, %v, want %d bytes", dst+ext, len(data), err, len(want))
		}
	}
	for _, ext := range []string{".state", ".mem", ".hdd", ".json"} {
		if fi, err := os.Stat(dst + ext); err != nil || fi.Mode().Perm() != 0600 {
			t.Errorf("%v: %v, %v, want mode 0600", dst+ext, fi, err)
		}
	}

	data, err := os.ReadFile(dst + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"vm_type": "firecracker"`)) {
		t.Errorf("sidecar does not have \"vm_type\": \"firecracker\":\n%s", data)
	}

	var info fcSnapshotInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	if info.Created.IsZero() {
		t.Errorf("sidecar has no created time")
	}

	vm.lock.Lock()
	vcpus, memory, mac := vm.VCPUs, vm.Memory, vm.Networks[0].MAC
	vm.lock.Unlock()

	regions, err := fcMemoryRegions(memory)
	if err != nil {
		t.Fatal(err)
	}
	want := fcSnapshotInfo{
		VMType:             "firecracker",
		FirecrackerVersion: "v1.17.0",
		SnapshotVersion:    "v12.0.0",
		Arch:               "x86_64",
		HostCPUModel:       fcTestHost.CPUModel,
		HostKernel:         fcTestHost.Kernel,
		VCPUs:              vcpus,
		MemoryMiB:          memory,
		Devices:            fcSnapshotDevices{Disks: 1, NICs: 1, Vsock: 1, Entropy: 1},
		NICMACs:            []string{mac},
		Drives:             []fcSnapshotDrive{{ID: "disk0", JailPath: "/disk0", File: "fc1snap.hdd"}},
		MemoryRegions:      regions,
		Created:            info.Created,
	}
	if !reflect.DeepEqual(info, want) {
		t.Errorf("sidecar =\n%+v\nwant\n%+v", info, want)
	}

	// Each file arrives through a temporary name, and the sidecar arrives last.
	mu.Lock()
	gotRenames := append([][2]string(nil), renames...)
	mu.Unlock()
	var dsts []string
	for _, r := range gotRenames {
		if !strings.HasPrefix(r[0], r[1]+".tmp-") {
			t.Errorf("rename %v to %v is not from a temporary name", r[0], r[1])
		}
		dsts = append(dsts, r[1])
	}
	if want := []string{"fc1snap.state", "fc1snap.mem", "fc1snap.hdd", "fc1snap.json"}; !reflect.DeepEqual(dsts, want) {
		t.Errorf("renames = %q, want %q", dsts, want)
	}

	if m, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*")); len(m) != 0 {
		t.Errorf("temporary files are left: %q", m)
	}
	for _, pat := range []string{"save-*", "export-*"} {
		if m, _ := filepath.Glob(filepath.Join(fcInstanceDir(vm.ID), pat)); len(m) != 0 {
			t.Errorf("save files in the instance directory: %q", m)
		}
	}
	vm.lock.Lock()
	out := vm.jail.HostPath("/out")
	vm.lock.Unlock()
	if ents, err := os.ReadDir(out); err != nil || len(ents) != 0 {
		t.Errorf("jail /out = %v, %v, want empty", ents, err)
	}

	resp := &minicli.Response{}
	if err := cliVMSave(ns, &minicli.Command{StringArgs: map[string]string{}, BoolArgs: map[string]bool{}}, resp); err != nil {
		t.Fatalf("cliVMSave() with no arguments = %v", err)
	}
	wantRows := [][]string{{strconv.Itoa(vm.ID), "fc1", "completed", "100.00"}}
	if !reflect.DeepEqual(resp.Header, fcExportHeader) || !reflect.DeepEqual(resp.Tabular, wantRows) {
		t.Errorf("vm save table = %q %q, want %q %q", resp.Header, resp.Tabular, fcExportHeader, wantRows)
	}

	// A save of a paused VM sends only the snapshot, and the VM stays paused.
	before = len(f.requests(1))
	if err := vm.Save(dst); err != nil {
		t.Fatalf("Save() of a paused VM = %v", err)
	}
	waitFCSave(t, vm.ID, fcExportCompleted)
	if got, want := f.requests(1)[before:], []string{"PUT /snapshot/create"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paused requests = %q, want %q", got, want)
	}
	if got := vm.GetState(); got != VM_PAUSED {
		t.Errorf("state = %v, want PAUSED", got)
	}

	// The renames wait for a restore that reads the prefix, and another save
	// to the prefix is refused while the save runs.
	unlock := fcPrefixes.lock(dst, false)
	if err := vm.Save(dst); err != nil {
		t.Fatalf("Save() while a restore reads the prefix = %v", err)
	}
	if err := fcPrefixes.beginSave(dst); err == nil || !strings.Contains(err.Error(), "another save to "+dst+" is running") {
		t.Errorf("beginSave() during a save = %v, want a refusal", err)
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := os.Stat(dst + ".json"); err != nil {
		t.Errorf("sidecar while the restore reads the prefix: %v, want the old sidecar", err)
	}
	fcSaves.mu.Lock()
	job := fcSaves.m[vm.ID]
	fcSaves.mu.Unlock()
	if status, _ := job.get(); status != fcExportActive {
		t.Errorf("save status while the restore reads the prefix = %v, want active", status)
	}
	unlock()
	if err := fcWaitSave(vm.ID); err != nil {
		t.Errorf("fcWaitSave() = %v", err)
	}
	if err := fcPrefixes.beginSave(dst); err != nil {
		t.Errorf("beginSave() after the save = %v", err)
	}
	fcPrefixes.endSave(dst)

	// vm start resumes the VM, as after vm save of a KVM VM.
	before = len(f.requests(1))
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() after Save() = %v", err)
	}
	if got := f.requests(1)[before:]; !reflect.DeepEqual(got, []string{"PATCH /vm"}) || f.reqAt(1, before).Body["state"] != "Resumed" {
		t.Errorf("start requests = %q, want PATCH /vm Resumed", got)
	}
	waitFCState(t, vm, VM_RUNNING)

	// A failed snapshot resumes the VM and keeps the previous row.
	fcSaveSnapshot = func(context.Context, *fcClient, string, string) error {
		return errors.New("disk full")
	}

	before = len(f.requests(1))
	fail := filepath.Join(dir, "fail")
	if err := vm.Save(fail); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("Save() with a failed snapshot = %v", err)
	}
	if got, want := f.requests(1)[before:], []string{"PATCH /vm", "PATCH /vm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("failed snapshot requests = %q, want %q", got, want)
	}
	if got := f.reqAt(1, before+1).Body["state"]; got != "Resumed" {
		t.Errorf("last request state = %v, want Resumed", got)
	}
	if got := vm.GetState(); got != VM_RUNNING {
		t.Errorf("state = %v, want RUNNING", got)
	}
	if m, _ := filepath.Glob(fail + "*"); len(m) != 0 {
		t.Errorf("files of a failed save: %q", m)
	}
	if got := fcSaves.table(DefaultNamespace); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("table after a failed save = %q, want %q", got, wantRows)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	if err := vm.Save(dst); err == nil || !strings.Contains(err.Error(), "needs a vm in running or paused") {
		t.Errorf("Save() in QUIT = %v, want state error", err)
	}
}

func TestFCWaitSave(t *testing.T) {
	setFCSnapshotSeams(t)

	if err := fcWaitSave(3); err == nil || !strings.Contains(err.Error(), "has no vm save") {
		t.Errorf("fcWaitSave() with no save = %v", err)
	}

	job := &fcExport{ID: 3, Name: "a", Dst: "/s/a", status: fcExportActive}
	fcSaves.m[3] = job
	go func() {
		time.Sleep(20 * time.Millisecond)
		job.finish(fcExportFailed)
	}()
	if err := fcWaitSave(3); err == nil || !strings.Contains(err.Error(), "vm save of vm a to /s/a failed") {
		t.Errorf("fcWaitSave() of a failed save = %v", err)
	}

	fcSaves.m[3] = &fcExport{ID: 3, status: fcExportCompleted}
	if err := fcWaitSave(3); err != nil {
		t.Errorf("fcWaitSave() of a completed save = %v", err)
	}
}

func TestFCPrefixLock(t *testing.T) {
	setFCSnapshotSeams(t)

	dir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}

	// Two paths to one prefix get one lock.
	unlock := fcPrefixes.lock(filepath.Join(dir, "link", "s"), false)
	done := make(chan struct{})
	go func() {
		fcPrefixes.lock(filepath.Join(dir, "s"), true)()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("the write lock did not wait for the read lock")
	case <-time.After(50 * time.Millisecond):
	}

	unlock()
	unlock()
	<-done

	fcPrefixes.mu.Lock()
	n := len(fcPrefixes.locks)
	fcPrefixes.mu.Unlock()
	if n != 0 {
		t.Errorf("locks after the unlocks = %d, want 0", n)
	}
}

func TestFCSaveTable(t *testing.T) {
	setFCSnapshotSeams(t)

	fcSaves.m[9] = &fcExport{ID: 9, Name: "b", Namespace: DefaultNamespace, status: fcExportActive, complete: 50}
	fcSaves.m[3] = &fcExport{ID: 3, Name: "a", Namespace: DefaultNamespace, status: fcExportCompleted, complete: 100}
	fcSaves.m[4] = &fcExport{ID: 4, Name: "c", Namespace: "other", status: fcExportActive}

	ns := &Namespace{Name: DefaultNamespace}
	resp := &minicli.Response{}
	if err := cliVMSave(ns, &minicli.Command{StringArgs: map[string]string{}, BoolArgs: map[string]bool{}}, resp); err != nil {
		t.Fatalf("cliVMSave() = %v", err)
	}

	want := [][]string{{"3", "a", "completed", "100.00"}, {"9", "b", "active", "50.00"}}
	if !reflect.DeepEqual(resp.Header, fcExportHeader) || !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("vm save table = %q %q, want %q", resp.Header, resp.Tabular, want)
	}
}

func TestFCCheckSnapshot(t *testing.T) {
	base := func() VMConfig {
		cfg := NewVMConfig()
		cfg.KernelPath = "/k/vmlinux"
		cfg.VCPUs = 2
		cfg.Memory = 512
		cfg.Disks = DiskConfigs{{Path: "/s/fc1.hdd"}, {Path: "/i/shared.squashfs", ReadOnly: true}}
		cfg.Networks = NetConfigs{{VLAN: 101, MAC: "06:00:00:00:00:01"}}
		cfg.Backchannel = true
		cfg.FirecrackerEntropy = true
		cfg.FirecrackerScratch = "16M"
		return cfg
	}

	info := fcTestSnapshotInfo(base())
	info.Drives[1].Source = "/i/shared.squashfs"

	if w, err := fcCheckSnapshot("/s/fc1", info, base(), fcTestHost); err != nil || len(w) != 0 {
		t.Errorf("fcCheckSnapshot() = %q, %v, want no warnings and no error", w, err)
	}

	for _, tc := range []struct {
		name string
		info func(*fcSnapshotInfo)
		cfg  func(*VMConfig)
		host func(*fcSnapshotHost)
		want string // empty for no error
	}{
		{name: "vm_type", info: func(i *fcSnapshotInfo) { i.VMType = "kvm" }, want: "is not a Firecracker snapshot"},
		{name: "kernel", cfg: func(c *VMConfig) { c.KernelPath = "" }, want: "firecracker-snapshot needs vm config kernel"},
		{name: "inject", cfg: func(c *VMConfig) { c.Injects = FCInjects{{Partition: 1, Src: "/a", Dst: "/b"}} }, want: "vm config inject does not apply"},
		{name: "arch", info: func(i *fcSnapshotInfo) { i.Arch = "aarch64" }, want: "is for aarch64, but this host is x86_64"},
		{name: "format major", host: func(h *fcSnapshotHost) { h.SnapshotVersion = fcVersion{13, 0, 0} }, want: "the major versions must be the same"},
		{name: "format minor", info: func(i *fcSnapshotInfo) { i.SnapshotVersion = "v12.1.0" }, want: "the snapshot minor version must not be higher"},
		{name: "older format minor", host: func(h *fcSnapshotHost) { h.SnapshotVersion = fcVersion{12, 3, 0}; h.Version = fcVersion{1, 18, 0} }},
		{name: "unknown format, same firecracker", host: func(h *fcSnapshotHost) { h.SnapshotVersionErr = errors.New("no flag") }},
		{
			name: "unknown format, other firecracker",
			host: func(h *fcSnapshotHost) { h.SnapshotVersionErr = errors.New("no flag"); h.Version = fcVersion{1, 18, 0} },
			want: "the snapshot format version is unknown (no flag), so the restore needs firecracker v1.17.0",
		},
		{
			name: "no recorded format, other firecracker",
			info: func(i *fcSnapshotInfo) { i.SnapshotVersion = "" },
			host: func(h *fcSnapshotHost) { h.Version = fcVersion{1, 18, 0} },
			want: "the snapshot does not record it",
		},
		{name: "cpu", host: func(h *fcSnapshotHost) { h.CPUModel = "Other CPU" }, want: "restore on the same kind of host"},
		{name: "host kernel", host: func(h *fcSnapshotHost) { h.Kernel = "6.9.0" }, want: "restore on the same kind of host"},
		{name: "vcpus", cfg: func(c *VMConfig) { c.VCPUs = 4 }, want: "has 2 vCPUs, but vm config vcpus is 4"},
		{name: "memory", cfg: func(c *VMConfig) { c.Memory = 1024 }, want: "has 512 MiB of memory, but vm config memory is 1024"},
		{name: "disks", cfg: func(c *VMConfig) { c.Disks = c.Disks[:1] }, want: "but vm config gives 1 disk (0 ro)"},
		{name: "ro disks", cfg: func(c *VMConfig) { c.Disks[1].ReadOnly = false }, want: "but vm config gives 2 disks (0 ro)"},
		{
			name: "ro disk order",
			cfg: func(c *VMConfig) {
				c.Disks = DiskConfigs{{Path: "/i/shared.squashfs", ReadOnly: true}, {Path: "/s/fc1.hdd"}}
			},
			want: "has the drives [/disk0, /disk1 (ro), /scratch (16777216 bytes)], but vm config gives [/disk0 (ro), /disk1, /scratch (16777216 bytes)]",
		},
		{name: "scratch", cfg: func(c *VMConfig) { c.FirecrackerScratch = "" }, want: "0 scratch"},
		{name: "scratch size", cfg: func(c *VMConfig) { c.FirecrackerScratch = "32M" }, want: "/scratch (33554432 bytes)"},
		{name: "nics", cfg: func(c *VMConfig) { c.Networks = append(c.Networks, NetConfig{VLAN: 102}) }, want: "but vm config gives 2 disks (1 ro), 1 scratch, 2 NICs"},
		{name: "vsock", cfg: func(c *VMConfig) { c.Backchannel = false }, want: "0 vsock"},
		{name: "entropy", cfg: func(c *VMConfig) { c.FirecrackerEntropy = false }, want: "0 entropy"},
		{name: "pci", cfg: func(c *VMConfig) { c.FirecrackerPCI = true }, want: "pci true"},
		{name: "macs", info: func(i *fcSnapshotInfo) { i.NICMACs = nil }, want: "has 0 NIC MACs for 1 NIC"},
		{name: "bad mac", info: func(i *fcSnapshotInfo) { i.NICMACs = []string{"$(reboot)"} }, want: `NIC 0 has the MAC "$(reboot)"`},
		{name: "long mac", info: func(i *fcSnapshotInfo) { i.NICMACs = []string{"00:00:00:00:fe:80:00:00"} }, want: "not a 6-byte MAC"},
		{name: "upper case mac", info: func(i *fcSnapshotInfo) { i.NICMACs = []string{"06:00:00:00:00:0A"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := info
			i.Drives = append([]fcSnapshotDrive(nil), info.Drives...)
			c := base()
			h := fcTestHost
			if tc.info != nil {
				tc.info(&i)
			}
			if tc.cfg != nil {
				tc.cfg(&c)
			}
			if tc.host != nil {
				tc.host(&h)
			}

			_, err := fcCheckSnapshot("/s/fc1", i, c, h)
			if tc.want == "" {
				if err != nil {
					t.Errorf("fcCheckSnapshot() = %v, want nil", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("fcCheckSnapshot() = %v, want %q", err, tc.want)
			}
		})
	}

	// Other disk paths are warnings.
	c := base()
	c.Disks[0].Path = "/s/other.hdd"
	c.Disks[1].Path = "/i/other.squashfs"
	w, err := fcCheckSnapshot("/s/fc1", info, c, fcTestHost)
	if err != nil || len(w) != 2 || !strings.Contains(w[0], "is /s/other.hdd, not /s/fc1.hdd") || !strings.Contains(w[1], "but the snapshot used /i/shared.squashfs") {
		t.Errorf("fcCheckSnapshot() with other disks = %q, %v", w, err)
	}
}

func TestFCRestore(t *testing.T) {
	f := setupFCDriver(t)
	setFCSnapshotSeams(t)

	cmds := &fcCmdLog{}
	fcRunCmd = func(args ...string) (string, error) {
		cmds.add(args)
		return "", nil
	}

	// The test records the owner of each chown and checks the real modes.
	var mu sync.Mutex
	owners := map[string][2]int{}
	var jailerArgs [][]string
	var tapUIDs []int

	fcChown = func(p string, uid, gid int) error {
		mu.Lock()
		defer mu.Unlock()
		owners[p] = [2]int{uid, gid}
		return nil
	}
	jailer := fcJailerCommand
	fcJailerCommand = func(bin string, args []string) *exec.Cmd {
		mu.Lock()
		jailerArgs = append(jailerArgs, args)
		mu.Unlock()
		return jailer(bin, args)
	}
	createTap := fcCreateTap
	fcCreateTap = func(nic NetConfig, ns *bridge.Netns, index, uid int) (string, int, error) {
		mu.Lock()
		tapUIDs = append(tapUIDs, uid)
		mu.Unlock()
		return createTap(nic, ns, index, uid)
	}
	jailerArg := func(n int, name string) string {
		mu.Lock()
		defer mu.Unlock()
		if n >= len(jailerArgs) {
			return ""
		}
		return fcArg(jailerArgs[n], name)
	}
	checkFile := func(p string, uid, gid int, mode os.FileMode) {
		t.Helper()
		fi, err := os.Stat(p)
		if err != nil {
			t.Error(err)
			return
		}
		mu.Lock()
		got := owners[p]
		mu.Unlock()
		if got != [2]int{uid, gid} || fi.Mode().Perm() != mode {
			t.Errorf("%v is %v:%v with mode %v, want %v:%v with mode %v", p, got[0], got[1], fi.Mode().Perm(), uid, gid, mode)
		}
	}

	prefix := filepath.Join(f.dir, "snap", "fc1")
	fcWriteFile(t, prefix+".state", []byte("state"))
	fcWriteFile(t, prefix+".mem", []byte("mem"))
	fcWriteFile(t, prefix+".hdd", make([]byte, 4096))
	scratch := bytes.Repeat([]byte("s"), 1<<20)
	fcWriteFile(t, prefix+".scratch", scratch)

	const savedMAC = "06:00:ac:10:00:02"

	cfg := f.config(t)
	cfg.Disks = DiskConfigs{{Path: prefix + ".hdd"}}
	cfg.FirecrackerScratch = "1M"
	cfg.FirecrackerSnapshotPath = prefix
	cfg.Networks[0].MAC = savedMAC

	info := fcTestSnapshotInfo(cfg)
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, prefix+".json", data)

	cfg.Networks[0].MAC = "00:11:22:33:44:55"
	vm := f.newVM(t, cfg)

	err = vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}

	// The test flushes vm at the end, so this flushes it only after a failure.
	flushed := false
	t.Cleanup(func() {
		if flushed {
			return
		}
		if vm.GetState()&VM_KILLABLE != 0 {
			if err := vm.Kill(); err != nil {
				t.Errorf("Kill() = %v", err)
			}
			waitFCState(t, vm, VM_QUIT)
		}
		if err := vm.Flush(); err != nil {
			t.Errorf("Flush() = %v", err)
		}
	})
	waitFCState(t, vm, VM_BUILDING)

	if got, want := f.requests(1), []string{"GET /", "PUT /logger", "PUT /serial", "PUT /snapshot/load"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restore requests =\n%q\nwant\n%q", got, want)
	}
	wantLoad := map[string]any{
		"snapshot_path": "/snapshot.state",
		"mem_backend": map[string]any{
			"backend_type": "File",
			"backend_path": "/snapshot.mem",
		},
		"resume_vm": false,
	}
	if got := f.body(1, "PUT", "/snapshot/load"); !reflect.DeepEqual(got, wantLoad) {
		t.Errorf("load = %v, want %v", got, wantLoad)
	}

	vm.lock.Lock()
	mac := vm.Networks[0].MAC
	vm.lock.Unlock()
	if mac != savedMAC {
		t.Errorf("MAC = %v, want %v from the snapshot", mac, savedMAC)
	}

	sameFile := func(a, b string) bool {
		fa, err1 := os.Stat(a)
		fb, err2 := os.Stat(b)
		return err1 == nil && err2 == nil && os.SameFile(fa, fb)
	}
	j := newFCJail(vm.ID, f.fcBin)
	for name, src := range map[string]string{"/snapshot.state": prefix + ".state", "/snapshot.mem": prefix + ".mem"} {
		if !sameFile(j.HostPath(name), src) {
			t.Errorf("jail %v is not a link to %v", name, src)
		}
	}

	if got, err := os.ReadFile(filepath.Join(fcInstanceDir(vm.ID), "scratch.raw")); err != nil || !bytes.Equal(got, scratch) {
		t.Errorf("scratch.raw = %d bytes, %v, want the content of %v", len(got), err, prefix+".scratch")
	}

	// The VMM runs as the VM uid and the group of the snapshot, the highest
	// gid below the uid base. Only the snapshot files get that group.
	uid := fcVMUID(vm.ID)
	gid := int(*f_firecrackerUIDBase) - 1
	if got, want := jailerArg(0, "--uid")+":"+jailerArg(0, "--gid"), strconv.Itoa(uid)+":"+strconv.Itoa(gid); got != want {
		t.Errorf("restore jailer --uid:--gid = %v, want %v", got, want)
	}
	for _, p := range []string{prefix + ".state", prefix + ".mem"} {
		checkFile(p, os.Geteuid(), gid, 0640)
	}
	for _, o := range []struct {
		path string
		mode os.FileMode
	}{
		{j.Root, 0700},
		{j.HostPath("out"), 0700},
		{j.HostPath("/console.fifo"), 0600},
		{j.HostPath("/firecracker.log"), 0600},
	} {
		checkFile(o.path, uid, uid, o.mode)
	}
	mu.Lock()
	if !reflect.DeepEqual(tapUIDs, []int{uid}) {
		t.Errorf("tap uids = %v, want [%v]", tapUIDs, uid)
	}
	mu.Unlock()
	if got := cmds.get(); len(got) != 0 {
		t.Errorf("commands = %q, want none", got)
	}

	// The VM uid owns the cc listener.
	cc := &fcFakeCC{t: t, vm: vm}
	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("connectCC() = %v", err)
	}
	expectCCCalls(t, cc, "register "+vm.UUID, "listen "+j.CCSocket()+" "+vm.UUID+" "+strconv.Itoa(uid))

	// vm start resumes the loaded snapshot.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)
	reqs := f.requests(1)
	if got := reqs[len(reqs)-1]; got != "PATCH /vm" || len(reqs) != 5 {
		t.Errorf("requests after start = %q, want PATCH /vm last", reqs)
	}
	if got := f.body(1, "PATCH", "/vm")["state"]; got != "Resumed" {
		t.Errorf("start state = %v, want Resumed", got)
	}

	// A relaunch boots the disk and does not load the snapshot again.
	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from QUIT = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	wantBoot := []string{
		"GET /",
		"PUT /logger",
		"PUT /machine-config",
		"PUT /boot-source",
		"PUT /drives/disk0",
		"PUT /drives/scratch",
		"PUT /network-interfaces/eth0",
		"PUT /vsock",
		"PUT /entropy",
		"PUT /serial",
		"PUT /actions",
	}
	if got := f.requests(2); !reflect.DeepEqual(got, wantBoot) {
		t.Errorf("relaunch requests =\n%q\nwant\n%q", got, wantBoot)
	}
	if _, err := os.Stat(j.HostPath("/snapshot.mem")); !os.IsNotExist(err) {
		t.Errorf("relaunch jail has /snapshot.mem: %v", err)
	}
	if got := jailerArg(1, "--gid"); got != strconv.Itoa(uid) {
		t.Errorf("boot jailer --gid = %v, want the VM uid %v", got, uid)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	flushed = true
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	for _, p := range []string{prefix + ".state", prefix + ".mem"} {
		checkFile(p, os.Geteuid(), os.Getegid(), 0600)
	}
	if got := cmds.get(); len(got) != 0 {
		t.Errorf("commands after flush = %q, want none", got)
	}
}

// fcTestSnapFiles writes the state and memory files of snapshot name in dir.
func fcTestSnapFiles(t *testing.T, dir, name string) *fcRestore {
	t.Helper()

	prefix := filepath.Join(dir, name)
	fcWriteFile(t, prefix+".state", []byte("state"))
	fcWriteFile(t, prefix+".mem", []byte("mem"))

	return &fcRestore{prefix: prefix, state: prefix + ".state", mem: prefix + ".mem"}
}

// setFCSnapGroupSeams gives the test its own snapshot groups, and records the
// owner that each chown gives and each command. chown returns the error of
// chownErr.
func setFCSnapGroupSeams(t *testing.T, chownErr error) (func(string) [2]int, *fcCmdLog) {
	t.Helper()

	setFCJailGlobals(t, t.TempDir())
	setFCOpsSeams(t)

	old := fcSnapGroups
	t.Cleanup(func() { fcSnapGroups = old })
	fcSnapGroups = newFCSnapGroups()

	var mu sync.Mutex
	owners := map[string][2]int{}
	fcChown = func(p string, uid, gid int) error {
		if chownErr != nil {
			return &os.PathError{Op: "chown", Path: p, Err: chownErr}
		}
		mu.Lock()
		defer mu.Unlock()
		owners[p] = [2]int{uid, gid}
		return nil
	}

	cmds := &fcCmdLog{}
	fcRunCmd = func(args ...string) (string, error) {
		cmds.add(args)
		return "", nil
	}

	return func(p string) [2]int {
		mu.Lock()
		defer mu.Unlock()
		return owners[p]
	}, cmds
}

func TestFCSnapGroups(t *testing.T) {
	owner, cmds := setFCSnapGroupSeams(t, nil)
	dir := t.TempDir()

	root := [2]int{os.Geteuid(), os.Getegid()}
	check := func(r *fcRestore, want [2]int, mode os.FileMode) {
		t.Helper()
		for _, p := range []string{r.state, r.mem} {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if got := owner(p); got != want || fi.Mode().Perm() != mode {
				t.Errorf("%v is %v with mode %v, want %v with mode %v", p, got, fi.Mode().Perm(), want, mode)
			}
		}
	}

	vms := make([]*FirecrackerVM, 5)
	for i := range vms {
		vms[i] = &FirecrackerVM{BaseVM: &BaseVM{ID: i + 1}}
	}
	grant := func(i int, r *fcRestore) int {
		t.Helper()
		gid, err := vms[i].grantSnapshotRead(r)
		if err != nil {
			t.Fatalf("grantSnapshotRead(vm %d) = %v", i+1, err)
		}
		return gid
	}

	a, b := fcTestSnapFiles(t, dir, "a"), fcTestSnapFiles(t, dir, "b")

	// Two restores of a share the highest gid below the uid base 200000.
	if gid := grant(0, a); gid != 199999 {
		t.Errorf("first restore of a: gid %v, want 199999", gid)
	}
	check(a, [2]int{os.Geteuid(), 199999}, 0640)

	// A vm save replaces the files, and the next restore gives them the group.
	if err := os.Remove(a.mem); err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, a.mem, []byte("new mem"))
	if err := os.Chmod(a.mem, 0600); err != nil {
		t.Fatal(err)
	}
	if gid := grant(1, a); gid != 199999 {
		t.Errorf("second restore of a: gid %v, want 199999", gid)
	}
	check(a, [2]int{os.Geteuid(), 199999}, 0640)
	if fcVMUID(vms[0].ID) == fcVMUID(vms[1].ID) {
		t.Errorf("the two restores of a have the same uid %v", fcVMUID(vms[0].ID))
	}

	// Another snapshot gets the next gid.
	if gid := grant(2, b); gid != 199998 {
		t.Errorf("restore of b: gid %v, want 199998", gid)
	}
	check(b, [2]int{os.Geteuid(), 199998}, 0640)

	// a keeps its group until the last restore of a is flushed.
	vms[0].revokeSnapshotRead()
	check(a, [2]int{os.Geteuid(), 199999}, 0640)
	vms[1].revokeSnapshotRead()
	check(a, root, 0600)

	// The free gid goes to the next new snapshot, and the reserved host IDs
	// do not.
	fcSnapGroups.reserve([]uint64{199997})
	c, d := fcTestSnapFiles(t, dir, "c"), fcTestSnapFiles(t, dir, "d")
	if gid := grant(3, c); gid != 199999 {
		t.Errorf("restore of c: gid %v, want the free gid 199999", gid)
	}
	if gid := grant(4, d); gid != 199996 {
		t.Errorf("restore of d: gid %v, want 199996 below the reserved 199997", gid)
	}

	for _, vm := range vms {
		vm.revokeSnapshotRead()
	}
	for _, r := range []*fcRestore{b, c, d} {
		check(r, root, 0600)
	}
	if n := len(fcSnapGroups.m); n != 0 {
		t.Errorf("%d snapshot groups after the flushes, want 0", n)
	}
	if got := cmds.get(); len(got) != 0 {
		t.Errorf("commands = %q, want none", got)
	}
}

func TestFCSnapGroupACLFallback(t *testing.T) {
	_, cmds := setFCSnapGroupSeams(t, errors.New("operation not permitted"))
	a := fcTestSnapFiles(t, t.TempDir(), "a")

	vm1 := &FirecrackerVM{BaseVM: &BaseVM{ID: 1}}
	vm2 := &FirecrackerVM{BaseVM: &BaseVM{ID: 2}}

	for _, vm := range []*FirecrackerVM{vm1, vm2} {
		gid, err := vm.grantSnapshotRead(a)
		if err != nil {
			t.Fatalf("grantSnapshotRead(vm %v) = %v", vm.ID, err)
		}
		if gid != fcVMUID(vm.ID) {
			t.Errorf("vm %v: gid %v, want the VM uid %v", vm.ID, gid, fcVMUID(vm.ID))
		}
	}

	vm1.revokeSnapshotRead()
	vm2.revokeSnapshotRead()

	want := [][]string{
		{"setfacl", "-m", "u:200001:r", a.state},
		{"setfacl", "-m", "u:200001:r", a.mem},
		{"setfacl", "-m", "u:200002:r", a.state},
		{"setfacl", "-m", "u:200002:r", a.mem},
		{"setfacl", "-x", "u:200001", a.state},
		{"setfacl", "-x", "u:200001", a.mem},
		{"setfacl", "-x", "u:200002", a.state},
		{"setfacl", "-x", "u:200002", a.mem},
	}
	if got := cmds.get(); !reflect.DeepEqual(got, want) {
		t.Errorf("commands =\n%q\nwant\n%q", got, want)
	}
	if n := len(fcSnapGroups.m); n != 0 {
		t.Errorf("%d snapshot groups after the flushes, want 0", n)
	}
}

func TestFCRestoreErrors(t *testing.T) {
	f := setupFCDriver(t)
	setFCSnapshotSeams(t)

	prefix := filepath.Join(f.dir, "snap", "fc1")
	cfg := f.config(t)
	cfg.Disks = DiskConfigs{{Path: prefix + ".hdd"}}
	cfg.FirecrackerSnapshotPath = prefix
	cfg.Networks[0].MAC = "06:00:00:00:00:01"

	info := fcTestSnapshotInfo(cfg)
	info.VCPUs = 4
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, prefix+".json", data)

	vm := f.newVM(t, cfg)
	if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "has 4 vCPUs, but vm config vcpus is 1") {
		t.Fatalf("Launch() = %v, want vCPU error", err)
	}
	waitFCState(t, vm, VM_ERROR)
	if got := f.get("create-netns"); got != 0 {
		t.Errorf("a failed check prepared the VM: %d netns", got)
	}
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	// A sidecar without the state file fails the launch.
	info.VCPUs = cfg.VCPUs
	if data, err = json.Marshal(info); err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, prefix+".json", data)

	vm = f.newVM(t, cfg)
	if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "fc1.state") {
		t.Fatalf("Launch() without a state file = %v", err)
	}
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestWriteFirecrackerRestore(t *testing.T) {
	cfg := NewVMConfig()
	cfg.KernelPath = "/images/vmlinux"
	cfg.Disks = DiskConfigs{
		{Path: "/images/rootfs.raw"},
		{Path: "/images/data.raw", Cache: "unsafe"},
		{Path: "/images/shared.squashfs", ReadOnly: true},
	}
	cfg.Injects = FCInjects{{Partition: 1, Src: "motd", Dst: "/etc/motd"}}
	vm := newTestFCVM(t, cfg)

	dst := filepath.Join(*f_iomBase, "saved", "ns1", "fc1")

	var buf bytes.Buffer
	if err := writeFirecrackerRestore(&buf, vm, dst, true); err != nil {
		t.Fatalf("writeFirecrackerRestore() = %v", err)
	}
	out := buf.String()

	if !strings.HasPrefix(out, "clear vm config\n") || !strings.Contains(out, "vm config kernel /images/vmlinux\n") {
		t.Errorf("output does not start with clear vm config and the VM config:\n%v", out)
	}
	wantEnd := "clear vm config inject\n" +
		"vm config firecracker-snapshot saved/ns1/fc1\n" +
		"vm config disks file:saved/ns1/fc1.hdd file:saved/ns1/fc1.hdd1,unsafe /images/shared.squashfs,ro\n" +
		"vm launch firecracker \"fc1\"\n\n"
	if !strings.HasSuffix(out, wantEnd) {
		t.Errorf("output =\n%v\nwant the end\n%v", out, wantEnd)
	}

	buf.Reset()
	if err := writeFirecrackerRestore(&buf, vm, "/saves/fc1", false); err != nil {
		t.Fatalf("writeFirecrackerRestore() = %v", err)
	}
	wantEnd = "clear vm config inject\n" +
		"vm config firecracker-snapshot /saves/fc1\n" +
		"vm config disks /saves/fc1.hdd /saves/fc1.hdd1,unsafe /images/shared.squashfs,ro\n" +
		"vm launch firecracker \"fc1\"\n\n"
	if out := buf.String(); !strings.HasSuffix(out, wantEnd) {
		t.Errorf("output =\n%v\nwant the end\n%v", out, wantEnd)
	}
}

func TestFCSnapshotFormat(t *testing.T) {
	old := fcRunSnapshotVersion
	t.Cleanup(func() { fcRunSnapshotVersion = old })

	var mu sync.Mutex
	runs := map[string]int{}
	version := "v12.0.0\n"
	fcRunSnapshotVersion = func(p string) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		runs[filepath.Base(p)]++
		if filepath.Base(p) == "bad" && runs["bad"] == 1 {
			return "", errors.New("exit status 1")
		}
		return version, nil
	}

	dir := t.TempDir()
	good := filepath.Join(dir, "firecracker")
	bad := filepath.Join(dir, "bad")
	for _, p := range []string{good, bad} {
		if err := os.WriteFile(p, nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if v, err := fcSnapshotFormat(good); err != nil || v != (fcVersion{12, 0, 0}) {
			t.Errorf("fcSnapshotFormat() = %v, %v, want v12.0.0", v, err)
		}
	}
	if _, err := fcSnapshotFormat(bad); err == nil || !strings.Contains(err.Error(), "--snapshot-version failed") {
		t.Errorf("fcSnapshotFormat(bad) = %v", err)
	}
	if v, err := fcSnapshotFormat(bad); err != nil || v != (fcVersion{12, 0, 0}) {
		t.Errorf("fcSnapshotFormat(bad) after a failure = %v, %v, want v12.0.0", v, err)
	}
	if runs["firecracker"] != 1 || runs["bad"] != 2 {
		t.Errorf("runs = %v, want one run of firecracker and two of bad", runs)
	}

	version = "v13.0.0\n"
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(good, future, future); err != nil {
		t.Fatal(err)
	}
	if v, err := fcSnapshotFormat(good); err != nil || v != (fcVersion{13, 0, 0}) {
		t.Errorf("fcSnapshotFormat() after an upgrade = %v, %v, want v13.0.0", v, err)
	}
}

func TestFCCPUModel(t *testing.T) {
	x86 := "processor\t: 0\nvendor_id\t: GenuineIntel\nmodel name\t: Intel(R) Xeon(R) CPU @ 2.20GHz\n\nprocessor\t: 1\nmodel name\t: other\n"
	if got := fcCPUModel(x86); got != "Intel(R) Xeon(R) CPU @ 2.20GHz" {
		t.Errorf("fcCPUModel(x86) = %q", got)
	}

	arm := "processor\t: 0\nBogoMIPS\t: 50.00\nCPU implementer\t: 0x41\nCPU architecture: 8\nCPU part\t: 0xd0c\n"
	if got := fcCPUModel(arm); got != "implementer 0x41 part 0xd0c" {
		t.Errorf("fcCPUModel(arm) = %q", got)
	}

	if got := fcCPUModel(""); got != "" {
		t.Errorf("fcCPUModel(empty) = %q", got)
	}
}
