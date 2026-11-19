// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/sandia-minimega/minimega/v2/internal/qemu"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// qemuMachines returns the machine types of a QEMU binary. Tests replace it.
var qemuMachines = qemu.Machines

// machineNameRe matches the machine names that rewriteIncoming accepts. These
// names are safe in a printf format and in a shell word.
var machineNameRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// stateMachine decides the machine type for a launch from a state file.
// configured is the "vm config machine" value, inFile is the machine type in
// the state file, and available has the machine types of the QEMU binary.
//
// stateMachine never rejects a launch that QEMU can accept. QEMU resolves
// aliases and compares only the first len(inFile) bytes of the names, so when
// minimega cannot be sure, it returns configured with rewrite false. Then the
// launch does not change and QEMU does the check.
//
// When QEMU has inFile, stateMachine returns inFile if configured is empty or
// is the alias of its family ("pc" for pc-i440fx-*, "q35" for pc-q35-*).
//
// Otherwise it uses the upstream type for inFile from qemu.UpstreamMachine,
// with rewrite true: QEMU must receive the stream with the upstream name in
// its header. Then configured must be empty, the upstream type, or the alias
// of its family. It returns an error only when no machine type of QEMU starts
// with inFile, because then QEMU cannot load the state.
func stateMachine(configured, inFile string, available map[string]bool) (machine string, rewrite bool, err error) {
	if inFile == "" {
		return configured, false, nil
	}

	if available[inFile] {
		if configured == "" || familyAlias(configured, inFile) {
			return inFile, false, nil
		}
		return configured, false, nil
	}

	up, ok := qemu.UpstreamMachine(inFile)
	mapped := ok && available[up]
	if mapped && (configured == "" || configured == up || familyAlias(configured, up)) {
		return up, true, nil
	}

	for name := range available {
		if strings.HasPrefix(name, inFile) {
			return configured, false, nil
		}
	}

	switch {
	case mapped:
		return "", false, fmt.Errorf("vm config machine is %v, but the state file has machine type %v. Clear vm config machine or set it to %v", configured, inFile, up)
	case ok:
		return "", false, fmt.Errorf("state file has machine type %v. This QEMU does not have that machine type or the upstream machine type %v", inFile, up)
	}

	return "", false, fmt.Errorf("state file has machine type %v. This QEMU does not have that machine type", inFile)
}

// familyAlias reports whether alias is the QEMU alias for the family of
// machine.
func familyAlias(alias, machine string) bool {
	return (alias == "pc" && strings.HasPrefix(machine, "pc-i440fx-")) ||
		(alias == "q35" && strings.HasPrefix(machine, "pc-q35-"))
}

// catIncoming returns the -incoming value that reads a state file with no
// change.
func catIncoming(path string) string {
	return fmt.Sprintf("exec:cat %v", path)
}

// shellQuote quotes s as one word for /bin/sh.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// rewriteIncoming returns an -incoming value that gives QEMU the state file
// at path with the machine name oldName in its header replaced by newName.
// QEMU runs the command of an "exec:" value with /bin/sh -c. The command
// sends the first 9 bytes of the file, a new name length and name, and the
// file after the old name. It does not copy the file.
func rewriteIncoming(path, oldName, newName string) (string, error) {
	if len(oldName) == 0 || len(oldName) > qemu.MaxStateMachineLen {
		return "", fmt.Errorf("invalid machine name length: %d", len(oldName))
	}
	if len(newName) > qemu.MaxStateMachineLen || !machineNameRe.MatchString(newName) {
		return "", fmt.Errorf("invalid machine name for the state file header: %q", newName)
	}
	// launch joins the QEMU arguments and splits them again on spaces and
	// double quotes (applyQemuOverrides), so these characters do not survive.
	for i, r := range path {
		if r == '"' || r == 0 || (unicode.IsSpace(r) && r != ' ') || (r == ' ' && i > 0 && path[i-1] == ' ') {
			return "", fmt.Errorf("state file path %q contains a character that minimega cannot pass to QEMU: %q", path, r)
		}
	}

	var length strings.Builder
	for _, b := range []byte{0, 0, byte(len(newName) >> 8), byte(len(newName))} {
		fmt.Fprintf(&length, `\%03o`, b)
	}

	// head and tail count from 1, so skip the header and the old name
	skip := qemu.StateHeaderLen + len(oldName) + 1
	file := shellQuote(path)

	return fmt.Sprintf("exec:{ head -c 9 %v; printf '%v%v'; tail -c +%d %v; }",
		file, length.String(), newName, skip, file), nil
}

