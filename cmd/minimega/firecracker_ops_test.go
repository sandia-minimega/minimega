// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// setFCOpsSeams gives the test its own export registry and restores the
// export seams.
func setFCOpsSeams(t *testing.T) {
	t.Helper()

	oldExports, oldConvert := fcExports, fcConvertQcow2
	oldReflink, oldClone, oldRun := fcReflinkOK, fcCloneFile, fcRunCmd
	oldInjects, oldWait := fcApplyInjectsFn, fcWaitDev
	t.Cleanup(func() {
		fcExports, fcConvertQcow2 = oldExports, oldConvert
		fcReflinkOK, fcCloneFile, fcRunCmd = oldReflink, oldClone, oldRun
		fcApplyInjectsFn, fcWaitDev = oldInjects, oldWait
	})

	fcExports = &fcExportRegistry{m: map[int]*fcExport{}}
}

// reqAt returns request i (from 0) of API server n (from 1).
func (f *fcDriverFakes) reqAt(n, i int) fcRecordedRequest {
	f.mu.Lock()
	s := f.servers[n-1]
	f.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.reqs[i]
}

func waitFCExport(t *testing.T, id int, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		fcExports.mu.Lock()
		e := fcExports.m[id]
		fcExports.mu.Unlock()

		var got string
		if e != nil {
			got, _ = e.get()
		}
		if got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("export status = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// cleanupFCVM kills vm when it is killable, then flushes it.
func cleanupFCVM(t *testing.T, vm *FirecrackerVM) {
	t.Cleanup(func() {
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
}

// launchFCRunning launches a VM through the fake driver and starts it.
func launchFCRunning(t *testing.T, f *fcDriverFakes) *FirecrackerVM {
	t.Helper()

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	cleanupFCVM(t, vm)

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	return vm
}

func TestFCShutdown(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}

	if err := vm.Shutdown(); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("Shutdown() in BUILDING = %v, want not running", err)
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	if err := vmShutdown(vm); err != nil {
		t.Fatalf("vmShutdown() = %v", err)
	}

	reqs := f.requests(1)
	if got := reqs[len(reqs)-1]; got != "PUT /actions" {
		t.Fatalf("last request = %v, want PUT /actions", got)
	}
	if b := f.body(1, "PUT", "/actions"); b["action_type"] != "SendCtrlAltDel" {
		t.Errorf("action = %v, want SendCtrlAltDel", b)
	}

	// The guest reboots, and the VMM exits with status 0.
	if err := os.WriteFile(f.exitFile(1), nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitFCState(t, vm, VM_QUIT)

	if err := vm.Shutdown(); err == nil {
		t.Errorf("Shutdown() in QUIT = nil, want error")
	}

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestFCShutdownTypes(t *testing.T) {
	for _, vm := range []VM{
		&ContainerVM{BaseVM: &BaseVM{Name: "c", Type: CONTAINER}},
		&AndroidVM{BaseVM: &BaseVM{Name: "a", Type: ANDROID}},
	} {
		want := "vm shutdown is not supported for " + vm.GetType().String() + " VMs"
		if err := vmShutdown(vm); err == nil || err.Error() != want {
			t.Errorf("vmShutdown(%v) = %v, want %q", vm.GetType(), err, want)
		}
	}

	kvm := &KvmVM{BaseVM: &BaseVM{Name: "k", Type: KVM, State: VM_BUILDING}}
	if err := vmShutdown(kvm); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Errorf("vmShutdown(kvm in BUILDING) = %v, want not running", err)
	}
}

func TestFCResetDisk(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	fcApplyInjectsFn = func(string, FCInjects) error {
		f.count("inject")
		return nil
	}

	vm := launchFCRunning(t, f)

	if err := vm.ResetDisk(); err == nil || !strings.Contains(err.Error(), "needs a vm in quit or error") {
		t.Errorf("ResetDisk() in RUNNING = %v, want state error", err)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	vm.lock.Lock()
	vm.Injects = FCInjects{{Partition: 1, Src: "/a", Dst: "/b"}}
	vm.lock.Unlock()

	if err := vm.ResetDisk(); err != nil {
		t.Fatalf("ResetDisk() = %v", err)
	}

	for k, want := range map[string]int{"release-disk": 1, "prepare-disk": 2, "inject": 1} {
		if got := f.get(k); got != want {
			t.Errorf("%v calls = %d, want %d", k, got, want)
		}
	}

	// The next start boots the new disk and does not build it again.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from QUIT = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	for k, want := range map[string]int{"prepare-disk": 2, "inject": 1} {
		if got := f.get(k); got != want {
			t.Errorf("after start, %v calls = %d, want %d", k, got, want)
		}
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	vm.lock.Lock()
	vm.Snapshot = false
	vm.lock.Unlock()

	if err := vm.ResetDisk(); err == nil || err.Error() != "vm reset-disk needs snapshot true" {
		t.Errorf("ResetDisk() with snapshot false = %v", err)
	}
}

func TestFCResetDiskReleaseFailure(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	vm := launchFCRunning(t, f)
	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	var released []string
	fail := true
	fcReleaseDiskFn = func(d fcDisk) error {
		if fail {
			return errors.New("device busy")
		}
		released = append(released, d.HostPath)
		return nil
	}

	vm.lock.Lock()
	disk := vm.disks[0].HostPath
	vm.lock.Unlock()

	if err := vm.ResetDisk(); err == nil || !strings.Contains(err.Error(), "device busy") {
		t.Fatalf("ResetDisk() = %v, want a release error", err)
	}

	fail = false
	if err := vm.ResetDisk(); err != nil {
		t.Fatalf("second ResetDisk() = %v", err)
	}
	if len(released) != 1 || released[0] != disk {
		t.Errorf("released = %v, want the disk that failed to release, %v", released, disk)
	}
}

func TestFCExportDiskRunning(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	var mu sync.Mutex
	var events []string

	fcReflinkOK = func() bool { return true }
	fcCloneFile = func(src, dst string) error {
		mu.Lock()
		events = append(events, "clone after "+strconv.Itoa(len(f.requests(1))))
		mu.Unlock()
		return fcSparseCopy(src, dst)
	}
	fcConvertQcow2 = func(src, dst string, progress func(float64)) error {
		mu.Lock()
		events = append(events, "convert after "+strconv.Itoa(len(f.requests(1))))
		mu.Unlock()
		if _, err := os.Stat(src); err != nil {
			t.Errorf("frozen copy: %v", err)
		}
		progress(50)
		return os.WriteFile(dst, []byte("qcow2"), 0600)
	}

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	cleanupFCVM(t, vm)

	dst := filepath.Join(f.dir, "out", "disk.qcow2")
	if err := vm.ExportDisk(dst, "qcow2"); err == nil || !strings.Contains(err.Error(), "needs a vm in running, paused, or quit") {
		t.Errorf("ExportDisk() in BUILDING = %v, want state error", err)
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		t.Fatal(err)
	}

	before := len(f.requests(1))
	if err := vm.ExportDisk(dst, "qcow2"); err != nil {
		t.Fatalf("ExportDisk() = %v", err)
	}
	waitFCExport(t, vm.ID, fcExportCompleted)

	if got := f.requests(1)[before:]; !reflect.DeepEqual(got, []string{"PATCH /vm", "PATCH /vm"}) {
		t.Fatalf("requests = %q, want pause and resume", got)
	}
	for i, want := range []string{"Paused", "Resumed"} {
		if got := f.reqAt(1, before+i).Body["state"]; got != want {
			t.Errorf("request %d state = %v, want %v", before+i, got, want)
		}
	}

	wantEvents := []string{
		"clone after " + strconv.Itoa(before+1),
		"convert after " + strconv.Itoa(before+2),
	}
	mu.Lock()
	if !reflect.DeepEqual(events, wantEvents) {
		t.Errorf("events = %q, want %q", events, wantEvents)
	}
	mu.Unlock()

	if got := vm.GetState(); got != VM_RUNNING {
		t.Errorf("state = %v, want RUNNING", got)
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "qcow2" {
		t.Errorf("destination = %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(fcInstanceDir(vm.ID), "export-1.raw")); !os.IsNotExist(err) {
		t.Errorf("frozen copy exists after export: %v", err)
	}

	want := [][]string{{strconv.Itoa(vm.ID), "fc1", "completed", "100.00"}}
	if got := fcExports.table(DefaultNamespace); !reflect.DeepEqual(got, want) {
		t.Errorf("table = %q, want %q", got, want)
	}

	// A failed reflink copy of a running VM is an error, not a slow copy.
	fcCloneFile = func(string, string) error { return errors.New("EOPNOTSUPP") }
	raw := filepath.Join(f.dir, "out", "disk.raw")
	if err := vm.ExportDisk(raw, "raw"); err == nil || !strings.Contains(err.Error(), "Pause or stop the VM first") {
		t.Fatalf("ExportDisk(raw) with a failed reflink = %v, want a fast copy error", err)
	}
	if got := vm.GetState(); got != VM_RUNNING {
		t.Errorf("state = %v, want RUNNING", got)
	}

	// raw, without reflink: refused while running. A paused VM gets a sparse
	// copy, then a sparse copy to the destination.
	fcReflinkOK = func() bool { return false }

	content := bytes.Repeat([]byte("x"), 4096)
	if err := os.WriteFile(filepath.Join(fcInstanceDir(vm.ID), "disk-0.raw"), content, 0600); err != nil {
		t.Fatal(err)
	}

	before = len(f.requests(1))
	if err := vm.ExportDisk(raw, "raw"); err == nil || !strings.Contains(err.Error(), "Pause or stop the VM first") {
		t.Fatalf("ExportDisk(raw) without reflink = %v, want a fast copy error", err)
	}
	if got := f.requests(1)[before:]; len(got) != 0 {
		t.Errorf("requests = %q, want none for a refused export", got)
	}

	if err := vm.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	if err := vm.ExportDisk(raw, "raw"); err != nil {
		t.Fatalf("ExportDisk(raw) = %v", err)
	}
	waitFCExport(t, vm.ID, fcExportCompleted)

	if data, err := os.ReadFile(raw); err != nil || !bytes.Equal(data, content) {
		t.Errorf("raw destination = %d bytes, %v", len(data), err)
	}
	if got := vm.GetState(); got != VM_PAUSED {
		t.Errorf("state = %v, want PAUSED", got)
	}
}

func TestFCExportDiskConcurrent(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	fcReflinkOK = func() bool { return true }
	fcCloneFile = fcSparseCopy

	release := make(chan struct{})
	fcConvertQcow2 = func(src, dst string, progress func(float64)) error {
		progress(25)
		<-release
		return os.WriteFile(dst, []byte("qcow2"), 0600)
	}

	vm := launchFCRunning(t, f)

	dst := filepath.Join(f.dir, "a.qcow2")
	if err := vm.ExportDisk(dst, "qcow2"); err != nil {
		t.Fatalf("ExportDisk() = %v", err)
	}

	if err := vm.ExportDisk(filepath.Join(f.dir, "b.qcow2"), "qcow2"); err == nil || !strings.Contains(err.Error(), "in progress") {
		t.Errorf("second ExportDisk() = %v, want in progress error", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for {
		got := fcExports.table(DefaultNamespace)
		if len(got) == 1 && got[0][2] == "active" && got[0][3] == "25.00" {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("table = %q, want one active row at 25.00", got)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	waitFCExport(t, vm.ID, fcExportCompleted)

	if _, err := os.Stat(filepath.Join(f.dir, "b.qcow2")); !os.IsNotExist(err) {
		t.Errorf("second destination exists: %v", err)
	}
}

func TestFCExportDiskDM(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	cmds := &fcCmdLog{}
	fcRunCmd = func(args ...string) (string, error) {
		cmds.add(args)
		if len(args) > 2 && args[0] == "losetup" && args[1] == "--find" {
			return "/dev/loop9\n", nil
		}
		return "", nil
	}

	var convertSrc string
	fcConvertQcow2 = func(src, dst string, progress func(float64)) error {
		convertSrc = src
		return os.WriteFile(dst, []byte("qcow2"), 0600)
	}

	var waited []string
	fcWaitDev = func(dev string, timeout time.Duration) error {
		waited = append(waited, dev)
		return nil
	}

	vm := f.newVM(t, f.config(t))
	vm.lock.Unlock()

	// Flush removes the alias that launch creates.
	if err := vm.createInstancePathAlias(); err != nil {
		t.Fatal(err)
	}

	dir := fcInstanceDir(vm.ID)
	cow := fcWriteFile(t, filepath.Join(dir, "disk-0.cow"), []byte("cow data"))
	fcWriteFile(t, filepath.Join(fcCacheDir(), "k.raw"), make([]byte, 8192))

	fcCache.bases["k"] = &fcBaseLoop{dev: "/dev/loop1", refs: 1}

	vm.lock.Lock()
	vm.disks = []fcDisk{{
		HostPath:     "/dev/mapper/mm-fc-t-0",
		IsBlock:      true,
		vmID:         vm.ID,
		key:          "k",
		baseAttached: true,
		cowFile:      cow,
		cowLoop:      "/dev/loop2",
		dmName:       "mm-fc-t-0",
	}}
	vm.State = VM_QUIT
	vm.lock.Unlock()

	// The CLI refuses a destination that exists.
	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{vm.ID: vm}

	dst := filepath.Join(f.dir, "disk.qcow2")
	fcWriteFile(t, dst, nil)
	cmd := &minicli.Command{StringArgs: map[string]string{"vm": "fc1", "dst": dst}, BoolArgs: map[string]bool{}}
	if err := cliVMExportDisk(ns, cmd, &minicli.Response{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("cliVMExportDisk() to an existing file = %v", err)
	}
	os.Remove(dst)

	if err := cliVMExportDisk(ns, cmd, &minicli.Response{}); err != nil {
		t.Fatalf("cliVMExportDisk() = %v", err)
	}
	waitFCExport(t, vm.ID, fcExportCompleted)

	name := "mm-fc-" + fcInstance + "-" + strconv.Itoa(vm.ID) + "-x1"
	xcow := filepath.Join(dir, "export-1.cow")
	want := [][]string{
		{"blockdev", "--flushbufs", "/dev/mapper/mm-fc-t-0"},
		{"losetup", "--find", "--show", xcow},
		{"dmsetup", "create", name, "--table", fcDMSnapshotTable(8192, "/dev/loop1", "/dev/loop9")},
		{"dmsetup", "mknodes", name},
		{"dmsetup", "remove", name},
		{"losetup", "-d", "/dev/loop9"},
	}
	if got := cmds.get(); !reflect.DeepEqual(got, want) {
		t.Errorf("commands =\n%q\nwant\n%q", got, want)
	}
	if want := []string{"/dev/mapper/" + name}; !reflect.DeepEqual(waited, want) {
		t.Errorf("waited for %q, want %q", waited, want)
	}

	if convertSrc != "/dev/mapper/"+name {
		t.Errorf("convert source = %v, want /dev/mapper/%v", convertSrc, name)
	}
	if _, err := os.Stat(xcow); !os.IsNotExist(err) {
		t.Errorf("frozen copy-on-write file exists after export: %v", err)
	}

	fcCache.mu.Lock()
	refs := fcCache.bases["k"].refs
	fcCache.mu.Unlock()
	if refs != 1 {
		t.Errorf("base loop refs = %d, want 1", refs)
	}

	if got := vm.GetState(); got != VM_QUIT {
		t.Errorf("state = %v, want QUIT", got)
	}

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
	if got := fcExports.table(DefaultNamespace); len(got) != 0 {
		t.Errorf("table after flush = %q", got)
	}
}

func TestFCExportDiskTypes(t *testing.T) {
	setFCOpsSeams(t)

	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{1: &ContainerVM{BaseVM: &BaseVM{ID: 1, Name: "c", Type: CONTAINER}}}

	cmd := &minicli.Command{StringArgs: map[string]string{"vm": "c", "dst": "/x.qcow2"}, BoolArgs: map[string]bool{}}
	if err := cliVMExportDisk(ns, cmd, &minicli.Response{}); err == nil || err.Error() != "vm export-disk is not supported for container VMs" {
		t.Errorf("cliVMExportDisk(container) = %v", err)
	}

	cmd = &minicli.Command{StringArgs: map[string]string{"vm": "c"}, BoolArgs: map[string]bool{}}
	if err := cliVMResetDisk(ns, cmd, &minicli.Response{}); err == nil || !strings.Contains(err.Error(), "vm reset-disk is not supported for container VMs") {
		t.Errorf("cliVMResetDisk(container) = %v", err)
	}
}

func TestFCExportTableMatchesSave(t *testing.T) {
	setFCOpsSeams(t)

	ns := &Namespace{Name: DefaultNamespace}
	cmd := &minicli.Command{StringArgs: map[string]string{}, BoolArgs: map[string]bool{}}

	save := &minicli.Response{}
	if err := cliVMSave(ns, cmd, save); err != nil {
		t.Fatalf("cliVMSave() = %v", err)
	}

	fcExports.m[7] = &fcExport{ID: 7, Name: "fc7", Namespace: DefaultNamespace, status: fcExportActive, complete: 12.5}
	fcExports.m[8] = &fcExport{ID: 8, Name: "other", Namespace: "other", status: fcExportActive}

	export := &minicli.Response{}
	if err := cliVMExportDisk(ns, cmd, export); err != nil {
		t.Fatalf("cliVMExportDisk() = %v", err)
	}

	if !reflect.DeepEqual(export.Header, save.Header) {
		t.Errorf("export-disk header = %q, vm save header = %q", export.Header, save.Header)
	}
	if want := [][]string{{"7", "fc7", "active", "12.50"}}; !reflect.DeepEqual(export.Tabular, want) {
		t.Errorf("export-disk rows = %q, want %q", export.Tabular, want)
	}
}

func TestFCReadProgress(t *testing.T) {
	var got []float64
	fcReadProgress(strings.NewReader("    (0.00/100%)\r    (12.50/100%)\r    (100.00/100%)\r\n"), func(p float64) {
		got = append(got, p)
	})

	if want := []float64{0, 12.5, 100}; !reflect.DeepEqual(got, want) {
		t.Errorf("progress = %v, want %v", got, want)
	}
}

func TestFCWriteExportNoReplace(t *testing.T) {
	setFCOpsSeams(t)

	dir := t.TempDir()
	dst := filepath.Join(dir, "disk.qcow2")
	fcConvertQcow2 = func(src, out string, progress func(float64)) error {
		// Another export makes the destination during this conversion.
		if err := os.WriteFile(dst, []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
		return os.WriteFile(out, []byte("second"), 0600)
	}

	e := &fcExport{ID: 1, Dst: dst, Format: "qcow2"}
	if err := fcWriteExport(e, filepath.Join(dir, "src.raw")); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("fcWriteExport() = %v, want an exists error", err)
	}
	if data, _ := os.ReadFile(dst); string(data) != "first" {
		t.Errorf("destination = %q, want the first export", data)
	}
	if m, _ := filepath.Glob(dst + ".tmp-*"); len(m) != 0 {
		t.Errorf("temporary files left: %v", m)
	}
}

func TestFCExportFlushActive(t *testing.T) {
	f := setupFCDriver(t)
	setFCOpsSeams(t)

	fcReflinkOK = func() bool { return true }
	fcCloneFile = fcSparseCopy

	release := make(chan struct{})
	fcConvertQcow2 = func(src, dst string, progress func(float64)) error {
		<-release
		if _, err := os.Stat(src); err != nil {
			return err
		}
		return os.WriteFile(dst, []byte("qcow2"), 0600)
	}

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	if err := vm.ExportDisk(filepath.Join(f.dir, "a.qcow2"), "qcow2"); err != nil {
		t.Fatalf("ExportDisk() = %v", err)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	close(release)
	waitFCExport(t, vm.ID, fcExportFailed)
}
