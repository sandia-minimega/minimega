// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
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
)

// fcDriverFakes replaces the host operations of the Firecracker driver. Each
// jailer start is a shell that exits 0 when its exit file appears, and a test
// API server on the jail socket.
type fcDriverFakes struct {
	t     *testing.T
	dir   string
	fcBin string

	mu      sync.Mutex
	counts  map[string]int
	servers []*fcTestServer

	// script is the shell script of the fake VMM. $1 is the exit file.
	script string

	// serve is false to start no API server.
	serve bool
}

func setupFCDriver(t *testing.T) *fcDriverFakes {
	t.Helper()

	// A short directory keeps the jail socket paths under the size limit.
	tmp, err := os.MkdirTemp("", "fcd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })

	dir := fcResolve(tmp)
	setFCJailGlobals(t, dir)

	oldBase, oldCache := *f_base, fcCache
	oldCheck, oldPrepare, oldRelease := fcCheckHost, fcPrepareDiskFn, fcReleaseDiskFn
	oldNetns, oldDelNetns, oldTap, oldDelTap := fcCreateNetns, fcCloseNetns, fcCreateTap, fcDestroyTap
	oldCommand := fcJailerCommand
	t.Cleanup(func() {
		*f_base, fcCache = oldBase, oldCache
		fcCheckHost, fcPrepareDiskFn, fcReleaseDiskFn = oldCheck, oldPrepare, oldRelease
		fcCreateNetns, fcCloseNetns, fcCreateTap, fcDestroyTap = oldNetns, oldDelNetns, oldTap, oldDelTap
		fcJailerCommand = oldCommand
	})

	f := &fcDriverFakes{
		t:      t,
		dir:    dir,
		fcBin:  filepath.Join(dir, "firecracker"),
		counts: map[string]int{},
		script: `while [ ! -e "$1" ]; do sleep 0.05; done`,
		serve:  true,
	}

	*f_base = filepath.Join(dir, "base")
	fcCache = newFCCacheState()
	hostCgroupVersion = 2
	fcCgroupRoot = filepath.Join(dir, "cgroup")
	fcMountinfoPath = filepath.Join(dir, "mountinfo")
	if err := os.WriteFile(fcMountinfoPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	fcChown = func(string, int, int) error { return nil }

	fcCheckHost = func(string, string) (string, string, error) {
		return f.fcBin, "jailer", nil
	}
	fcPrepareDiskFn = func(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (fcDisk, error) {
		f.count("prepare-disk")
		p := filepath.Join(instanceDir, fmt.Sprintf("disk-%d.raw", index))
		if err := os.WriteFile(p, make([]byte, 4096), 0600); err != nil {
			return fcDisk{}, err
		}
		return fcDisk{HostPath: p, CacheType: "Writeback"}, nil
	}
	fcReleaseDiskFn = func(fcDisk) error {
		f.count("release-disk")
		return nil
	}
	fcCreateNetns = func() (*bridge.Netns, error) {
		f.count("create-netns")
		return new(bridge.Netns), nil
	}
	fcCloseNetns = func(ns *bridge.Netns) error {
		if ns != nil {
			f.count("destroy-netns")
		}
		return nil
	}
	fcCreateTap = func(nic NetConfig, ns *bridge.Netns, index, uid int) (string, int, error) {
		if ns == nil {
			return "", 0, errors.New("tap without a netns")
		}
		f.count("create-tap")
		return fmt.Sprintf("mega_tap%d", index), 1500, nil
	}
	fcDestroyTap = func(NetConfig) error {
		f.count("destroy-tap")
		return nil
	}
	fcJailerCommand = f.command

	return f
}

func (f *fcDriverFakes) count(k string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.counts[k]++
}

func (f *fcDriverFakes) get(k string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.counts[k]
}

func fcArg(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func (f *fcDriverFakes) command(bin string, args []string) *exec.Cmd {
	root := filepath.Join(fcArg(args, "--chroot-base-dir"), filepath.Base(fcArg(args, "--exec-file")), fcArg(args, "--id"), "root")

	f.mu.Lock()
	n := len(f.servers) + 1
	f.mu.Unlock()

	if f.serve {
		s := newFCTestServer(f.t, fcListen(f.t, filepath.Join(root, "api.sock")))
		f.mu.Lock()
		f.servers = append(f.servers, s)
		f.mu.Unlock()
	}

	return exec.Command("sh", "-c", f.script, "sh", f.exitFile(n))
}

func (f *fcDriverFakes) exitFile(n int) string {
	return filepath.Join(f.dir, fmt.Sprintf("exit%d", n))
}

// requests returns "METHOD path" for each request to API server n (from 1).
func (f *fcDriverFakes) requests(n int) []string {
	f.mu.Lock()
	s := f.servers[n-1]
	f.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	var res []string
	for _, r := range s.reqs {
		res = append(res, r.Method+" "+r.Path)
	}
	return res
}

func (f *fcDriverFakes) body(n int, method, path string) map[string]any {
	f.mu.Lock()
	s := f.servers[n-1]
	f.mu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	var res map[string]any
	for _, r := range s.reqs {
		if r.Method == method && r.Path == path {
			res = r.Body
		}
	}
	return res
}

func (f *fcDriverFakes) newVM(t *testing.T, cfg VMConfig) *FirecrackerVM {
	t.Helper()

	vm, err := NewFirecracker("fc1", DefaultNamespace, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return vm
}

func (f *fcDriverFakes) config(t *testing.T) VMConfig {
	t.Helper()

	kernel := filepath.Join(f.dir, "vmlinux")
	if err := os.WriteFile(kernel, []byte("kernel"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := NewVMConfig()
	cfg.KernelPath = kernel
	cfg.Disks = DiskConfigs{{Path: filepath.Join(f.dir, "rootfs.raw")}}
	cfg.Networks = NetConfigs{{VLAN: 101, Bridge: DefaultBridge}}
	cfg.Backchannel = true
	cfg.FirecrackerEntropy = true
	return cfg
}

func waitFCState(t *testing.T, vm *FirecrackerVM, want VMState) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if got := vm.GetState(); got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("state = %v, want %v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func skipNoFIFO(t *testing.T, err error) {
	t.Helper()

	if err != nil && strings.Contains(err.Error(), "mkfifo") {
		t.Skipf("mkfifo not supported here: %v", err)
	}
}

var fcLaunchRequests = []string{
	"GET /",
	"PUT /logger",
	"PUT /machine-config",
	"PUT /boot-source",
	"PUT /drives/disk0",
	"PUT /network-interfaces/eth0",
	"PUT /vsock",
	"PUT /entropy",
	"PUT /serial",
}

func TestFCDriverLifecycle(t *testing.T) {
	f := setupFCDriver(t)
	cfg := f.config(t)
	vm := f.newVM(t, cfg)

	jailDir := newFCJail(vm.ID, f.fcBin).Dir()
	instanceDir := fcInstanceDir(vm.ID)

	expect := func(n int, want ...string) {
		t.Helper()
		if got := f.requests(n); !reflect.DeepEqual(got, want) {
			t.Fatalf("API server %d requests =\n%q\nwant\n%q", n, got, want)
		}
	}
	jailExists := func(want bool) {
		t.Helper()
		_, err := os.Stat(jailDir)
		if got := err == nil; got != want {
			t.Fatalf("jail %v exists = %v, want %v (%v)", jailDir, got, want, err)
		}
	}

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}

	waitFCState(t, vm, VM_BUILDING)
	expect(1, fcLaunchRequests...)
	jailExists(true)

	if fi, err := os.Lstat(vm.instancePath); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("instance path %v is not a symlink: %v", vm.instancePath, err)
	}
	if target, _ := os.Readlink(vm.instancePath); target != instanceDir {
		t.Errorf("instance path links to %v, want %v", target, instanceDir)
	}

	drive := f.body(1, "PUT", "/drives/disk0")
	if drive["path_on_host"] != "/disk0" || drive["is_root_device"] != true || drive["cache_type"] != "Writeback" {
		t.Errorf("drive = %v", drive)
	}
	nic := f.body(1, "PUT", "/network-interfaces/eth0")
	if nic["host_dev_name"] != "fc0" || nic["mtu"] != float64(1500) || nic["guest_mac"] != vm.Networks[0].MAC {
		t.Errorf("network interface = %v", nic)
	}
	boot := f.body(1, "PUT", "/boot-source")
	if boot["kernel_image_path"] != "/vmlinux" || boot["initrd_path"] != nil || !strings.Contains(boot["boot_args"].(string), "console=ttyS0") {
		t.Errorf("boot source = %v", boot)
	}
	if v, _ := vm.Info("tap"); v != "[mega_tap0]" {
		t.Errorf("Info(tap) = %v", v)
	}
	if v, _ := vm.Info("console"); v != filepath.Join(*f_base, strconv.Itoa(vm.ID), "console.log") {
		t.Errorf("Info(console) = %v", v)
	}
	if v, _ := vm.Info("pid"); v == "0" {
		t.Errorf("Info(pid) = %v", v)
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from BUILDING = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	if err := vm.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	waitFCState(t, vm, VM_PAUSED)
	if err := vm.Stop(); err == nil {
		t.Errorf("Stop() from PAUSED = nil, want error")
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from PAUSED = %v", err)
	}
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from RUNNING = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	expect(1, append(append([]string{}, fcLaunchRequests...), "PUT /actions", "PATCH /vm", "PATCH /vm")...)
	if b := f.body(1, "PUT", "/actions"); b["action_type"] != "InstanceStart" {
		t.Errorf("action = %v", b)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	jailExists(false)

	// Relaunch from QUIT.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from QUIT = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)
	expect(2, append(append([]string{}, fcLaunchRequests...), "PUT /actions")...)
	jailExists(true)

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	jailExists(false)

	// Relaunch, then a guest kernel panic: exit status 0 gives ERROR.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from QUIT = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	fifo, err := os.OpenFile(filepath.Join(newFCJail(vm.ID, f.fcBin).Root, "console.fifo"), os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fifo.WriteString("Kernel panic - not syncing: VFS\n"); err != nil {
		t.Fatal(err)
	}
	fifo.Close()
	waitFileContent(t, filepath.Join(instanceDir, "console.log"), "Kernel panic - not syncing: VFS\n")

	if err := os.WriteFile(f.exitFile(3), nil, 0600); err != nil {
		t.Fatal(err)
	}
	waitFCState(t, vm, VM_ERROR)
	if got := vm.Tag("error"); got != "guest kernel panic" {
		t.Errorf("error tag = %q, want guest kernel panic", got)
	}
	jailExists(false)

	if _, err := os.Stat(filepath.Join(instanceDir, "console.log.1")); err != nil {
		t.Errorf("console.log was not rotated: %v", err)
	}

	// The relaunches reuse the disk, the netns, and the tap.
	for k, want := range map[string]int{"prepare-disk": 1, "create-netns": 1, "create-tap": 1} {
		if got := f.get(k); got != want {
			t.Errorf("%v calls = %d, want %d", k, got, want)
		}
	}

	id, err := fcStatSource(cfg.KernelPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fcCache.inUse(fcCacheKey(id)); got != 1 {
		t.Errorf("kernel cache users = %d, want 1", got)
	}

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}

	for k, want := range map[string]int{"release-disk": 1, "destroy-tap": 1, "destroy-netns": 1} {
		if got := f.get(k); got != want {
			t.Errorf("%v calls = %d, want %d", k, got, want)
		}
	}
	if got := fcCache.inUse(fcCacheKey(id)); got != 0 {
		t.Errorf("kernel cache users after flush = %d, want 0", got)
	}
	for _, p := range []string{instanceDir, vm.instancePath, jailDir} {
		if _, err := os.Lstat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%v exists after flush: %v", p, err)
		}
	}
}

func TestFCDriverKillBuilding(t *testing.T) {
	f := setupFCDriver(t)
	vm := f.newVM(t, f.config(t))

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestFCDriverErrors(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		f := setupFCDriver(t)
		cfg := f.config(t)
		cfg.KernelPath = ""
		vm := f.newVM(t, cfg)

		if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "need a kernel") {
			t.Fatalf("Launch() = %v, want kernel error", err)
		}
		waitFCState(t, vm, VM_ERROR)

		if err := vm.Flush(); err != nil {
			t.Fatalf("Flush() = %v", err)
		}
		if _, err := os.Lstat(vm.instancePath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("instance path exists after flush: %v", err)
		}
	})

	t.Run("instance-dir", func(t *testing.T) {
		f := setupFCDriver(t)
		vm := f.newVM(t, f.config(t))

		if err := os.MkdirAll(filepath.Dir(vm.instancePath), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), vm.instancePath); err != nil {
			t.Fatal(err)
		}

		if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "unable to create VM dir") {
			t.Fatalf("Launch() = %v, want an instance directory error", err)
		}
		if err := vm.Flush(); err != nil {
			t.Fatalf("Flush() = %v", err)
		}
		if _, err := os.Lstat(fcInstanceDir(vm.ID)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("instance directory exists after flush: %v", err)
		}
	})

	t.Run("restore", func(t *testing.T) {
		f := setupFCDriver(t)
		cfg := f.config(t)
		cfg.FirecrackerSnapshotPath = "/snap/vm"
		vm := f.newVM(t, cfg)

		if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "/snap/vm.json does not exist") {
			t.Fatalf("Launch() = %v, want missing sidecar error", err)
		}
		if err := vm.Flush(); err != nil {
			t.Fatalf("Flush() = %v", err)
		}
	})

	t.Run("early-exit", func(t *testing.T) {
		f := setupFCDriver(t)
		f.serve = false
		f.script = `echo "jailer: bad netns" >&2; exit 3`
		vm := f.newVM(t, f.config(t))

		err := vm.Launch()
		skipNoFIFO(t, err)
		if err == nil || !strings.Contains(err.Error(), "before its API socket") || !strings.Contains(err.Error(), "bad netns") {
			t.Fatalf("Launch() = %v, want early exit error", err)
		}
		waitFCState(t, vm, VM_ERROR)

		if err := vm.Flush(); err != nil {
			t.Fatalf("Flush() = %v", err)
		}
		if got := f.get("release-disk"); got != 1 {
			t.Errorf("release-disk calls = %d, want 1", got)
		}
	})
}

