// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
)

var (
	vmConfig  VMConfig                    // current vm config, updated by CLI
	savedInfo = make(map[string]VMConfig) // saved configs, may be reloaded
)

func init() {
	// set everything to defaults
	vmConfig.Clear(Wildcard)
}

// vmconfigCLIHandlers are special cases that are not worth generating via
// vmconfiger.
var vmconfigCLIHandlers = []minicli.Handler{
	{ // vm config
		HelpShort: "display, save, or restore the current VM configuration",
		HelpLong: `
Display, save, or restore the current VM configuration. Note that saving and
restoring configuration applies to all VM configurations including KVM-based VM
configurations.

To display the current configuration, call vm config with no arguments.

List the current saved configurations with 'vm config restore'.

To save a configuration:

	vm config save <config name>

To restore a configuration:

	vm config restore <config name>

To clone the configuration of an existing VM:

	vm config clone <vm name>

Calling clear vm config will clear all VM configuration options, but will not
remove saved configurations.`,
		Patterns: []string{
			"vm config",
			"vm config <save,> <name>",
			"vm config <restore,> [name]",
			"vm config <clone,> <vm name>",
		},
		Call: wrapSimpleCLI(cliVMConfig),
	},
	{ // vm config net
		HelpShort: "specific the networks a VM is a member of",
		HelpLong: `
Specify the network(s) that the VM is a member of by VLAN. A corresponding VLAN
will be created for each network. Optionally, you may specify the bridge the
interface will be connected on. If the bridge name is omitted, minimega will
use the default 'mega_bridge'.

You can also optionally specify the mac address of the interface to connect to
that network. If not specifed, the mac address will be randomly generated.

You can also optionally specify a network device for qemu to use (which is
ignored by containers). By default, e1000 is used. To see a list of valid
network devices, from run "qemu-kvm -device help".

Examples:

To connect a VM to VLANs 1 and 5:

	vm config net 1 5

To connect a VM to VLANs 100, 101, and 102 with specific mac addresses:

	vm config net 100,00:00:00:00:00:00 101,00:00:00:00:01:00 102,00:00:00:00:02:00

To connect a VM to VLAN 1 on bridge0 and VLAN 2 on bridge1:

	vm config net bridge0,1 bridge1,2

To connect a VM to VLAN 100 on bridge0 with a specific mac:

	vm config net bridge0,100,00:11:22:33:44:55

To specify a specific driver, such as i82559c:

	vm config net 100,i82559c

If you prefer, you can also use aliases for VLANs:

	vm config net DMZ CORE

These aliases will be allocated from the pool of available VLANs and is
namespace-aware (i.e. 'DMZ' in namespace 'foo' will be a different VLAN than
'DMZ' in namespace 'bar'). Internally, this is implemented by concatenating the
namespace name with the VLAN alias (e.g. 'DMZ' in namespace 'foo' becomes
'foo//DMZ'). If you wish to connect VLANs in different namespaces, you may
use/abuse this implementation detail:

	namespace bar
	vm config net foo//DMZ

Calling vm config net with no arguments prints the current configuration.`,
		Patterns: []string{
			"vm config net [netspec]...",
		},
		Call: wrapSimpleCLI(cliVMConfigNet),
	},
	{ // vm config tag
		HelpShort: "set tags for newly launched VMs",
		HelpLong: `
Set tags in the same manner as "vm tag". These tags will apply to all newly
launched VMs.`,
		Patterns: []string{
			"vm config tag [key]",
			"vm config tag <key> <value>",
		},
		Call: wrapSimpleCLI(cliVMConfigTag),
	},
	{ // vm config qemu-override
		HelpShort: "override parts of the QEMU launch string",
		HelpLong: `
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string. Overrides are applied in the order that they are defined
and do not replace earlier overrides -- if more than override share the same
"match" will later overrides will be applied to the overridden launch string.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config qemu-override",
			"vm config qemu-override <match> <replacement>",
		},
		Call: wrapSimpleCLI(cliVMConfigQemuOverride),
	},
}

func cliVMConfig(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["save"] {
		// Save the current config
		savedInfo[c.StringArgs["name"]] = vmConfig.Copy()

		return nil
	} else if c.BoolArgs["restore"] {
		if name, ok := c.StringArgs["name"]; ok {
			// Try to restore an existing config
			if _, ok := savedInfo[name]; !ok {
				return fmt.Errorf("config %v does not exist", name)
			}

			vmConfig = savedInfo[name].Copy()

			return nil
		} else if len(savedInfo) == 0 {
			return errors.New("no vm configs saved")
		}

		// List the save configs
		for k := range savedInfo {
			resp.Response += fmt.Sprintln(k)
		}

		return nil
	} else if c.BoolArgs["clone"] {
		// Clone the config of an existing vm
		vm := vms.FindVM(c.StringArgs["vm"])
		if vm == nil {
			return vmNotFound(c.StringArgs["vm"])
		}

		switch vm := vm.(type) {
		case *KvmVM:
			vmConfig.BaseConfig = vm.BaseConfig.Copy()
			vmConfig.KVMConfig = vm.KVMConfig.Copy()
		case *ContainerVM:
			vmConfig.BaseConfig = vm.BaseConfig.Copy()
			vmConfig.ContainerConfig = vm.ContainerConfig.Copy()
		}

		return nil
	}

	// Print the config
	resp.Response = vmConfig.String()
	return nil
}

func cliVMConfigNet(c *minicli.Command, resp *minicli.Response) error {
	if len(c.ListArgs) == 0 {
		resp.Response = vmConfig.NetworkString()
		return nil
	}

	vmConfig.Networks = nil

	for _, spec := range c.ListArgs["netspec"] {
		net, err := processVMNet(spec)
		if err != nil {
			vmConfig.Networks = nil
			return err
		}

		vmConfig.Networks = append(vmConfig.Networks, net)
	}

	return nil
}

func cliVMConfigTag(c *minicli.Command, resp *minicli.Response) error {
	k := c.StringArgs["key"]

	// if Tags were cleared, reinitialize them
	if vmConfig.Tags == nil {
		vmConfig.Tags = map[string]string{}
	}

	if v, ok := c.StringArgs["value"]; ok {
		// Setting a new value
		vmConfig.Tags[k] = v
	} else if k != "" {
		// Printing a single tag
		resp.Response = vmConfig.Tags[k]
	} else {
		// Printing all configured tags
		resp.Response = vmConfig.Tags.String()
	}

	return nil
}

func cliVMConfigQemuOverride(c *minicli.Command, resp *minicli.Response) error {
	if len(c.StringArgs) == 0 {
		resp.Response = vmConfig.qemuOverrideString()
		return nil
	}

	vmConfig.QemuOverride = append(vmConfig.QemuOverride, qemuOverride{
		Match: c.StringArgs["match"],
		Repl:  c.StringArgs["replacement"],
	})

	return nil
}
