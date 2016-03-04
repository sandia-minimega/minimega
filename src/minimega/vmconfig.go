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

// VMConfig contains all the configs possible for a VM. When a VM of a
// particular kind is launched, only the pertinent configuration is copied so
// fields from other configs will have the zero value for the field type.
type VMConfig struct {
	BaseConfig
	KVMConfig
	ContainerConfig
}

type VMConfigFns struct {
	Update   func(interface{}, *minicli.Command) error
	Clear    func(interface{}, *minicli.Command) error
	Print    func(interface{}) string
	PrintCLI func(interface{}) []string // If not specified, Print is used
}

func (old *VMConfig) Copy() *VMConfig {
	return &VMConfig{
		BaseConfig:      *old.BaseConfig.Copy(),
		KVMConfig:       *old.KVMConfig.Copy(),
		ContainerConfig: *old.ContainerConfig.Copy(),
	}
}

func (vm VMConfig) String() string {
	return vm.BaseConfig.String() + vm.KVMConfig.String() + vm.ContainerConfig.String()
}

func mustBaseConfig(val interface{}) *BaseConfig {
	if val, ok := val.(*BaseConfig); ok {
		return val
	}
	log.Fatal("`%#v` is not a BaseConfig", val)
	return nil
}

func mustKVMConfig(val interface{}) *KVMConfig {
	if val, ok := val.(*KVMConfig); ok {
		return val
	}
	log.Fatal("`%#v` is not a KVMConfig", val)
	return nil
}

func mustContainerConfig(val interface{}) *ContainerConfig {
	if val, ok := val.(*ContainerConfig); ok {
		return val
	}
	log.Fatal("`%#v` is not a ContainerConfig", val)
	return nil
}

// Functions for configuring VMs.
var baseConfigFns = map[string]VMConfigFns{
	"memory": vmConfigString(func(vm interface{}) *string {
		return &mustBaseConfig(vm).Memory
	}, VM_MEMORY_DEFAULT),
	"vcpus": vmConfigString(func(vm interface{}) *string {
		return &mustBaseConfig(vm).Vcpus
	}, "1"),
	"uuid": vmConfigString(func(vm interface{}) *string {
		return &mustBaseConfig(vm).UUID
	}, ""),
	"snapshot": vmConfigBool(func(vm interface{}) *bool {
		return &mustBaseConfig(vm).Snapshot
	}, true),
	"net": {
		Update: func(v interface{}, c *minicli.Command) error {
			vm := mustBaseConfig(v)

			// Reset any previously configured networks
			vm.Networks = []NetConfig{}

			for _, spec := range c.ListArgs["netspec"] {
				net, err := processVMNet(spec)
				if err != nil {
					return err
				}
				vm.Networks = append(vm.Networks, net)
			}

			return nil
		},
		Clear: func(vm interface{}, _ *minicli.Command) error {
			mustBaseConfig(vm).Networks = []NetConfig{}
			return nil
		},
		Print: func(vm interface{}) string {
			return mustBaseConfig(vm).NetworkString()
		},
		PrintCLI: func(v interface{}) []string {
			vm := mustBaseConfig(v)
			if len(vm.Networks) == 0 {
				return nil
			}

			nics := []string{}
			for _, net := range vm.Networks {
				nic := fmt.Sprintf("%v,%v,%v,%v", net.Bridge, net.VLAN, net.MAC, net.Driver)
				nics = append(nics, nic)
			}
			return []string{"vm config net " + strings.Join(nics, " ")}
		},
	},
	"tag": {
		// see cliVmConfigTag
		Update: nil,
		Print:  nil,
		Clear: func(v interface{}, c *minicli.Command) error {
			vm := mustBaseConfig(v)

			if c != nil && c.StringArgs["key"] != "" {
				delete(vm.Tags, c.StringArgs["key"])
			} else {
				vm.Tags = map[string]string{}
			}

			return nil
		},
		PrintCLI: func(v interface{}) []string {
			vm := mustBaseConfig(v)

			res := []string{}
			for k, v := range vm.Tags {
				res = append(res, fmt.Sprintf("vm config tag %q %q", k, v))
			}
			return res
		},
	},
}