func TestFCDriverReadOnlyScratch(t *testing.T) {
	f := setupFCDriver(t)

	shared := filepath.Join(f.dir, "shared.raw")
	if err := os.WriteFile(shared, make([]byte, 4096), 0444); err != nil {
		t.Fatal(err)
	}
	fcPrepareDiskFn = func(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (fcDisk, error) {
		f.count("prepare-disk")
		return fcDisk{HostPath: shared, ReadOnly: dc.ReadOnly, CacheType: "Writeback"}, nil
	}

	cfg := f.config(t)
	cfg.Disks = DiskConfigs{{Path: filepath.Join(f.dir, "rootfs.squashfs"), ReadOnly: true}}
	cfg.FirecrackerScratch = "16M"
	vm := f.newVM(t, cfg)

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	cleanupFCVM(t, vm)
	waitFCState(t, vm, VM_BUILDING)

	want := []string{
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
	}
	if got := f.requests(1); !reflect.DeepEqual(got, want) {
		t.Fatalf("requests =\n%q\nwant\n%q", got, want)
	}

	root := f.body(1, "PUT", "/drives/disk0")
	if root["path_on_host"] != "/disk0" || root["is_root_device"] != true || root["is_read_only"] != true {
		t.Errorf("root drive = %v", root)
	}
	scratch := f.body(1, "PUT", "/drives/scratch")
	if scratch["drive_id"] != "scratch" || scratch["path_on_host"] != "/scratch" || scratch["is_root_device"] != false || scratch["is_read_only"] != false {
		t.Errorf("scratch drive = %v", scratch)
	}

	j := newFCJail(vm.ID, f.fcBin)
	instanceDir := fcInstanceDir(vm.ID)
	scratchFile := filepath.Join(instanceDir, "scratch.raw")

	sameFile := func(a, b string) bool {
		fa, err1 := os.Stat(a)
		fb, err2 := os.Stat(b)
		return err1 == nil && err2 == nil && os.SameFile(fa, fb)
	}
	if !sameFile(j.HostPath("/disk0"), shared) {
		t.Errorf("jail /disk0 is not a link to %v", shared)
	}
	if !sameFile(j.HostPath("/scratch"), scratchFile) {
		t.Errorf("jail /scratch is not a link to %v", scratchFile)
	}
	if fi, err := os.Stat(scratchFile); err != nil || fi.Size() != 16<<20 {
		t.Errorf("scratch disk: %v, %v, want 16 MiB", fi, err)
	}

	// The VMM log goes through a pipe in the jail to the instance directory.
	logFIFO := j.HostPath("/firecracker.log")
	if fi, err := os.Lstat(logFIFO); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		t.Fatalf("jail firecracker.log is not a named pipe: %v, %v", fi, err)
	}
	w, err := os.OpenFile(logFIFO, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("Running Firecracker\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	waitFileContent(t, filepath.Join(instanceDir, "firecracker.log"), "Running Firecracker\n")

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	// vm reset-disk makes the scratch disk empty again.
	sf, err := os.OpenFile(scratchFile, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sf.WriteAt([]byte("guest data"), 0); err != nil {
		t.Fatal(err)
	}
	sf.Close()

	if err := vm.ResetDisk(); err != nil {
		t.Fatalf("ResetDisk() = %v", err)
	}
	data, err := os.ReadFile(scratchFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 16<<20 || !fcAllZero(data) {
		t.Errorf("scratch disk after reset has %d bytes, all zero %v", len(data), fcAllZero(data))
	}
	for k, want := range map[string]int{"prepare-disk": 2, "release-disk": 2} {
		if got := f.get(k); got != want {
			t.Errorf("%v calls = %d, want %d", k, got, want)
		}
	}
}
