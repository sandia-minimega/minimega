// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package recovery

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeProc(t *testing.T, root, pid, comm string, args ...string) {
	t.Helper()

	dir := filepath.Join(root, pid)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	stat := pid + " (" + comm + ") S 1 1 1 0 -1 0 0 0 0 0 0 0 0 0 20 0 1 0 0 0 0\n"
	if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(stat), 0644); err != nil {
		t.Fatal(err)
	}

	cmdline := strings.Join(args, "\x00") + "\x00"
	if err := os.WriteFile(filepath.Join(dir, "cmdline"), []byte(cmdline), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestProcessesIgnoresFirecracker(t *testing.T) {
	root := t.TempDir()

	old := procRoot
	t.Cleanup(func() { procRoot = old })
	procRoot = root

	writeProc(t, root, "100", "qemu-system-x86", "qemu-system-x86_64", "-name", "7", "-uuid", "u7")
	writeProc(t, root, "200", "firecracker", "/firecracker", "--id", "vm8", "--api-sock", "/api.sock")
	writeProc(t, root, "201", "jailer", "jailer", "--id", "vm9", "--exec-file", "/usr/bin/firecracker")
	if err := os.MkdirAll(filepath.Join(root, "self"), 0755); err != nil {
		t.Fatal(err)
	}

	got, err := processes()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]QemuProcess{"u7": {PID: 100, UUID: "u7", VMID: "7"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("processes() = %v, want %v", got, want)
	}
}

func TestRecoverVMsFirecrackerQuit(t *testing.T) {
	root := t.TempDir()
	fcVM := filepath.Join(t.TempDir(), "vm", "3")

	old := procRoot
	t.Cleanup(func() { procRoot = old })
	procRoot = t.TempDir()

	if err := os.MkdirAll(fcVM, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fcVM, "state"), []byte("RUNNING\n"), 0644); err != nil {
		t.Fatal(err)
	}

	nsDir := filepath.Join(root, "namespaces", "minimega")
	if err := os.MkdirAll(nsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fcVM, filepath.Join(root, "3")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "3"), filepath.Join(nsDir, "u3")); err != nil {
		t.Fatal(err)
	}

	got, err := RecoverVMs(root)
	if err != nil {
		t.Fatal(err)
	}

	vms := got["minimega"]
	if len(vms) != 1 {
		t.Fatalf("RecoverVMs() = %v, want one VM", got)
	}
	if vms[0].State != "QUIT" || vms[0].PID != 0 || vms[0].UUID != "u3" {
		t.Errorf("RecoverVMs() VM = %+v, want state QUIT, no PID, UUID u3", vms[0])
	}

	resolved, err := filepath.EvalSymlinks(fcVM)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := filepath.EvalSymlinks(filepath.Join(root, vms[0].VMID)); err != nil || p != resolved {
		t.Errorf("VMID %q resolves to %q (%v), want %q", vms[0].VMID, p, err, resolved)
	}
}
