// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"sort"
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
	Clear    func(interface{})
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
		Clear: func(vm interface{}) {
			mustBaseConfig(vm).Networks = []NetConfig{}
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
		// see also cliClearVmConfigTag
		Clear: func(v interface{}) {
			mustBaseConfig(v).Tags = map[string]string{}
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
		Clear: func(vm interface{}) {
			mustContainerConfig(vm).Init = []string{"/init"}
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
	}, "qemu-append"),
	"disk": vmConfigSlice(func(vm interface{}) *[]string {
		return &mustKVMConfig(vm).DiskPaths
	}, "disk"),
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
		PrintCLI: func(_ interface{}) []string {
			res := []string{}
			for _, q := range QemuOverrides {
				res = append(res, fmt.Sprintf("vm config qemu-override add %s %s", q.match, q.repl))
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

func vmConfigSlice(fn func(interface{}) *[]string, name string) VMConfigFns {
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
		Clear: func(vm interface{}) { *fn(vm) = []string{} },
		Print: func(vm interface{}) string {
			if v := *fn(vm); len(v) > 0 {
				return fmt.Sprintf("%v", v)
			}
			return ""
		},
		PrintCLI: func(vm interface{}) []string {
			if v := *fn(vm); len(v) > 0 {
				res := fmt.Sprintf("vm config %s %s", name, strings.Join(v, " "))
				return []string{res}
			}
			return nil
		},
	}
}

func saveConfig(fns map[string]VMConfigFns, configs interface{}) []string {
	var cmds = []string{}

	for k, fns := range fns {
		if fns.PrintCLI != nil {
			if v := fns.PrintCLI(configs); len(v) > 0 {
				cmds = append(cmds, v...)
			}
		} else if v := fns.Print(configs); len(v) > 0 {
			cmds = append(cmds, fmt.Sprintf("vm config %s %s", k, v))
		}
	}

	// Return in predictable order (nothing here should be order-sensitive)
	sort.Strings(cmds)

	return cmds
}
