// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCounterSkipPast(t *testing.T) {
	c := NewCounter()
	defer c.Stop()

	if got := c.Next(); got != 0 {
		t.Fatalf("Next() = %v, want 0", got)
	}

	c.SkipPast(5)
	if got := c.Next(); got != 6 {
		t.Fatalf("Next() after SkipPast(5) = %v, want 6", got)
	}

	c.SkipPast(2)
	if got := c.Next(); got != 7 {
		t.Fatalf("Next() after SkipPast(2) = %v, want 7", got)
	}

	c.SkipPast(7)
	if got := c.Next(); got != 8 {
		t.Fatalf("Next() after SkipPast(7) = %v, want 8", got)
	}
}

func TestCounterSkipPastAfterStop(t *testing.T) {
	c := NewCounter()
	c.Stop()

	done := make(chan struct{})
	go func() {
		c.SkipPast(5)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("SkipPast after Stop did not return")
	}
}

// writeRecoveryVM writes the files that a previous minimega instance kept for
// a VM in dir, and links the VM from the namespaces directory of base.
func writeRecoveryVM(t *testing.T, base, namespace, uuid, link, dir string, cfg VMConfig) {
	t.Helper()

	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.WriteConfig(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	for file, body := range map[string]string{"name": uuid, "state": "QUIT"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}

	nsDir := filepath.Join(base, "namespaces", namespace)
	if err := os.MkdirAll(nsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(link, filepath.Join(nsDir, uuid)); err != nil {
		t.Fatal(err)
	}
}

func TestRecoverSkipsPreviousVMIDs(t *testing.T) {
	const namespace = "recovery-test"

	fcDir := fcResolve(t.TempDir())
	setFCJailGlobals(t, fcDir)

	oldBase, oldVMID := *f_base, vmID
	vmID = NewCounter()
	t.Cleanup(func() {
		vmID.Stop()
		*f_base, vmID = oldBase, oldVMID
	})

	base := fcResolve(t.TempDir())
	*f_base = base

	ns := &Namespace{Name: namespace, VMs: VMs{m: map[int]VM{}}}
	namespaceLock.Lock()
	namespaces[namespace] = ns
	namespaceLock.Unlock()
	t.Cleanup(func() {
		namespaceLock.Lock()
		delete(namespaces, namespace)
		namespaceLock.Unlock()
	})

	kvmDir := filepath.Join(base, "3")
	writeRecoveryVM(t, base, namespace, "kvm-3", kvmDir, kvmDir, NewVMConfig())

	fcInst := filepath.Join(fcDir, "vm", "7")
	fcLink := filepath.Join(base, "7")
	if err := os.MkdirAll(fcInst, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fcInst, fcLink); err != nil {
		t.Fatal(err)
	}
	writeRecoveryVM(t, base, namespace, "fc-7", fcLink, fcInst, NewVMConfig())

	androidCfg := NewVMConfig()
	androidCfg.AndroidConfig.AVDName = "test-avd"
	androidDir := filepath.Join(base, "9")
	writeRecoveryVM(t, base, namespace, "android-9", androidDir, androidDir, androidCfg)

	if err := recover(); err != nil {
		t.Fatalf("recover() = %v, want nil", err)
	}

	if _, ok := ns.VMs.m[3]; !ok || len(ns.VMs.m) != 1 {
		t.Errorf("recovered %d VMs (VM 3 recovered: %v), want only VM 3", len(ns.VMs.m), ok)
	}

	vm, err := NewBaseVM("", namespace, NewVMConfig())
	if err != nil {
		t.Fatal(err)
	}
	vm.lock.Unlock()

	if vm.ID != 10 {
		t.Errorf("NewBaseVM() ID = %v, want 10", vm.ID)
	}
}
