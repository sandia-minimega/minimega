// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// kvmQMPRaw sends a raw QMP command to the QEMU process of vm. Tests replace
// it.
var kvmQMPRaw = func(vm *KvmVM, input string) (string, error) {
	return vm.q.Raw(input)
}

var memdumpCLIHandlers = []minicli.Handler{
	{ // vm memory-dump
		HelpShort: "write the guest memory of a VM to a file",
		HelpLong: `
Write the guest memory of a KVM VM to an ELF core file. The VM must be in the
running or paused state. The ELF core file has one PT_LOAD segment for each
range of guest physical memory.

A relative path is relative to the files directory (-filepath). minimega
creates the parent directories. The file must not exist.

The command returns when the dump starts. To see the progress of the dumps,
use vm memory-dump with no arguments. The output has the same columns and
status values as "vm save" with no arguments.

minimega sends the QMP command dump-guest-memory. QEMU stops the guest while
it writes the file, then continues the guest.

Other VM types do not support vm memory-dump.`,
		Patterns: []string{
			"vm memory-dump",
			"vm memory-dump <vm name> <file>",
		},
		Call:    wrapVMTargetCLI(cliVMMemoryDump),
		Suggest: wrapVMSuggest(VM_RUNNING|VM_PAUSED, false),
	},
}

// memoryDumper is a VM type that vm memory-dump supports.
type memoryDumper interface {
	MemoryDump(dst, format string) error
}

func cliVMMemoryDump(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target, ok := c.StringArgs["vm"]
	if !ok {
		rows, err := memoryDumpRows(ns)
		if err != nil {
			return err
		}

		resp.Header = []string{"id", "name", "status", "complete (%)"}
		resp.Tabular = rows
		return nil
	}

	found := ns.FindVM(target)
	if found == nil {
		return vmNotFound(target)
	}

	var dumper memoryDumper
	switch vm := found.(type) {
	case *KvmVM:
		dumper = vm
	default:
		return fmt.Errorf("vm memory-dump is not supported for %v VMs", found.GetType())
	}

	format := "elf"

	dst := c.StringArgs["file"]
	if !filepath.IsAbs(dst) {
		dst = filepath.Join(*f_iomBase, dst)
	}

	if _, err := os.Lstat(dst); err == nil {
		return fmt.Errorf("%v already exists", dst)
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	log.Info("dumping memory of VM %v to %v as %v", found.GetID(), dst, format)

	return dumper.MemoryDump(dst, format)
}

// memoryDumpRows returns one progress row for each memory dump of a VM in ns,
// in VM ID order. The rows come from query-dump.
func memoryDumpRows(ns *Namespace) ([][]string, error) {
	type row struct {
		id   int
		cols []string
	}

	var rows []row

	for _, vm := range ns.FindKvmVMs() {
		if vm.GetState()&(VM_RUNNING|VM_PAUSED) == 0 {
			continue
		}

		status, complete, err := vm.QueryDump()
		if err != nil {
			return nil, fmt.Errorf("vm %v: %v", vm.GetName(), err)
		}
		if status == "" {
			continue
		}

		rows = append(rows, row{
			id: vm.GetID(),
			cols: []string{
				strconv.Itoa(vm.GetID()),
				vm.GetName(),
				status,
				fmt.Sprintf("%.2f", complete),
			},
		})
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })

	var res [][]string
	for _, r := range rows {
		res = append(res, r.cols)
	}

	return res, nil
}

// MemoryDump starts a dump of the guest memory to dst with the QMP command
// dump-guest-memory. QEMU writes the dump in the background, and QueryDump
// reports its progress. KVM VMs support only the elf format.
func (vm *KvmVM) MemoryDump(dst, format string) error {
	if format != "elf" {
		return fmt.Errorf("%v memory dumps are not supported for kvm VMs", format)
	}

	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&(VM_RUNNING|VM_PAUSED) == 0 {
		return fmt.Errorf("vm %v is %v: vm memory-dump needs a vm in running or paused", vm.Name, vm.State)
	}

	cmd, err := json.Marshal(map[string]any{
		"execute": "dump-guest-memory",
		"arguments": map[string]any{
			"paging":   false,
			"protocol": "file:" + dst,
			"detach":   true,
			"format":   "elf",
		},
	})
	if err != nil {
		return err
	}

	if _, err := kvmQMPRaw(vm, string(cmd)); err != nil {
		return fmt.Errorf("dump-guest-memory: %v", err)
	}

	return nil
}

// QueryDump returns the status and the percent complete of the latest
// dump-guest-memory of the VM. The status is empty when the VM has no dump.
func (vm *KvmVM) QueryDump() (string, float64, error) {
	out, err := kvmQMPRaw(vm, `{"execute": "query-dump"}`)
	if err != nil {
		return "", 0, fmt.Errorf("query-dump: %v", err)
	}

	var r struct {
		Return struct {
			Status    string  `json:"status"`
			Completed float64 `json:"completed"`
			Total     float64 `json:"total"`
		} `json:"return"`
	}
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		return "", 0, fmt.Errorf("query-dump: %v", err)
	}

	switch s := r.Return.Status; s {
	case "", "none":
		return "", 0, nil
	case "completed":
		return s, 100, nil
	case "active":
		if r.Return.Total > 0 {
			return s, r.Return.Completed * 100 / r.Return.Total, nil
		}
		return s, 0, nil
	default:
		return s, 0, nil
	}
}
