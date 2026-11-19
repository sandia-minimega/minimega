// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func machineSet(names ...string) map[string]bool {
	res := map[string]bool{}
	for _, n := range names {
		res[n] = true
	}
	return res
}

func TestStateMachineDecision(t *testing.T) {
	upstream := machineSet("pc", "pc-i440fx-6.2", "pc-i440fx-6.1", "q35", "pc-q35-6.2", "pc-q35-6.1")
	jammy := machineSet("ubuntu", "pc-i440fx-jammy", "pc", "pc-i440fx-6.2", "pc-i440fx-6.1", "ubuntu-q35", "pc-q35-jammy", "q35", "pc-q35-6.2")
	newer := machineSet("pc", "pc-i440fx-11.1", "q35", "pc-q35-11.1")

	tests := []struct {
		name       string
		configured string
		inFile     string
		available  map[string]bool
		want       string
		rewrite    bool
		wantErr    string
	}{
		// no machine name in the state file: launch as configured
		{name: "no name, not configured", available: upstream, want: ""},
		{name: "no name, configured", configured: "q35", available: upstream, want: "q35"},
		{name: "no name, unknown configured", configured: "not-a-machine", available: nil, want: "not-a-machine"},

		// the state file's machine type is in this QEMU: pin it
		{name: "available, not configured", inFile: "pc-i440fx-6.1", available: upstream, want: "pc-i440fx-6.1"},
		{name: "available, pc alias", configured: "pc", inFile: "pc-i440fx-6.1", available: upstream, want: "pc-i440fx-6.1"},
		{name: "available, q35 alias", configured: "q35", inFile: "pc-q35-6.1", available: upstream, want: "pc-q35-6.1"},
		{name: "ubuntu name on ubuntu QEMU", inFile: "pc-i440fx-jammy", available: jammy, want: "pc-i440fx-jammy"},
		{name: "ubuntu name on ubuntu QEMU, pc alias", configured: "pc", inFile: "pc-i440fx-jammy", available: jammy, want: "pc-i440fx-jammy"},

		// the state file's machine type is in this QEMU, and another value is
		// configured: no pin, QEMU does the check
		{name: "available, same configured", configured: "pc-i440fx-6.1", inFile: "pc-i440fx-6.1", available: upstream, want: "pc-i440fx-6.1"},
		{name: "ubuntu name on ubuntu QEMU, same configured", configured: "pc-i440fx-jammy", inFile: "pc-i440fx-jammy", available: jammy, want: "pc-i440fx-jammy"},
		{name: "ubuntu alias", configured: "ubuntu", inFile: "pc-i440fx-jammy", available: jammy, want: "ubuntu"},
		{name: "virt alias", configured: "virt", inFile: "virt-8.2", available: machineSet("virt", "virt-8.2", "virt-8.1"), want: "virt"},
		{name: "longer name with the same prefix", configured: "pc-i440fx-jammy-hpb", inFile: "pc-i440fx-jammy", available: jammy, want: "pc-i440fx-jammy-hpb"},
		{name: "pc alias with q35 state", configured: "pc", inFile: "pc-q35-6.2", available: upstream, want: "pc"},
		{name: "q35 alias with i440fx state", configured: "q35", inFile: "pc-i440fx-6.1", available: upstream, want: "q35"},
		{name: "other version configured", configured: "pc-i440fx-6.2", inFile: "pc-i440fx-6.1", available: upstream, want: "pc-i440fx-6.2"},

		// the state file's machine type has a corresponding upstream type: pin and rewrite
		{name: "mapped, not configured", inFile: "pc-i440fx-jammy", available: upstream, want: "pc-i440fx-6.2", rewrite: true},
		{name: "mapped, upstream name configured", configured: "pc-i440fx-6.2", inFile: "pc-i440fx-jammy", available: upstream, want: "pc-i440fx-6.2", rewrite: true},
		{name: "mapped, pc alias", configured: "pc", inFile: "pc-i440fx-jammy", available: upstream, want: "pc-i440fx-6.2", rewrite: true},
		{name: "mapped q35", configured: "q35", inFile: "pc-q35-jammy", available: upstream, want: "pc-q35-6.2", rewrite: true},

		// not in this QEMU, but QEMU has a longer name with the same prefix
		{name: "prefix only, configured", configured: "pc-i440fx-focal-hpb", inFile: "pc-i440fx-focal", available: machineSet("pc-i440fx-focal-hpb"), want: "pc-i440fx-focal-hpb"},
		{name: "prefix only, not configured", inFile: "pc-i440fx-focal", available: machineSet("pc-i440fx-focal-hpb"), want: ""},
		// a mapped name comes before a longer name with the same prefix
		{name: "mapped, and a longer name", inFile: "pc-i440fx-noble", available: machineSet("pc-i440fx-noble-v2", "pc", "pc-i440fx-8.2"), want: "pc-i440fx-8.2", rewrite: true},

		// errors: QEMU cannot load the state
		{
			name: "unknown name", inFile: "pc-i440fx-focal", available: upstream,
			wantErr: "state file has machine type pc-i440fx-focal. This QEMU does not have that machine type",
		},
		{
			name: "unknown name, configured", configured: "pc", inFile: "pc-i440fx-focal", available: upstream,
			wantErr: "state file has machine type pc-i440fx-focal. This QEMU does not have that machine type",
		},
		{
			name: "hpb variant is not mapped", inFile: "pc-i440fx-jammy-hpb", available: upstream,
			wantErr: "state file has machine type pc-i440fx-jammy-hpb. This QEMU does not have that machine type",
		},
		{
			name: "mapped name not available", inFile: "pc-i440fx-jammy", available: newer,
			wantErr: "state file has machine type pc-i440fx-jammy. This QEMU does not have that machine type or the upstream machine type pc-i440fx-6.2",
		},
		{
			name: "mapped, file name configured", configured: "pc-i440fx-jammy", inFile: "pc-i440fx-jammy", available: upstream,
			wantErr: "vm config machine is pc-i440fx-jammy, but the state file has machine type pc-i440fx-jammy. Clear vm config machine or set it to pc-i440fx-6.2",
		},
		{
			name: "mapped, other version configured", configured: "pc-i440fx-6.1", inFile: "pc-i440fx-jammy", available: upstream,
			wantErr: "vm config machine is pc-i440fx-6.1, but the state file has machine type pc-i440fx-jammy. Clear vm config machine or set it to pc-i440fx-6.2",
		},
		{
			name: "mapped, pc alias with q35 state", configured: "pc", inFile: "pc-q35-jammy", available: upstream,
			wantErr: "vm config machine is pc, but the state file has machine type pc-q35-jammy. Clear vm config machine or set it to pc-q35-6.2",
		},
		{
			name: "mapped, ubuntu alias", configured: "ubuntu", inFile: "pc-i440fx-jammy", available: upstream,
			wantErr: "vm config machine is ubuntu, but the state file has machine type pc-i440fx-jammy. Clear vm config machine or set it to pc-i440fx-6.2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, rewrite, err := stateMachine(tt.configured, tt.inFile, tt.available)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("stateMachine() = %q, %v, %v; want error %q", got, rewrite, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("stateMachine() error: %v", err)
			}
			if got != tt.want || rewrite != tt.rewrite {
				t.Fatalf("stateMachine() = %q, %v; want %q, %v", got, rewrite, tt.want, tt.rewrite)
			}
		})
	}
}