// replaceArgValue replaces the value after flag in args when that value is
// old. It returns an error when args do not have that pair.
func replaceArgValue(args []string, flag, old, new string) ([]string, error) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == old {
			res := append([]string(nil), args...)
			res[i+1] = new
			return res, nil
		}
	}

	return nil, fmt.Errorf("QEMU arguments do not contain %v %v", flag, old)
}

// appendedMachine returns the machine type that the last -M or -machine
// option in args sets, or "" when none sets a type. QEMU merges repeated
// options, and a later type replaces an earlier one.
func appendedMachine(args []string) string {
	var typ string
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "-M", "-machine", "--M", "--machine":
		default:
			continue
		}
		i++
		for j, kv := range strings.Split(args[i], ",") {
			switch {
			case strings.HasPrefix(kv, "type="):
				typ = strings.TrimPrefix(kv, "type=")
			case j == 0 && !strings.Contains(kv, "="):
				typ = kv
			}
		}
	}
	return typ
}

// pinStateMachine reads the machine type from the VM's state file and sets
// c.Machine to the machine type that the launch must use. c is a copy of the
// VM's configuration, so the configuration of the VM does not change. It
// returns the -incoming value when the stream header must change, or "" to
// use the state file with no change. When it changes the launch, it records
// the machine type in vm.pinnedMachine for "vm info". When qemu-append sets a
// machine type, QEMU uses that type, so the launch does not change.
//
// LOCK: Assumes the VM is locked.
func (vm *KvmVM) pinStateMachine(c *VMConfig) (string, error) {
	vm.pinnedMachine = ""

	if c.MigratePath == "" {
		return "", nil
	}
	if t := appendedMachine(c.QemuAppend); t != "" {
		log.Info("vm %v: qemu-append sets machine type %v, not using the state file machine type", vm.ID, t)
		return "", nil
	}

	f, err := os.Open(c.MigratePath)
	if err != nil {
		return "", fmt.Errorf("unable to open state file: %v", err)
	}
	inFile, err := qemu.StateMachine(f)
	f.Close()
	if err != nil {
		return "", fmt.Errorf("unable to read state file %v: %v", c.MigratePath, err)
	}
	if inFile == "" {
		return "", nil
	}

	available, err := qemuMachines(c.QemuPath)
	if err != nil {
		return "", err
	}

	machine, rewrite, err := stateMachine(c.Machine, inFile, available)
	if err != nil {
		return "", fmt.Errorf("state file %v: %v", c.MigratePath, err)
	}
	if machine == c.Machine && !rewrite {
		log.Debug("vm %v: state file %v has machine type %v and vm config machine is %q", vm.ID, c.MigratePath, inFile, c.Machine)
		return "", nil
	}

	var incoming string
	if rewrite {
		if incoming, err = rewriteIncoming(c.MigratePath, inFile, machine); err != nil {
			return "", err
		}
		log.Info("vm %v: QEMU does not have machine type %v from state file %v, using the upstream machine type %v", vm.ID, inFile, c.MigratePath, machine)
	}
	if machine != c.Machine {
		log.Info("vm %v: using machine type %v from state file %v, vm config machine is %q", vm.ID, machine, c.MigratePath, c.Machine)
	}

	c.Machine = machine
	vm.pinnedMachine = machine

	return incoming, nil
}
