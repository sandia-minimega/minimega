// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

// setMemDumpSeams restores the memory dump seams.
func setMemDumpSeams(t *testing.T) {
	t.Helper()

	oldQMP := kvmQMPRaw
	t.Cleanup(func() { kvmQMPRaw = oldQMP })
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

	if err := vm.MemoryDump("/x/k.raw", "raw"); err == nil || err.Error() != "raw memory dumps are not supported for kvm VMs" {
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
		9: &ContainerVM{BaseVM: &BaseVM{ID: 9, Name: "c", Type: CONTAINER}},
	}

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
	if want := [][]string{{"2", "k", "active", "25.00"}}; !reflect.DeepEqual(resp.Tabular, want) {
		t.Errorf("memory-dump rows = %q, want %q", resp.Tabular, want)
	}
	if len(sent) != 1 {
		t.Errorf("sent %d QMP commands, want 1 query-dump", len(sent))
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

	cmd = &minicli.Command{StringArgs: map[string]string{"vm": "c", "file": "c.elf"}, BoolArgs: map[string]bool{}}
	if err := cliVMMemoryDump(ns, cmd, &minicli.Response{}); err == nil || err.Error() != "vm memory-dump is not supported for container VMs" {
		t.Errorf("cliVMMemoryDump(container) = %v", err)
	}
}