// fakeState returns a migration stream header for name followed by size
// bytes of random data.
func fakeState(name string, size int) []byte {
	var b bytes.Buffer
	b.WriteString("QEVM")
	binary.Write(&b, binary.BigEndian, uint32(3))
	b.WriteByte(0x07)
	binary.Write(&b, binary.BigEndian, uint32(len(name)))
	b.WriteString(name)

	tail := make([]byte, size)
	rand.New(rand.NewSource(int64(size))).Read(tail)
	// bytes that a shell or printf could change
	copy(tail, "\x00\n%s\\'\"$(x)`\x00")
	b.Write(tail)

	return b.Bytes()
}

// referenceRewrite replaces the machine name in a migration stream header.
func referenceRewrite(state []byte, oldName, newName string) []byte {
	var b bytes.Buffer
	b.Write(state[:9])
	binary.Write(&b, binary.BigEndian, uint32(len(newName)))
	b.WriteString(newName)
	b.Write(state[13+len(oldName):])
	return b.Bytes()
}

func TestRewriteIncoming(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skipf("no /bin/sh: %v", err)
	}

	shells := []string{"/bin/sh"}
	for _, name := range []string{"dash", "bash"} {
		if p, err := exec.LookPath(name); err == nil {
			shells = append(shells, p)
		}
	}

	dir := filepath.Join(t.TempDir(), "saved it's here")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct{ oldName, newName string }{
		{"pc-i440fx-jammy", "pc-i440fx-6.2"},
		{"pc-q35-noble", "pc-q35-8.2"},
		{"pc-i440fx-6.2", "pc-i440fx-jammy"},
		{"pc-i440fx-jammy", strings.Repeat("m", 256)},
	} {
		path := filepath.Join(dir, "vm 'one'.state")
		state := fakeState(tt.oldName, 4096+len(tt.newName))
		if err := os.WriteFile(path, state, 0600); err != nil {
			t.Fatal(err)
		}
		want := referenceRewrite(state, tt.oldName, tt.newName)

		arg, err := rewriteIncoming(path, tt.oldName, tt.newName)
		if err != nil {
			t.Fatalf("rewriteIncoming(%q, %q): %v", tt.oldName, tt.newName, err)
		}

		// launch passes every argument through applyQemuOverrides
		args := VMConfig{}.applyQemuOverrides([]string{"-incoming", arg})
		if !reflect.DeepEqual(args, []string{"-incoming", arg}) {
			t.Fatalf("applyQemuOverrides changed the argument:\n got %q\nwant %q", args, arg)
		}

		cmd, ok := strings.CutPrefix(args[1], "exec:")
		if !ok {
			t.Fatalf("argument %q does not start with exec:", args[1])
		}
		for _, sh := range shells {
			out, err := exec.Command(sh, "-c", cmd).Output()
			if err != nil {
				t.Fatalf("%v -c %q: %v", sh, cmd, err)
			}
			if !bytes.Equal(out, want) {
				t.Errorf("%v: %v to %v: output differs from the reference (%d bytes, want %d)\nhead: %q\nwant: %q",
					sh, tt.oldName, tt.newName, len(out), len(want), out[:min(len(out), 32)], want[:32])
			}
		}
	}
}

