package main

import (
	"testing"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestVMNetAddParseFailureKeepsVMConfigNetworks(t *testing.T) {
	vm := &nicVM{KvmVM: &KvmVM{BaseVM: &BaseVM{ID: 0, Name: "vm0", Type: KVM, instancePath: t.TempDir()}}}
	ns := &Namespace{Name: "t", VMs: VMs{m: map[int]VM{0: vm}}}
	ns.vmConfig.QemuPath = "no-such-qemu" // missing qemu is only a warning
	ns.vmConfig.Networks = NetConfigs{{Alias: "100", VLAN: 100}, {Alias: "101", VLAN: 101}}

	c := &minicli.Command{
		BoolArgs:   map[string]bool{"add": true},
		StringArgs: map[string]string{"vm": "vm0"},
		ListArgs:   map[string][]string{"netspec": {"a,b,c,d,e,f"}},
	}

	if err := cliVMNetMod(ns, c, &minicli.Response{}); err == nil {
		t.Fatal("expected malformed netspec error")
	}
	if got := len(ns.vmConfig.Networks); got != 2 {
		t.Errorf("vm config networks = %d entries after failed vm net add, want 2", got)
	}
}

func TestVMConfigParseFailureKeepsExistingConfig(t *testing.T) {
	ns := &Namespace{Name: "t"}
	ns.vmConfig.QemuPath = "no-such-qemu" // missing qemu is only a warning
	ns.vmConfig.Networks = NetConfigs{{Alias: "100", VLAN: 100, Raw: "100"}}
	ns.vmConfig.Bonds = BondConfigs{{Raw: "0,1"}}
	ns.vmConfig.Disks = DiskConfigs{{Path: "/a.qc2"}}

	cases := []struct {
		name string
		call func(*Namespace, *minicli.Command, *minicli.Response) error
		arg  string
		spec string
		got  func() int
	}{
		{"net", cliVMConfigNet, "netspec", "a,b,c,d,e,f", func() int { return len(ns.vmConfig.Networks) }},
		{"bonds", cliVMConfigBond, "bondspec", "x", func() int { return len(ns.vmConfig.Bonds) }},
		{"disk", cliVMConfigDisk, "diskspec", "/b.qc2,no-such-interface,x,y", func() int { return len(ns.vmConfig.Disks) }},
	}
	for _, tc := range cases {
		c := &minicli.Command{ListArgs: map[string][]string{tc.arg: {tc.spec}}}
		if err := tc.call(ns, c, &minicli.Response{}); err == nil {
			t.Errorf("vm config %s %s: expected parse error", tc.name, tc.spec)
		}
		if n := tc.got(); n != 1 {
			t.Errorf("vm config %s: %d entries after failed parse, want the existing 1", tc.name, n)
		}
	}
}
