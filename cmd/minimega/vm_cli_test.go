package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestVMCdromEjectReportsError(t *testing.T) {
	ns := &Namespace{Name: "t", VMs: VMs{m: map[int]VM{
		0: &KvmVM{BaseVM: &BaseVM{ID: 0, Name: "vm0", Type: KVM}},
	}}}

	c := &minicli.Command{
		BoolArgs:   map[string]bool{"eject": true},
		StringArgs: map[string]string{"vm": "vm0"},
	}

	err := cliVMCdrom(ns, c, &minicli.Response{})
	if err == nil || !strings.Contains(err.Error(), "no cdrom inserted") {
		t.Fatalf("eject on VM with no cdrom: err = %v, want \"no cdrom inserted\"", err)
	}
}

// nicVM is a KVM VM whose AddNIC fails for the first call only.
type nicVM struct {
	*KvmVM
	calls int
}

func (v *nicVM) AddNIC(NetConfig) error {
	v.calls++
	if v.calls == 1 {
		return errors.New("first nic failed")
	}
	return nil
}

func TestVMNetAddReportsFirstNICFailure(t *testing.T) {
	vm := &nicVM{KvmVM: &KvmVM{BaseVM: &BaseVM{ID: 0, Name: "vm0", Type: KVM, instancePath: t.TempDir()}}}
	ns := &Namespace{Name: "t", VMs: VMs{m: map[int]VM{0: vm}}}
	ns.vmConfig.QemuPath = "no-such-qemu" // missing qemu is only a warning

	c := &minicli.Command{
		BoolArgs:   map[string]bool{"add": true},
		StringArgs: map[string]string{"vm": "vm0"},
		ListArgs:   map[string][]string{"netspec": {"100", "101"}},
	}

	err := cliVMNetMod(ns, c, &minicli.Response{})
	if err == nil || !strings.Contains(err.Error(), "first nic failed") {
		t.Fatalf("err = %v, want first nic failed", err)
	}
	if vm.calls != 1 {
		t.Errorf("AddNIC called %d times, want 1 (stop after failure)", vm.calls)
	}
}