func TestRewriteIncomingFormat(t *testing.T) {
	got, err := rewriteIncoming("/files/saved/vm1.state", "pc-i440fx-jammy", "pc-i440fx-6.2")
	if err != nil {
		t.Fatal(err)
	}
	want := `exec:{ head -c 9 '/files/saved/vm1.state'; printf '\000\000\000\015pc-i440fx-6.2'; tail -c +29 '/files/saved/vm1.state'; }`
	if got != want {
		t.Errorf("rewriteIncoming() =\n%v\nwant\n%v", got, want)
	}
}

func TestRewriteIncomingRejects(t *testing.T) {
	for _, tt := range []struct {
		path, oldName, newName, wantErr string
	}{
		{"/f/vm.state", "", "pc", "invalid machine name length: 0"},
		{"/f/vm.state", strings.Repeat("m", 257), "pc", "invalid machine name length: 257"},
		{"/f/vm.state", "pc-i440fx-jammy", "", `invalid machine name for the state file header: ""`},
		{"/f/vm.state", "pc-i440fx-jammy", "pc%s", `invalid machine name for the state file header: "pc%s"`},
		{"/f/vm.state", "pc-i440fx-jammy", `pc\n`, `invalid machine name for the state file header: "pc\\n"`},
		{"/f/vm.state", "pc-i440fx-jammy", "pc'x", `invalid machine name for the state file header: "pc'x"`},
		{"/f/vm.state", "pc-i440fx-jammy", "pc 6.2", `invalid machine name for the state file header: "pc 6.2"`},
		{"/f/vm.state", "pc-i440fx-jammy", strings.Repeat("m", 257), "invalid machine name for the state file header"},
		{`/f/"vm".state`, "pc-i440fx-jammy", "pc-i440fx-6.2", `contains a character that minimega cannot pass to QEMU: '"'`},
		{"/f/vm\t1.state", "pc-i440fx-jammy", "pc-i440fx-6.2", `contains a character that minimega cannot pass to QEMU: '\t'`},
		{"/f/vm\n1.state", "pc-i440fx-jammy", "pc-i440fx-6.2", `contains a character that minimega cannot pass to QEMU: '\n'`},
		{"/f/vm  1.state", "pc-i440fx-jammy", "pc-i440fx-6.2", `contains a character that minimega cannot pass to QEMU: ' '`},
		{"/f/vm\u00a01.state", "pc-i440fx-jammy", "pc-i440fx-6.2", `contains a character that minimega cannot pass to QEMU: '\u00a0'`},
	} {
		got, err := rewriteIncoming(tt.path, tt.oldName, tt.newName)
		if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
			t.Errorf("rewriteIncoming(%q, %q, %q) = %q, %v; want error %q", tt.path, tt.oldName, tt.newName, got, err, tt.wantErr)
		}
	}
}

