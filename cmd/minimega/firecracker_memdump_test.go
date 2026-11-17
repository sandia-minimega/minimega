// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestFCMemoryRegions checks the layout against Firecracker v1.17.0.
// arch_memory_regions (src/vmm/src/arch/x86_64/mod.rs:116-159) calls
// arch_memory_regions_with_gap (src/vmm/src/arch/mod.rs:164-189) for two gaps:
// MMIO32_MEM_START = 3 GiB (layout.rs:100) with MMIO32_MEM_SIZE = 1 GiB
// (layout.rs:98), and MMIO64_MEM_START = 256 GiB (layout.rs:124) with
// MMIO64_MEM_SIZE = 256 GiB (layout.rs:126). The snapshot memory file holds
// the regions back to back (src/vmm/src/vstate/memory.rs:892-907 for the
// offsets, :1093-1106 for the dump).
func TestFCMemoryRegions(t *testing.T) {
	for _, tc := range []struct {
		mib  uint64
		want []memRegion
	}{
		{128, []memRegion{{0, 128 * mdMiB, 0}}},
		{2048, []memRegion{{0, 2 * mdGiB, 0}}},
		{3072, []memRegion{{0, 3 * mdGiB, 0}}},
		{3073, []memRegion{{0, 3 * mdGiB, 0}, {4 * mdGiB, mdMiB, 3 * mdGiB}}},
		{6144, []memRegion{{0, 3 * mdGiB, 0}, {4 * mdGiB, 3 * mdGiB, 3 * mdGiB}}},
		{255 * 1024, []memRegion{{0, 3 * mdGiB, 0}, {4 * mdGiB, 252 * mdGiB, 3 * mdGiB}}},
		{256 * 1024, []memRegion{{0, 3 * mdGiB, 0}, {4 * mdGiB, 252 * mdGiB, 3 * mdGiB}, {512 * mdGiB, mdGiB, 255 * mdGiB}}},
		{767 * 1024, []memRegion{{0, 3 * mdGiB, 0}, {4 * mdGiB, 252 * mdGiB, 3 * mdGiB}, {512 * mdGiB, 512 * mdGiB, 255 * mdGiB}}},
	} {
		got, err := fcMemoryRegions(tc.mib)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("fcMemoryRegions(%d) = %v, %v, want %v", tc.mib, got, err, tc.want)
		}

		var total uint64
		for _, r := range got {
			total += r.Size
		}
		if total != tc.mib<<20 {
			t.Errorf("fcMemoryRegions(%d) holds %d bytes, want %d", tc.mib, total, tc.mib<<20)
		}
	}

	for _, mib := range []uint64{0, 767*1024 + 1} {
		if got, err := fcMemoryRegions(mib); err == nil {
			t.Errorf("fcMemoryRegions(%d) = %v, want error", mib, got)
		}
	}
}

// fcFakeSnapshot returns a fcMemDumpSnapshot that sends the request to the
// fake API server, then writes mem as the memory file in the jail.
func fcFakeSnapshot(mem []byte) func(context.Context, *fcClient, string, string) error {
	return func(ctx context.Context, c *fcClient, statePath, memPath string) error {
		if err := c.CreateSnapshot(ctx, statePath, memPath); err != nil {
			return err
		}

		root := filepath.Dir(c.socket)
		if err := os.WriteFile(filepath.Join(root, statePath), []byte("state"), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(root, memPath), mem, 0600)
	}
}

// checkFCMemELF checks that path is an ELF core file with one PT_LOAD segment
// at guest physical address 0 that holds mem.
func checkFCMemELF(t *testing.T, path string, mem []byte) {
	t.Helper()

	f, err := elf.Open(path)
	if err != nil {
		t.Fatalf("open %v: %v", path, err)
	}
	defer f.Close()

	if f.Type != elf.ET_CORE || f.Machine != elf.EM_X86_64 || len(f.Progs) != 1 {
		t.Fatalf("%v: type %v, machine %v, %d program headers", path, f.Type, f.Machine, len(f.Progs))
	}

	p := f.Progs[0]
	if p.Type != elf.PT_LOAD || p.Paddr != 0 || p.Vaddr != 0 || p.Filesz != uint64(len(mem)) {
		t.Errorf("%v: segment %+v", path, p.ProgHeader)
	}

	data, err := io.ReadAll(p.Open())
	if err != nil || !bytes.Equal(data, mem) {
		t.Errorf("%v: segment data differs (%d bytes, %v)", path, len(data), err)
	}
}

