// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"strconv"
	"strings"
)

type VMConfigFns struct {
	Update   func(interface{}, *minicli.Command) error
	Clear    func(interface{})
	Print    func(interface{}) string
	PrintCLI func(interface{}) string // If not specified, Print is used
}

func mustVMConfig(vm interface{}) *VMConfig {
	if vm, ok := vm.(*VMConfig); ok {
		return vm
	}
	log.Fatal("`%#v` is not a VMConfig", vm)
	return nil
}

func mustKVMConfig(vm interface{}) *KVMConfig {
	if vm, ok := vm.(*KVMConfig); ok {
		return vm
	}
	log.Fatal("`%#v` is not a KVMConfig", vm)
	return nil
}

// Functions for configuring VMs.
var vmConfigFns = map[string]VMConfigFns{
	"memory": vmConfigString(func(vm interface{}) *string {
		return &mustVMConfig(vm).Memory
	}, VM_MEMORY_DEFAULT),
	"vcpus": vmConfigString(func(vm interface{}) *string {
		return &mustVMConfig(vm).Vcpus
	}, "1"),
	"net": {
		Update: func(v interface{}, c *minicli.Command) error {
			vm := mustVMConfig(v)
			for _, spec := range c.ListArgs["netspec"] {
				net, err := processVMNet(spec)
				if err != nil {
					return err
				}
				vm.Networks = append(vm.Networks, net)
			}
			return nil
		},
		Clear: func(vm interface{}) {
			mustVMConfig(vm).Networks = []NetConfig{}
		},
		Print: func(vm interface{}) string {
			return mustVMConfig(vm).NetworkString()
		},
		PrintCLI: func(v interface{}) string {
			vm := mustVMConfig(v)
			if len(vm.Networks) == 0 {
				return ""
			}

			nics := []string{}
			for _, net := range vm.Networks {
				nic := fmt.Sprintf("%v,%v,%v,%v", net.Bridge, net.VLAN, net.MAC, net.Driver)
				nics = append(nics, nic)
			}
			return "vm config net " + strings.Join(nics, " ")
		},
	},
}

// Functions for configuring KVM-based VMs. Note: if keys overlap with
// vmConfigFns, the functions in vmConfigFns take priority.
var kvmConfigFns = map[string]VMConfigFns{
	"cdrom": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).CdromPath
	}, ""),
	"initrd": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).InitrdPath
	}, ""),
	"kernel": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).KernelPath
	}, ""),
	"migrate": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).MigratePath
	}, ""),
	"uuid": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).UUID
	}, ""),
	"snapshot": vmConfigBool(func(vm interface{}) *bool {
		return &mustKVMConfig(vm).Snapshot
	}, true),
	"serial": vmConfigInt(func(vm interface{}) *int {
		return &mustKVMConfig(vm).SerialPorts
	}, "number", 1), // TODO: What should default be?
	"virtio-serial": vmConfigInt(func(vm interface{}) *int {
		return &mustKVMConfig(vm).VirtioPorts
	}, "number", 0), // TODO: What should default be?
	"qemu-append": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).QemuAppend
	}, "qemu-append", "kvm"),
	"disk": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).DiskPaths
	}, "disk", "kvm"),
	"append": {
		Update: func(vm interface{}, c *minicli.Command) error {
			mustKVMConfig(vm).Append = strings.Join(c.ListArgs["arg"], " ")
			return nil
		},
		Clear: func(vm interface{}) { mustKVMConfig(vm).Append = "" },
		Print: func(vm interface{}) string { return mustKVMConfig(vm).Append },
	},
	"qemu": {
		Update: func(_ interface{}, c *minicli.Command) error {
			customExternalProcesses["qemu"] = c.StringArgs["path"]
			return nil
		},
		Clear: func(_ interface{}) { delete(customExternalProcesses, "qemu") },
		Print: func(_ interface{}) string { return process("qemu") },
	},
	"qemu-override": {
		Update: func(_ interface{}, c *minicli.Command) error {
			if c.StringArgs["match"] != "" {
				return addVMQemuOverride(c.StringArgs["match"], c.StringArgs["replacement"])
			} else if c.StringArgs["id"] != "" {
				return delVMQemuOverride(c.StringArgs["id"])
			}

			log.Fatalln("someone goofed the qemu-override patterns")
			return nil
		},
		Clear: func(_ interface{}) { QemuOverrides = make(map[int]*qemuOverride) },
		Print: func(_ interface{}) string {
			return qemuOverrideString()
		},
		PrintCLI: func(_ interface{}) string {
			overrides := []string{}
			for _, q := range QemuOverrides {
				override := fmt.Sprintf("vm kvm config qemu-override add %s %s", q.match, q.repl)
				overrides = append(overrides, override)
			}
			return strings.Join(overrides, "\n")
		},
	},
}

func vmConfigString(fn func(interface{}) *string, defaultVal string) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			// Update the value, have to use range since we don't know the key
			for _, v := range c.StringArgs {
				*fn(vm) = v
			}
			return nil
		},
		Clear: func(vm interface{}) { *fn(vm) = defaultVal },
		Print: func(vm interface{}) string { return *fn(vm) },
	}
}

func vmConfigBool(fn func(interface{}) *bool, defaultVal bool) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			if c.BoolArgs["true"] || c.BoolArgs["false"] {
				*fn(vm) = c.BoolArgs["true"]
			} else {
				log.Fatalln("someone goofed on the patterns, bool args should be true/false")
			}
			return nil
		},
		Clear: func(vm interface{}) { *fn(vm) = defaultVal },
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", *fn(vm)) },
	}
}

func vmConfigInt(fn func(interface{}) *int, arg string, defaultVal int) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			v, err := strconv.Atoi(c.StringArgs[arg])
			if err != nil {
				return err
			}

			*fn(vm) = v
			return nil
		},
		Clear: func(vm interface{}) { *fn(vm) = defaultVal },
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", *fn(vm)) },
	}
}

func vmConfigSlice(fn func(interface{}) *[]string, name, ns string) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			// Update the value, have to use range since we don't know the key
			for _, v := range c.ListArgs {
				*fn(vm) = append(*fn(vm), v...)
			}
			return nil
		},
		Clear: func(vm interface{}) { *fn(vm) = []string{} },
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", *fn(vm)) },
		PrintCLI: func(vm interface{}) string {
			if v := *fn(vm); len(v) > 0 {
				return fmt.Sprintf("vm %s config %s %s", ns, name, strings.Join(v, " "))
			}
			return ""
		},
	}
}