func TestQemuArgsIncomingUnchanged(t *testing.T) {
	c := NewVMConfig()
	c.MigratePath = "/files/saved/vm1.state"
	if args := c.qemuArgs(1, "/tmp/minimega/1"); !hasArgSequence(args, "-incoming", "exec:cat /files/saved/vm1.state") {
		t.Errorf("missing -incoming exec:cat in %q", args)
	}
}

// TestLaunchPinsStateMachine launches a VM from a state file with a missing
// QEMU binary, so QEMU never starts. The launch must use the machine type
// from the state file and leave the VM's configuration as the user set it.
func TestLaunchPinsStateMachine(t *testing.T) {
	dir := t.TempDir()
	oldBase := *f_base
	*f_base = filepath.Join(dir, "base")
	defer func() { *f_base = oldBase }()

	available := machineSet("pc", "pc-i440fx-6.2", "q35", "pc-q35-6.2")
	oldMachines := qemuMachines
	qemuMachines = func(string) (map[string]bool, error) { return available, nil }
	defer func() { qemuMachines = oldMachines }()

	state := filepath.Join(dir, "vm1.state")
	if err := os.WriteFile(state, fakeState("pc-i440fx-jammy", 1024), 0600); err != nil {
		t.Fatal(err)
	}

	c := NewVMConfig()
	c.QemuPath = filepath.Join(dir, "no-such-qemu")
	c.MigratePath = state
	c.Machine = "pc"

	vm, err := NewKVM("restore", "test", c)
	if err != nil {
		t.Fatal(err)
	}

	// NewKVM returns the VM locked, and Launch unlocks it
	if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "unable to start qemu") {
		t.Fatalf("Launch() = %v, want an error from the missing QEMU binary", err)
	}

	if vm.KVMConfig.Machine != "pc" {
		t.Errorf("KVMConfig.Machine = %q after launch, want pc as configured", vm.KVMConfig.Machine)
	}
	var config bytes.Buffer
	if err := vm.KVMConfig.WriteConfig(&config); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config.String(), "pc-i440fx-6.2") {
		t.Errorf("saved configuration has the pinned machine type:\n%s", config.String())
	}
	if got, _ := vm.Info("machine"); got != "pc-i440fx-6.2" {
		t.Errorf(`Info("machine") = %q, want pc-i440fx-6.2`, got)
	}
	if got := vm.Copy().(*KvmVM); got.KVMConfig.Machine != "pc" || got.pinnedMachine != "pc-i440fx-6.2" {
		t.Errorf("Copy() machine = %q, pinned %q", got.KVMConfig.Machine, got.pinnedMachine)
	}

	vm.lock.Lock()
	args, err := vm.launchArgs()
	vm.lock.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgSequence(args, "-M", "pc-i440fx-6.2") {
		t.Errorf("missing -M pc-i440fx-6.2 in %q", args)
	}
	incoming, _ := rewriteIncoming(state, "pc-i440fx-jammy", "pc-i440fx-6.2")
	if !hasArgSequence(args, "-incoming", incoming) {
		t.Errorf("missing -incoming %q in %q", incoming, args)
	}

	// a relaunch decides again: this QEMU has the state file's machine type
	available = machineSet("pc", "pc-i440fx-jammy", "pc-i440fx-6.2")
	vm.lock.Lock()
	args, err = vm.launchArgs()
	vm.lock.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgSequence(args, "-M", "pc-i440fx-jammy") || !hasArgSequence(args, "-incoming", "exec:cat "+state) {
		t.Errorf("relaunch arguments %q, want -M pc-i440fx-jammy and exec:cat", args)
	}
	if got, _ := vm.Info("machine"); got != "pc-i440fx-jammy" {
		t.Errorf(`Info("machine") after relaunch = %q, want pc-i440fx-jammy`, got)
	}

	// a relaunch that fails the decision clears the old machine type
	available = machineSet("pc", "pc-i440fx-11.1")
	vm.lock.Lock()
	_, err = vm.launchArgs()
	vm.lock.Unlock()
	if err == nil || !strings.Contains(err.Error(), "does not have that machine type") {
		t.Errorf("launchArgs() = %v, want a missing machine type error", err)
	}
	if got, _ := vm.Info("machine"); got != "pc" {
		t.Errorf(`Info("machine") after a failed decision = %q, want pc`, got)
	}

	// another alias that QEMU resolves: no pin, the launch is as configured
	available = machineSet("ubuntu", "pc", "pc-i440fx-jammy", "pc-i440fx-6.2")
	vm.lock.Lock()
	if _, err := vm.launchArgs(); err != nil || vm.pinnedMachine != "pc-i440fx-jammy" {
		t.Errorf("launch with pc: pinned %q, error %v", vm.pinnedMachine, err)
	}
	vm.KVMConfig.Machine = "ubuntu"
	args, err = vm.launchArgs()
	vm.lock.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgSequence(args, "-M", "ubuntu") || !hasArgSequence(args, "-incoming", "exec:cat "+state) {
		t.Errorf("arguments %q, want -M ubuntu and exec:cat", args)
	}
	if got, _ := vm.Info("machine"); got != "ubuntu" {
		t.Errorf(`Info("machine") with no pin = %q, want ubuntu`, got)
	}
}

