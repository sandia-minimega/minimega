// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

// fcFakeCC records the ron calls of one Firecracker VM. ListenUnixBound
// creates a placeholder file at the path, and CloseUnix records whether the
// path still exists, so a test can see that the listener closes before the
// jail is removed.
type fcFakeCC struct {
	t  *testing.T
	vm *FirecrackerVM

	mu    sync.Mutex
	calls []string
}

func (c *fcFakeCC) record(format string, args ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, fmt.Sprintf(format, args...))
}

// take returns the calls since the last take.
func (c *fcFakeCC) take() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := c.calls
	c.calls = nil
	return res
}

func (c *fcFakeCC) checkUnlocked(op string) {
	if !c.vm.lock.TryLock() {
		c.t.Errorf("%v called with vm.lock held", op)
		return
	}
	c.vm.lock.Unlock()
}

func (c *fcFakeCC) RegisterVM(vm ron.VM) {
	c.checkUnlocked("RegisterVM")
	c.record("register %v", vm.GetUUID())
}

func (c *fcFakeCC) UnregisterVM(vm ron.VM) {
	c.checkUnlocked("UnregisterVM")
	c.record("unregister %v", vm.GetUUID())
}

func (c *fcFakeCC) ListenUnixBound(path, uuid string, uid int) error {
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%v is in use", path)
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		return err
	}

	c.record("listen %v %v %v", path, uuid, uid)
	return nil
}

func (c *fcFakeCC) CloseUnix(path string) error {
	_, err := os.Lstat(path)
	c.record("close %v %v", path, err == nil)
	os.Remove(path)
	return nil
}

func expectCCCalls(t *testing.T, cc *fcFakeCC, want ...string) {
	t.Helper()

	if got := cc.take(); !reflect.DeepEqual(got, want) {
		t.Fatalf("cc calls =\n%q\nwant\n%q", got, want)
	}
}

func TestFCDriverCC(t *testing.T) {
	f := setupFCDriver(t)
	vm := f.newVM(t, f.config(t))
	cc := &fcFakeCC{t: t, vm: vm}

	path := newFCJail(vm.ID, f.fcBin).CCSocket()
	uuid := vm.UUID
	uid := fcVMUID(vm.ID)

	if want := filepath.Join(newFCJail(vm.ID, f.fcBin).Root, "v.sock_9002"); path != want {
		t.Fatalf("CCSocket() = %v, want %v", path, want)
	}

	register := "register " + uuid
	listen := fmt.Sprintf("listen %v %v %v", path, uuid, uid)
	closed := fmt.Sprintf("close %v true", path)
	unregister := "unregister " + uuid

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	waitFCState(t, vm, VM_BUILDING)

	// The namespace calls Connect after the first Start.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	expectCCCalls(t, cc)

	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("connectCC() = %v", err)
	}
	expectCCCalls(t, cc, register, listen)

	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("second connectCC() = %v", err)
	}
	expectCCCalls(t, cc)

	// The VMM exit closes the listener before the jail is removed.
	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	expectCCCalls(t, cc, closed)

	// A relaunch creates the listener before InstanceStart.
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from QUIT = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)
	expectCCCalls(t, cc, listen)

	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("connectCC() after relaunch = %v", err)
	}
	expectCCCalls(t, cc)

	// Disconnect closes the listener, then unregisters the VM.
	if err := vm.disconnectCC(cc); err != nil {
		t.Fatalf("disconnectCC() = %v", err)
	}
	expectCCCalls(t, cc, closed, unregister)

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	expectCCCalls(t, cc)

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
	expectCCCalls(t, cc)
}

func TestFCDriverCCFlush(t *testing.T) {
	f := setupFCDriver(t)
	vm := f.newVM(t, f.config(t))
	cc := &fcFakeCC{t: t, vm: vm}

	path := newFCJail(vm.ID, f.fcBin).CCSocket()

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("connectCC() = %v", err)
	}
	cc.take()

	// Flush closes a listener that is still open, before it removes the jail.
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
	expectCCCalls(t, cc, fmt.Sprintf("close %v true", path))
}

func TestFCDriverCCNoBackchannel(t *testing.T) {
	f := setupFCDriver(t)
	cfg := f.config(t)
	cfg.Backchannel = false
	vm := f.newVM(t, cfg)
	cc := &fcFakeCC{t: t, vm: vm}

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if err := vm.connectCC(cc); err != nil {
		t.Fatalf("connectCC() = %v", err)
	}
	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)
	if err := vm.disconnectCC(cc); err != nil {
		t.Fatalf("disconnectCC() = %v", err)
	}
	expectCCCalls(t, cc)

	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestFCCCSocketPathCheck(t *testing.T) {
	f := setupFCDriver(t)
	vm := f.newVM(t, f.config(t))

	// Leave room for api.sock, but not for v.sock_9002.
	j := newFCJail(vm.ID, f.fcBin)
	n := fcMaxSocketPath - len(j.HostPath("/api.sock")) - 2
	if n < 1 {
		t.Fatalf("test directory %v is too long", f.dir)
	}
	*f_firecrackerDir = filepath.Join(f.dir, strings.Repeat("d", n))
	if err := os.MkdirAll(*f_firecrackerDir, 0700); err != nil {
		t.Fatal(err)
	}

	j = newFCJail(vm.ID, f.fcBin)
	if err := fcCheckSocketPath(j.HostPath("/api.sock")); err != nil {
		t.Fatalf("api.sock path: %v", err)
	}
	if err := fcCheckSocketPath(j.CCSocket()); err == nil {
		t.Fatalf("cc socket path %v: no error", j.CCSocket())
	}

	err := vm.Launch()
	if err == nil || !strings.Contains(err.Error(), "v.sock_9002") {
		t.Fatalf("Launch() = %v, want a socket path error for v.sock_9002", err)
	}
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestFCResetOOMScore(t *testing.T) {
	old := fcProcDir
	t.Cleanup(func() { fcProcDir = old })
	fcProcDir = t.TempDir()

	p := filepath.Join(fcProcDir, "42", "oom_score_adj")
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("-1000\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := fcResetOOMScore(42); err != nil {
		t.Fatalf("fcResetOOMScore(42) = %v", err)
	}
	if data, _ := os.ReadFile(p); string(data) != "0" {
		t.Errorf("oom_score_adj = %q, want 0", data)
	}

	err := fcResetOOMScore(43)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("fcResetOOMScore(43) = %v, want a not exist error", err)
	}
	if _, err := os.Stat(filepath.Join(fcProcDir, "43")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("fcResetOOMScore(43) created a file: %v", err)
	}
}