func TestFCMemoryDump(t *testing.T) {
	f := setupFCDriver(t)
	setMemDumpSeams(t)

	mem := make([]byte, 4*mdMiB)
	copy(mem, bytes.Repeat([]byte("a"), 4096))
	copy(mem[2*mdMiB:], "b")

	fcMemDumpSnapshot = fcFakeSnapshot(mem)

	vm := f.newVM(t, f.config(t))
	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	cleanupFCVM(t, vm)

	vm.lock.Lock()
	vm.Memory = 4
	vm.VCPUs = 2
	out := vm.jail.HostPath("/out")
	vm.lock.Unlock()

	dst := filepath.Join(f.dir, "dump.elf")
	if err := vm.MemoryDump(dst, "elf"); err == nil || !strings.Contains(err.Error(), "needs a vm in running or paused") {
		t.Errorf("MemoryDump() in BUILDING = %v, want state error", err)
	}

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, vm, VM_RUNNING)

	// elf from a running VM: pause, snapshot, resume.
	before := len(f.requests(1))
	if err := vm.MemoryDump(dst, "elf"); err != nil {
		t.Fatalf("MemoryDump() = %v", err)
	}
	waitFCMemDump(t, vm.ID, fcExportCompleted)

	if got, want := f.requests(1)[before:], []string{"PATCH /vm", "PUT /snapshot/create", "PATCH /vm"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("requests = %q, want %q", got, want)
	}
	if got := f.reqAt(1, before).Body["state"]; got != "Paused" {
		t.Errorf("first request state = %v, want Paused", got)
	}
	wantBody := map[string]any{
		"snapshot_type":       "Full",
		"snapshot_path":       "/out/dump.state",
		"mem_file_path":       "/out/dump.mem",
		"sync_snapshot_files": false,
	}
	if got := f.reqAt(1, before+1).Body; !reflect.DeepEqual(got, wantBody) {
		t.Errorf("snapshot request = %v, want %v", got, wantBody)
	}
	if got := f.reqAt(1, before+2).Body["state"]; got != "Resumed" {
		t.Errorf("last request state = %v, want Resumed", got)
	}

	if got := vm.GetState(); got != VM_RUNNING {
		t.Errorf("state = %v, want RUNNING", got)
	}

	checkFCMemELF(t, dst, mem)

	if m, _ := filepath.Glob(filepath.Join(fcInstanceDir(vm.ID), "memdump-*")); len(m) != 0 {
		t.Errorf("memory files in the instance directory: %q", m)
	}
	if ents, err := os.ReadDir(out); err != nil || len(ents) != 0 {
		t.Errorf("jail /out = %v, %v, want empty", ents, err)
	}

	wantRows := [][]string{{strconv.Itoa(vm.ID), "fc1", "completed", "100.00"}}
	if got := fcMemDumps.table(DefaultNamespace); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("table = %q, want %q", got, wantRows)
	}

	// raw: the memory file itself and a JSON file with the region map.
	raw := filepath.Join(f.dir, "dump.raw")
	before = len(f.requests(1))
	if err := vm.MemoryDump(raw, "raw"); err != nil {
		t.Fatalf("MemoryDump(raw) = %v", err)
	}
	waitFCMemDump(t, vm.ID, fcExportCompleted)

	if got, want := f.requests(1)[before:], []string{"GET /version", "PATCH /vm", "PUT /snapshot/create", "PATCH /vm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("raw requests = %q, want %q", got, want)
	}
	if data, err := os.ReadFile(raw); err != nil || !bytes.Equal(data, mem) {
		t.Errorf("raw dump = %d bytes, %v", len(data), err)
	}

	data, err := os.ReadFile(raw + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var info fcMemDumpInfo
	if err := json.Unmarshal(data, &info); err != nil {
		t.Fatal(err)
	}
	wantInfo := fcMemDumpInfo{Format: "raw", Arch: "x86_64", FirecrackerVersion: "1.17.0", VCPUs: 2, MemoryMiB: 4}
	wantInfo.Regions = []memRegion{{Start: 0, Size: 4 * mdMiB, Offset: 0}}
	if !reflect.DeepEqual(info, wantInfo) {
		t.Errorf("raw JSON = %+v, want %+v", info, wantInfo)
	}

	// elf from a paused VM: no pause or resume.
	if err := vm.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}

	paused := filepath.Join(f.dir, "paused.elf")
	before = len(f.requests(1))
	if err := vm.MemoryDump(paused, "elf"); err != nil {
		t.Fatalf("MemoryDump() of a paused VM = %v", err)
	}
	waitFCMemDump(t, vm.ID, fcExportCompleted)

	if got, want := f.requests(1)[before:], []string{"PUT /snapshot/create"}; !reflect.DeepEqual(got, want) {
		t.Errorf("paused requests = %q, want %q", got, want)
	}
	if got := vm.GetState(); got != VM_PAUSED {
		t.Errorf("state = %v, want PAUSED", got)
	}
	checkFCMemELF(t, paused, mem)

	if err := vm.Start(); err != nil {
		t.Fatalf("Start() from PAUSED = %v", err)
	}

	// A failed snapshot still resumes the VM and keeps the previous row.
	fcMemDumpSnapshot = func(ctx context.Context, c *fcClient, statePath, memPath string) error {
		return errors.New("disk full")
	}

	before = len(f.requests(1))
	if err := vm.MemoryDump(filepath.Join(f.dir, "fail.elf"), "elf"); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("MemoryDump() with a failed snapshot = %v", err)
	}
	if got, want := f.requests(1)[before:], []string{"PATCH /vm", "PATCH /vm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("failed snapshot requests = %q, want %q", got, want)
	}
	if got := vm.GetState(); got != VM_RUNNING {
		t.Errorf("state = %v, want RUNNING", got)
	}
	if got := fcMemDumps.table(DefaultNamespace); !reflect.DeepEqual(got, wantRows) {
		t.Errorf("table after a failed snapshot = %q, want %q", got, wantRows)
	}

	// A memory file that does not match the layout fails the dump.
	fcMemDumpSnapshot = fcFakeSnapshot(mem[:mdMiB])

	short := filepath.Join(f.dir, "short.elf")
	if err := vm.MemoryDump(short, "elf"); err != nil {
		t.Fatalf("MemoryDump() = %v", err)
	}
	waitFCMemDump(t, vm.ID, fcExportFailed)

	if _, err := os.Stat(short); !os.IsNotExist(err) {
		t.Errorf("destination of a failed dump: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(fcInstanceDir(vm.ID), "memdump-*")); len(m) != 0 {
		t.Errorf("memory files in the instance directory after a failed dump: %q", m)
	}

	fcMemDumpArch = "arm64"
	if err := vm.MemoryDump(filepath.Join(f.dir, "arm.elf"), "elf"); err == nil || !strings.Contains(err.Error(), "x86_64 host") {
		t.Errorf("MemoryDump() on arm64 = %v", err)
	}
}