// TestLaunchStateFileError launches a VM with a missing state file. The launch
// must fail before it starts QEMU and put the VM in the error state.
func TestLaunchStateFileError(t *testing.T) {
	dir := t.TempDir()
	oldBase := *f_base
	*f_base = filepath.Join(dir, "base")
	defer func() { *f_base = oldBase }()

	oldMachines := qemuMachines
	qemuMachines = func(string) (map[string]bool, error) {
		t.Error("launch with a missing state file asked QEMU for its machine types")
		return nil, nil
	}
	defer func() { qemuMachines = oldMachines }()

	c := NewVMConfig()
	c.QemuPath = filepath.Join(dir, "no-such-qemu")
	c.MigratePath = filepath.Join(dir, "missing.state")

	vm, err := NewKVM("restore", "test", c)
	if err != nil {
		t.Fatal(err)
	}

	// NewKVM returns the VM locked, and Launch unlocks it
	err = vm.Launch()
	if err == nil || !strings.Contains(err.Error(), "unable to launch VM: unable to open state file:") {
		t.Fatalf("Launch() = %v, want an error from the missing state file", err)
	}
	if !strings.Contains(err.Error(), "missing.state") {
		t.Errorf("Launch() = %v, want the path of the state file", err)
	}
	if got := vm.GetState(); got != VM_ERROR {
		t.Errorf("state after launch = %v, want %v", got, VM_ERROR)
	}
}

