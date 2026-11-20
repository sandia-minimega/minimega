// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

const (
	mdMiB = uint64(1) << 20
	mdGiB = uint64(1) << 30
)

// setMemDumpSeams gives the test its own memory dump registry, sets the host
// architecture to amd64, and restores the memory dump seams.
func setMemDumpSeams(t *testing.T) {
	t.Helper()

	oldDumps, oldSnapshot, oldArch, oldQMP := fcMemDumps, fcMemDumpSnapshot, fcMemDumpArch, kvmQMPRaw
	t.Cleanup(func() {
		fcMemDumps, fcMemDumpSnapshot, fcMemDumpArch, kvmQMPRaw = oldDumps, oldSnapshot, oldArch, oldQMP
	})

	fcMemDumps = &fcExportRegistry{m: map[int]*fcExport{}}
	fcMemDumpArch = "amd64"
}

func waitFCMemDump(t *testing.T, id int, want string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		fcMemDumps.mu.Lock()
		e := fcMemDumps.m[id]
		fcMemDumps.mu.Unlock()

		var got string
		if e != nil {
			got, _ = e.get()
		}
		if got == want {
			return
		} else if time.Now().After(deadline) {
			t.Fatalf("memory dump status = %q, want %q", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestWriteELFCore(t *testing.T) {
	regions := []memRegion{
		{Start: 0, Size: 8192, Offset: 0},
		{Start: 4 * mdGiB, Size: 2*mdMiB + 4096, Offset: 8192},
	}

	var total uint64
	for _, r := range regions {
		total += r.Size
	}

	// Region 1 starts with an all-zero block, which becomes a hole.
	mem := make([]byte, total)
	for i := range mem[:8192] {
		mem[i] = byte(i%251 + 1)
	}
	tail := mem[8192+mdMiB:]
	for i := range tail {
		tail[i] = byte(i%13 + 7)
	}

	dir := t.TempDir()
	src := filepath.Join(dir, "mem")
	if err := os.WriteFile(src, mem, 0600); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(dir, "core")

	var last float64
	if err := fcMemDumpELF(src, dst, regions, func(p float64) { last = p }); err != nil {
		t.Fatalf("fcMemDumpELF() = %v", err)
	}
	if last != 100 {
		t.Errorf("last progress = %v, want 100", last)
	}

	f, err := elf.Open(dst)
	if err != nil {
		t.Fatalf("elf.Open() = %v", err)
	}
	defer f.Close()

	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Type != elf.ET_CORE || f.Machine != elf.EM_X86_64 {
		t.Errorf("header = %v %v %v %v, want ELFCLASS64 ELFDATA2LSB ET_CORE EM_X86_64", f.Class, f.Data, f.Type, f.Machine)
	}
	if len(f.Progs) != len(regions) {
		t.Fatalf("got %d program headers, want %d", len(f.Progs), len(regions))
	}

	for i, r := range regions {
		p := f.Progs[i]
		if p.Type != elf.PT_LOAD || p.Paddr != r.Start || p.Vaddr != r.Start || p.Filesz != r.Size || p.Memsz != r.Size {
			t.Errorf("segment %d = %+v, want region %+v", i, p.ProgHeader, r)
		}
		if p.Off%memDumpAlign != 0 {
			t.Errorf("segment %d offset %d is not aligned", i, p.Off)
		}

		want := mem[r.Offset : r.Offset+r.Size]
		data, err := io.ReadAll(p.Open())
		if err != nil || !bytes.Equal(data, want) {
			t.Errorf("segment %d data differs (%d bytes, %v)", i, len(data), err)
		}
	}

	fi, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	p := f.Progs[len(f.Progs)-1]
	if end := p.Off + p.Filesz; uint64(fi.Size()) != end {
		t.Errorf("file size = %d, want %d", fi.Size(), end)
	}

	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, mem[:4096], 0600); err != nil {
		t.Fatal(err)
	}
	if err := fcMemDumpELF(short, filepath.Join(dir, "core2"), regions, nil); err == nil {
		t.Errorf("fcMemDumpELF() with a short memory file = nil, want error")
	}
}

func TestKVMMemoryDump(t *testing.T) {
	setMemDumpSeams(t)

	var sent []string
	reply := `{"return": {}}`
	kvmQMPRaw = func(vm *KvmVM, input string) (string, error) {
		sent = append(sent, input)
		return reply, nil
	}

	vm := &KvmVM{BaseVM: &BaseVM{ID: 3, Name: "k", Type: KVM, State: VM_RUNNING}}

	if err := vm.MemoryDump("/x/k.elf", "elf"); err != nil {
		t.Fatalf("MemoryDump() = %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent %d QMP commands, want 1", len(sent))
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(sent[0]), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"execute": "dump-guest-memory",
		"arguments": map[string]any{
			"paging":   false,
			"protocol": "file:/x/k.elf",
			"detach":   true,
			"format":   "elf",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("QMP command = %v, want %v", got, want)
	}

	if err := vm.MemoryDump("/x/k.raw", "raw"); err == nil || err.Error() != "raw memory dumps are supported only for firecracker VMs" {
		t.Errorf("MemoryDump(raw) = %v", err)
	}

	vm.State = VM_QUIT
	if err := vm.MemoryDump("/x/k.elf", "elf"); err == nil || !strings.Contains(err.Error(), "needs a vm in running or paused") {
		t.Errorf("MemoryDump() in QUIT = %v, want state error", err)
	}
	if len(sent) != 1 {
		t.Errorf("sent %d QMP commands, want 1", len(sent))
	}

	for _, tc := range []struct {
		reply    string
		status   string
		complete float64
	}{
		{`{"return": {"status": "none", "completed": 0, "total": 0}}`, "", 0},
		{`{"return": {"status": "active", "completed": 25, "total": 100}}`, "active", 25},
		{`{"return": {"status": "completed", "completed": 100, "total": 100}}`, "completed", 100},
		{`{"return": {"status": "failed", "completed": 10, "total": 100}}`, "failed", 0},
	} {
		reply = tc.reply

		status, complete, err := vm.QueryDump()
		if err != nil || status != tc.status || complete != tc.complete {
			t.Errorf("QueryDump() for %v = %q, %v, %v, want %q, %v", tc.reply, status, complete, err, tc.status, tc.complete)
		}

		var q map[string]any
		if err := json.Unmarshal([]byte(sent[len(sent)-1]), &q); err != nil || q["execute"] != "query-dump" {
			t.Errorf("QMP command = %v, %v, want query-dump", q, err)
		}
	}
}

func TestCLIVMMemoryDump(t *testing.T) {
	setMemDumpSeams(t)

	oldIOMBase := *f_iomBase
	t.Cleanup(func() { *f_iomBase = oldIOMBase })
	*f_iomBase = t.TempDir()

	var sent []string
	kvmQMPRaw = func(vm *KvmVM, input string) (string, error) {
		sent = append(sent, input)
		if strings.Contains(input, "query-dump") {
			return `{"return": {"status": "active", "completed": 50, "total": 200}}`, nil
		}
		return `{"return": {}}`, nil
	}

	ns := &Namespace{Name: DefaultNamespace}
	ns.VMs.m = map[int]VM{
		2: &KvmVM{BaseVM: &BaseVM{ID: 2, Name: "k", Type: KVM, State: VM_RUNNING}},
		4: &KvmVM{BaseVM: &BaseVM{ID: 4, Name: "q", Type: KVM, State: VM_QUIT}},
		7: &FirecrackerVM{BaseVM: &BaseVM{ID: 7, Name: "fc7", Type: FIRECRACKER, State: VM_RUNNING}},
		9: &ContainerVM{BaseVM: &BaseVM{ID: 9, Name: "c", Type: CONTAINER}},
	}

	fcMemDumps.m[7] = &fcExport{ID: 7, Name: "fc7", Namespace: DefaultNamespace, status: fcExportCompleted, complete: 100}
	fcMemDumps.m[8] = &fcExport{ID: 8, Name: "gone", Namespace: DefaultNamespace, status: fcExportCompleted, complete: 100}
	fcMemDumps.m[11] = &fcExport{ID: 11, Name: "other", Namespace: "other", status: fcExportActive}

	empty := &minicli.Command{StringArgs: map[string]string{}, BoolArgs: map[string]bool{}}

	save := &minicli.Response{}
	if err := cliVMSave(&Namespace{Name: DefaultNamespace}, empty, save); err != nil {
		t.Fatalf("cliVMSave() = %v", err)
	}

	resp := &minicli.Response{}
	if err := cliVMMemoryDump(ns, empty, resp); err != nil {
		t.Fatalf("cliVMMemoryDump() = %v", err)
	}
	if !reflect.DeepEqual(resp.Header, save.Header) {
		t.Errorf("memory-dump header = %q, vm save header = %q", resp.Header, save.Header)
	}
	if want := [][]string{{"2", "k", "active", "25.00"}, {"7", "fc7", "completed", "100.00"}}; !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("memory-dump rows = %q, want %q", resp.Tabular, want)
	}
	if len(sent) != 1 {
		t.Errorf("sent %d QMP commands, want 1 query-dump", len(sent))
	}
	if _, ok := fcMemDumps.m[8]; ok {
		t.Errorf("the dump of a flushed VM is still in the registry")
	}
	if _, ok := fcMemDumps.m[11]; !ok {
		t.Errorf("the dump of a VM in another namespace was removed")
	}

	cmd := &minicli.Command{StringArgs: map[string]string{"vm": "k", "file": "dumps/k.elf"}, BoolArgs: map[string]bool{}}
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err != nil {
		t.Fatalf("cliVMMemoryDump(k) = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(sent[len(sent)-1]), &got); err != nil {
		t.Fatal(err)
	}
	args, _ := got["arguments"].(map[string]any)
	if want := "file:" + filepath.Join(*f_iomBase, "dumps", "k.elf"); args["protocol"] != want {
		t.Errorf("protocol = %v, want %v", args["protocol"], want)
	}
	if fi, err := os.Stat(filepath.Join(*f_iomBase, "dumps")); err != nil || !fi.IsDir() {
		t.Errorf("parent directory = %v, %v", fi, err)
	}

	if err := os.WriteFile(filepath.Join(*f_iomBase, "old.elf"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd.StringArgs["file"] = "old.elf"
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("cliVMMemoryDump() to an existing file = %v", err)
	}

	cmd.StringArgs["file"] = "k.raw"
	cmd.BoolArgs["raw"] = true
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err == nil || err.Error() != "raw memory dumps are supported only for firecracker VMs" {
		t.Errorf("cliVMMemoryDump(k raw) = %v", err)
	}

	if err := os.WriteFile(filepath.Join(*f_iomBase, "fc.raw.json"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd.StringArgs["vm"] = "fc7"
	cmd.StringArgs["file"] = "fc.raw"
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err == nil || !strings.Contains(err.Error(), "fc.raw.json already exists") {
		t.Errorf("cliVMMemoryDump(fc7 raw) with an existing JSON file = %v", err)
	}

	cmd = &minicli.Command{StringArgs: map[string]string{"vm": "c", "file": "c.elf"}, BoolArgs: map[string]bool{}}
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err == nil || err.Error() != "vm memory-dump is not supported for container VMs" {
		t.Errorf("cliVMMemoryDump(container) = %v", err)
	}
}