// Functions for configuring container-based VMs. Note: if keys overlap with
// vmConfigFns, the functions in vmConfigFns take priority.
var containerConfigFns = map[string]VMConfigFns{
	"filesystem": vmConfigString(func(vm interface{}) *string {
		return &mustContainerConfig(vm).FSPath
	}, ""),
	"hostname": vmConfigString(func(vm interface{}) *string {
		return &mustContainerConfig(vm).Hostname
	}, ""),
	"init": {
		Update: func(v interface{}, c *minicli.Command) error {
			vm := mustContainerConfig(v)
			vm.Init = c.ListArgs["init"]
			return nil
		},
		Clear: func(vm interface{}, _ *minicli.Command) error {
			mustContainerConfig(vm).Init = []string{"/init"}

			return nil
		},
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", mustContainerConfig(vm).Init) },
	},
	"preinit": vmConfigString(func(vm interface{}) *string {
		return &mustContainerConfig(vm).Preinit
	}, ""),
	"fifo": vmConfigInt(func(vm interface{}) *int {
		return &mustContainerConfig(vm).Fifos
	}, "number", 0),
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
	"cpu": vmConfigString(func(vm interface{}) *string {
		return &mustKVMConfig(vm).CPU
	}, DefaultKVMCPU),
	"serial": vmConfigInt(func(vm interface{}) *int {
		return &mustKVMConfig(vm).SerialPorts
	}, "number", 0),
	"virtio-serial": vmConfigInt(func(vm interface{}) *int {
		return &mustKVMConfig(vm).VirtioPorts
	}, "number", 0),
	"qemu-append": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).QemuAppend
	}, "qemu-append", "kvm"),
	"disk": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).DiskPaths
	}, "disk", "kvm"),
	"append": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).Append
	}, "append", "kvm"),
	"qemu": {
		Update: func(_ interface{}, c *minicli.Command) error {
			customExternalProcesses["qemu"] = c.StringArgs["path"]
			return nil
		},
		Clear: func(_ interface{}, _ *minicli.Command) error {
			delete(customExternalProcesses, "qemu")
			return nil
		},
		Print: func(_ interface{}) string { return process("qemu") },
	},
	"qemu-override": {
		Update: func(_ interface{}, c *minicli.Command) error {
			m := c.StringArgs["match"]
			r := c.StringArgs["replacement"]

			return addVMQemuOverride(m, r)
		},
		Clear: func(_ interface{}, c *minicli.Command) error {
			if c != nil && c.StringArgs["id"] != "" {
				return delVMQemuOverride(c.StringArgs["id"])
			}

			return delVMQemuOverride(Wildcard)
		},
		Print: func(_ interface{}) string {
			return qemuOverrideString()
		},
		PrintCLI: func(_ interface{}) []string {
			res := []string{}
			for _, q := range QemuOverrides {
				res = append(res, fmt.Sprintf("vm kvm config qemu-override add %s %s", q.match, q.repl))
			}

			return res
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
		Clear: func(vm interface{}, _ *minicli.Command) error {
			*fn(vm) = defaultVal
			return nil
		},
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
		Clear: func(vm interface{}, _ *minicli.Command) error {
			*fn(vm) = defaultVal
			return nil
		},
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
		Clear: func(vm interface{}, _ *minicli.Command) error {
			*fn(vm) = defaultVal
			return nil
		},
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", *fn(vm)) },
	}
}

func vmConfigSlice(fn func(interface{}) *[]string, name, ns string) VMConfigFns {
	return VMConfigFns{
		Update: func(vm interface{}, c *minicli.Command) error {
			// Reset to empty list
			*fn(vm) = []string{}

			// Update the value, have to use range since we don't know the key
			for _, v := range c.ListArgs {
				*fn(vm) = append(*fn(vm), v...)
			}

			return nil
		},
		Clear: func(vm interface{}, _ *minicli.Command) error {
			*fn(vm) = []string{}
			return nil
		},
		Print: func(vm interface{}) string { return fmt.Sprintf("%v", *fn(vm)) },
		PrintCLI: func(vm interface{}) []string {
			if v := *fn(vm); len(v) > 0 {
				res := fmt.Sprintf("vm %s config %s %s", ns, name, strings.Join(v, " "))
				return []string{res}
			}
			return nil
		},
	}
}
