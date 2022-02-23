// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	"strconv"
)

// vmconfigCLIHandlers are special cases that are not worth generating via
// vmconfiger.
var vmconfigCLIHandlers = []minicli.Handler{
	{
		HelpShort: "configures vnc host",
		HelpLong: `Configure the hostname or ip address that hosts the rkvm vnc interface
Note: this configuration only applies to rkvm and must be specified.
`,
		Patterns: []string{
			"vm config vnc_host [value]",
		},

		Call: wrapSimpleCLI(func(ns *Namespace, c *minicli.Command, r *minicli.Response) error {
			if len(c.StringArgs) == 0 {
				r.Response = ns.vmConfig.Vnc_host
				return nil
			}

			v := c.StringArgs["value"]

			ns.vmConfig.Vnc_host = v

			return nil
		}),
	},
	{
		HelpShort: "configures vnc port",
		HelpLong: `Configure the port that hosts the rkvm vnc interface
Note: this configuration only applies to rkvm and must be specified.
`,
		Patterns: []string{
			"vm config vnc_port [value]",
		},

		Call: wrapSimpleCLI(func(ns *Namespace, c *minicli.Command, r *minicli.Response) error {
			if len(c.StringArgs) == 0 {
				r.Response = strconv.FormatInt(int64(ns.vmConfig.Vnc_port), 10)
				return nil
			}

			i, err := strconv.ParseInt(c.StringArgs["value"], 10, 64)
			if err != nil {
				return err
			}

			ns.vmConfig.Vnc_port = int(i)
			return nil
		}),
	},
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

Clone reparses the original network "vm config net". If the cloned VM was
configured with a static MAC, the VM config will not be launchable. Clone also
clears the UUID.

Calling clear vm config will clear all VM configuration options, but will not
remove saved configurations.`,
		Patterns: []string{
			"vm config",
			"vm config <save,> <name>",
			"vm config <restore,> [name]",
			"vm config <clone,> <vm name>",
		},
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
		Call:    wrapSimpleCLI(cliVMConfig),
	},
	{ // vm config disk
		HelpShort: "specify disks for VM",
		HelpLong: `Specify one or more disks to be connected to a VM. Any disk image supported by QEMU is a valid parameter.

Optionally, you may specify the drive interface for QEMU to use. By default,
"ide" is used. Supported interfaces are "ahci", "ide", "scsi", "sd", "mtd",
"floppy", "pflash", and "virtio".

Optionally, you may specify the cache mode to be used by the drive. By default,
"unsafe" is used for vms launched in snapshot mode, and "writeback" is used
otherwise. Supported cache modes are "none", "writeback", "unsafe",
"directsync", and "writethrough".

Note: although disk snapshot image files are saved in the temporary vm instance
paths, they may not be usable if the "unsafe" cache mode is used, as all flush
commands from the guest are ignored in that cache mode. For example, even if
you shut down the guest cleanly, there may still be data not yet written to the
snapshot image file. If you wish to copy and use the snapshot image file
cleanly, you can flush the disk cache manually via the QMP command socket, or
specify a different cache mode such as "writeback".

The order is:

	<path>,<interface>,<cache mode>

Examples:

To attach a disk with the default interface and cache mode:

	vm config disk linux_disk.qcow2

To attach 2 disks using the "ide" interface for the first disk and default
interface for the second disk:

	vm config disk linux_disk.qcow2,ide storage_disk.qcow2

To attach a disk using the "ide" interface with the "unsafe" cache mode:

	vm config disk linux_disk.qcow2,ide,unsafe

Disk images launched in snapshot mode may safely be used for multiple VMs.

Calling vm config disks with no arguments prints the current configuration.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config disks [diskspec]...",
		},
		Call: wrapSimpleCLI(cliVMConfigDisk),
	},
	{ // vm config net
		HelpShort: "specify network interfaces for VM",
		HelpLong: `
Specify the network(s) that the VM is a member of by VLAN. A corresponding VLAN
will be created for each network. Optionally, you may specify the bridge the
interface will be connected on. If the bridge name is omitted, minimega will
use the default "mega_bridge".

You can also optionally specify the MAC address of the interface to connect to
that network. If not specifed, the MAC address will be randomly generated.

Finally, you can also optionally specify a network device for qemu to use
(which is ignored by containers). By default, "e1000" is used. To see a list of
valid network devices, from run "qemu-kvm -device help".

The order is:

	<bridge>,<VLAN>,<MAC>,<driver>

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
			"vm config networks [netspec]...",
		},
		Call: wrapSimpleCLI(cliVMConfigNet),
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
	{ // clear vm config tag
		HelpShort: "remove tags for newly launched VMs",
		HelpLong: `
Remove tags in the same manner as "clear vm tag".`,
		Patterns: []string{
			"clear vm config tag <key>",
		},
		Call: wrapSimpleCLI(cliClearVMConfigTag),
	},
}

func cliVMConfig(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["save"] {
		// Save the current config
		ns.savedVMConfig[c.StringArgs["name"]] = ns.vmConfig.Copy()

		return nil
	} else if c.BoolArgs["restore"] {
		if name, ok := c.StringArgs["name"]; ok {
			// Try to restore an existing config
			if _, ok := ns.savedVMConfig[name]; !ok {
				return fmt.Errorf("config %v does not exist", name)
			}

			ns.vmConfig = ns.savedVMConfig[name].Copy()

			return nil
		} else if len(ns.savedVMConfig) == 0 {
			return errors.New("no vm configs saved")
		}

		// List the save configs
		for k := range ns.savedVMConfig {
			resp.Response += fmt.Sprintln(k)
		}

		return nil
	} else if c.BoolArgs["clone"] {
		// Clone the config of an existing vm, search across the namespace
		name := c.StringArgs["vm"]
		id, err := strconv.Atoi(name)

		var found VM

		for _, vm := range globalVMs(ns) {
			if err == nil && id == vm.GetID() {
				found = vm
			} else if name == vm.GetName() {
				found = vm
			}
		}

		if found == nil {
			return vmNotFound(name)
		}

		switch vm := found.(type) {
		case *KvmVM:
			ns.vmConfig.BaseConfig = vm.BaseConfig.Copy()
			ns.vmConfig.KVMConfig = vm.KVMConfig.Copy()

			// Clear SnapshotPaths since we can't launch VMs with the same SnapshotPath
			for i, _ := range ns.vmConfig.KVMConfig.Disks {
				ns.vmConfig.KVMConfig.Disks[i].SnapshotPath = ""
			}
		case *ContainerVM:
			ns.vmConfig.BaseConfig = vm.BaseConfig.Copy()
			ns.vmConfig.ContainerConfig = vm.ContainerConfig.Copy()
		case *RKvmVM:
			ns.vmConfig.BaseConfig = vm.BaseConfig.Copy()
			ns.vmConfig.RKVMConfig = vm.RKVMConfig.Copy()
		}

		// clear UUID since we can't launch VMs with the same UUID
		ns.vmConfig.UUID = ""

		// reprocess the network configs from their original input
		vals := []string{}
		for _, nic := range ns.vmConfig.Networks {
			vals = append(vals, nic.Raw)
		}

		return ns.processVMNets(vals)
	}

	// Print the config
	resp.Response = ns.vmConfig.String(ns.Name)
	return nil
}

func cliVMConfigDisk(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if len(c.ListArgs) == 0 {
		resp.Response = ns.vmConfig.DiskString(ns.Name)
		return nil
	}

	return ns.processVMDisks(c.ListArgs["diskspec"])
}

func cliVMConfigNet(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if len(c.ListArgs) == 0 {
		resp.Response = ns.vmConfig.NetworkString(ns.Name)
		return nil
	}

	return ns.processVMNets(c.ListArgs["netspec"])
}

func cliVMConfigQemuOverride(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if len(c.StringArgs) == 0 {
		resp.Response = ns.vmConfig.qemuOverrideString()
		return nil
	}

	ns.vmConfig.QemuOverride = append(ns.vmConfig.QemuOverride, qemuOverride{
		Match: c.StringArgs["match"],
		Repl:  c.StringArgs["replacement"],
	})

	return nil
}

func cliClearVMConfigTag(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if k := c.StringArgs["key"]; k == Wildcard {
		ns.vmConfig.Tags = nil
	} else {
		delete(ns.vmConfig.Tags, k)
	}

	return nil
}