func TestAppendedMachine(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{
		{nil, ""},
		{[]string{"-serial", "tcp:localhost:4001"}, ""},
		{[]string{"-M", "q35"}, "q35"},
		{[]string{"-machine", "q35,smm=on"}, "q35"},
		{[]string{"-machine", "type=q35,smm=on"}, "q35"},
		{[]string{"-machine", "smm=on,type=pc"}, "pc"},
		{[]string{"--machine", "pc-q35-8.2"}, "pc-q35-8.2"},
		{[]string{"-machine", "smm=on"}, ""},
		{[]string{"-append", "console=ttyS0 -M x"}, ""},
		{[]string{"-drive", "file=a -M b"}, ""},
		{[]string{"-M", "q35", "-machine", "pc,smm=on"}, "pc"},
		{[]string{"-M", "q35", "-machine", "smm=on"}, "q35"},
		{[]string{"-machine"}, ""},
		{[]string{"-smbios", "type=1"}, ""},
	} {
		if got := appendedMachine(tt.args); got != tt.want {
			t.Errorf("appendedMachine(%q) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

// TestLaunchArgsAppendedMachine launches from a state file when qemu-append
// has a -machine option. QEMU uses the last machine type in its arguments, so
// minimega must not pin a type when qemu-append sets one.
func TestLaunchArgsAppendedMachine(t *testing.T) {
	oldMachines := qemuMachines
	qemuMachines = func(string) (map[string]bool, error) { return machineSet("q35", "pc-q35-6.2"), nil }
	defer func() { qemuMachines = oldMachines }()

	state := filepath.Join(t.TempDir(), "vm1.state")
	if err := os.WriteFile(state, fakeState("pc-q35-6.2", 1024), 0600); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		appended []string
		pinned   string
	}{
		{[]string{"-machine", "q35,smm=on"}, ""},
		{[]string{"-machine", "smm=on"}, "pc-q35-6.2"},
	} {
		c := NewVMConfig()
		c.MigratePath = state
		c.QemuAppend = tt.appended
		vm := &KvmVM{BaseVM: &BaseVM{BaseConfig: c.BaseConfig}, KVMConfig: c.KVMConfig}

		args, err := vm.launchArgs()
		if err != nil {
			t.Fatal(err)
		}
		if vm.pinnedMachine != tt.pinned {
			t.Errorf("qemu-append %q: pinned %q, want %q", tt.appended, vm.pinnedMachine, tt.pinned)
		}
		if got, want := hasArgSequence(args, "-M", "pc-q35-6.2"), tt.pinned != ""; got != want {
			t.Errorf("qemu-append %q: arguments %q have -M pc-q35-6.2: %v, want %v", tt.appended, args, got, want)
		}
		if !hasArgSequence(args, tt.appended...) || !hasArgSequence(args, "-incoming", "exec:cat "+state) {
			t.Errorf("qemu-append %q: arguments %q, want the appended arguments and exec:cat", tt.appended, args)
		}
		if got, _ := vm.Info("machine"); got != tt.pinned {
			t.Errorf(`qemu-append %q: Info("machine") = %q, want %q`, tt.appended, got, tt.pinned)
		}
	}
}

func TestLaunchArgsWithoutState(t *testing.T) {
	oldMachines := qemuMachines
	qemuMachines = func(string) (map[string]bool, error) {
		t.Error("launch without a state file asked QEMU for its machine types")
		return nil, nil
	}
	defer func() { qemuMachines = oldMachines }()

	c := NewVMConfig()
	c.Machine = "q35"
	vm := &KvmVM{BaseVM: &BaseVM{BaseConfig: c.BaseConfig}, KVMConfig: c.KVMConfig}

	args, err := vm.launchArgs()
	if err != nil {
		t.Fatal(err)
	}
	if !hasArgSequence(args, "-M", "q35") || hasArgSequence(args, "-incoming") {
		t.Errorf("arguments %q, want -M q35 and no -incoming", args)
	}
	if got, _ := vm.Info("machine"); got != "q35" {
		t.Errorf(`Info("machine") = %q, want q35`, got)
	}
}

func TestLaunchArgsStateWithoutMachine(t *testing.T) {
	oldMachines := qemuMachines
	qemuMachines = func(string) (map[string]bool, error) {
		t.Error("state file without a machine name asked QEMU for its machine types")
		return nil, nil
	}
	defer func() { qemuMachines = oldMachines }()

	// a stream from a machine type that sends no configuration section
	state := filepath.Join(t.TempDir(), "old.state")
	if err := os.WriteFile(state, []byte("QEVM\x00\x00\x00\x03\x01rest"), 0600); err != nil {
		t.Fatal(err)
	}

	c := NewVMConfig()
	c.MigratePath = state
	vm := &KvmVM{BaseVM: &BaseVM{BaseConfig: c.BaseConfig}, KVMConfig: c.KVMConfig}

	args, err := vm.launchArgs()
	if err != nil {
		t.Fatal(err)
	}
	if hasArgSequence(args, "-M") || !hasArgSequence(args, "-incoming", "exec:cat "+state) {
		t.Errorf("arguments %q, want no -M and exec:cat", args)
	}

	// a missing state file is a launch error
	vm.MigratePath = state + ".missing"
	if _, err := vm.launchArgs(); err == nil || !strings.Contains(err.Error(), "unable to open state file") {
		t.Errorf("launchArgs() with a missing state file = %v", err)
	}
}
