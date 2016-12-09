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
		Call: wrapSimpleCLI(cliVmConfig),
	},
	{ // vm config memory
		HelpShort: "set the amount of physical memory for a VM",
		HelpLong: `
Set the amount of physical memory to allocate in megabytes.`,
		Patterns: []string{
			"vm config memory [memory in megabytes]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "memory")
		}),
	},
	{ // vm config vcpus
		HelpShort: "set the number of virtual CPUs for a VM",
		HelpLong: `
Set the number of virtual CPUs to allocate for a VM.`,
		Patterns: []string{
			"vm config vcpus [number of CPUs]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "vcpus")
		}),
	},
	{ // vm config cpu
		HelpShort: "set the virtual CPU architecture",
		HelpLong: `
Set the virtual CPU architecture.

By default, set to 'host' which matches the host architecture. See 'kvm -cpu
help' for a list of architectures available for your version of kvm.`,
		Patterns: []string{
			"vm config cpu [cpu]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "cpu")
		}),
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
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "net")
		}),
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
		Call: wrapSimpleCLI(cliVmConfigTag),
	},
	{ // vm config append
		HelpShort: "set an append string to pass to a kernel set with vm kernel",
		HelpLong: `
Add an append string to a kernel set with vm kernel. Setting vm append without
using vm kernel will result in an error.

For example, to set a static IP for a linux VM:

	vm config append ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config append [arg]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "append")
		}),
	},
	{ // vm config qemu
		HelpShort: "set the QEMU process to invoke. Relative paths are ok.",
		HelpLong: `
Set the QEMU process to invoke. Relative paths are ok. When unspecified,
minimega uses "kvm" in the default path.


Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config qemu [path to qemu]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "qemu")
		}),
	},
	{ // vm config qemu-override
		HelpShort: "override parts of the QEMU launch string",
		HelpLong: `
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config qemu-override",
			"vm config qemu-override add <match> <replacement>",
			"vm config qemu-override delete <id or all>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "qemu-override")
		}),
	},
	{ // vm config qemu-append
		HelpShort: "add additional arguments to the QEMU command",
		HelpLong: `
Add additional arguments to be passed to the QEMU instance. For example:

	vm config qemu-append -serial tcp:localhost:4001

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config qemu-append [argument]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "qemu-append")
		}),
	},
	{ // vm config migrate
		HelpShort: "set migration image for a saved VM",
		HelpLong: `
Assign a migration image, generated by a previously saved VM to boot with. By
default, images are read from the files directory as specified with -filepath.
This can be overriden by using an absolute path.  Migration images should be
booted with a kernel/initrd, disk, or cdrom. Use 'vm migrate' to generate
migration images from running VMs.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config migrate [path to migration image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "migrate")
		}),
	},
	{ // vm config disk
		HelpShort: "set disk images to attach to a VM",
		HelpLong: `
Attach one or more disks to a vm. Any disk image supported by QEMU is a valid
parameter. Disk images launched in snapshot mode may safely be used for
multiple VMs.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config disk [path to disk image]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "disk")
		}),
	},
	{ // vm config cdrom
		HelpShort: "set a cdrom image to attach to a VM",
		HelpLong: `
Attach a cdrom to a VM. When using a cdrom, it will automatically be set to be
the boot device.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config cdrom [path to cdrom image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "cdrom")
		}),
	},
	{ // vm config kernel
		HelpShort: "set a kernel image to attach to a VM",
		HelpLong: `
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config kernel [path to kernel]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "kernel")
		}),
	},
	{ // vm config initrd
		HelpShort: "set a initrd image to attach to a VM",
		HelpLong: `
Attach an initrd image to a VM. Passed along with the kernel image at boot
time.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config initrd [path to initrd]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "initrd")
		}),
	},
	{ // vm config uuid
		HelpShort: "set the UUID for a VM",
		HelpLong: `
Set the UUID for a virtual machine. If not set, minimega will create a random
one when the VM is launched.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config uuid [uuid]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "uuid")
		}),
	},
	{ // vm config serial
		HelpShort: "specify the serial ports a VM will use",
		HelpLong: `
Specify the serial ports that will be created for the VM to use.
Serial ports specified will be mapped to the VM's /dev/ttySX device, where X
refers to the connected unix socket on the host at
$minimega_runtime/<vm_id>/serialX.

Examples:

To display current serial ports:
  vm config serial

To create three serial ports:
  vm config serial 3

Note: Whereas modern versions of Windows support up to 256 COM ports, Linux
typically only supports up to four serial devices. To use more, make sure to
pass "8250.n_uarts = 4" to the guest Linux kernel at boot. Replace 4 with
another number.`,
		Patterns: []string{
			"vm config serial [number of serial ports]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "serial")
		}),
	},
	{ // vm config virtio-serial
		HelpShort: "specify the virtio-serial ports a VM will use",
		HelpLong: `
Specify the virtio-serial ports that will be created for the VM to use.
Virtio-serial ports specified will be mapped to the VM's
/dev/virtio-port/<portname> device, where <portname> refers to the connected
unix socket on the host at $minimega_runtime/<vm_id>/virtio-serialX.

Examples:

To display current virtio-serial ports:
  vm config virtio-serial

To create three virtio-serial ports:
  vm config virtio-serial 3`,
		Patterns: []string{
			"vm config virtio-serial [number of virtio-serial ports]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "virtio-serial")
		}),
	},
	{ // vm config snapshot
		HelpShort: "enable or disable snapshot mode when using disk images",
		HelpLong: `
Enable or disable snapshot mode when using disk images. When enabled, disks
images will be loaded in memory when run and changes will not be saved. This
allows a single disk image to be used for many VMs.

Note: this configuration only applies to KVM-based VMs.`,
		Patterns: []string{
			"vm config snapshot [true,false]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "snapshot")
		}),
	},
	{ // vm config hostname
		HelpShort: "set a hostname for containers",
		HelpLong: `
Set a hostname for a container before launching the init program. If not set,
the hostname will be that of the physical host. The hostname can also be set by
the init program or other root process in the container.`,
		Patterns: []string{
			"vm config hostname [hostname]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "hostname")
		}),
	},
	{ // vm config init
		HelpShort: "container init program and args",
		HelpLong: `
Set the init program and args to exec into upon container launch. This will be
PID 1 in the container.`,
		Patterns: []string{
			"vm config init [init]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "init")
		}),
	},
	{ // vm config preinit
		HelpShort: "container preinit program",
		HelpLong: `
Containers start in a highly restricted environment. vm config preinit allows
running processes before isolation mechanisms are enabled. This occurs when the
vm is launched and before the vm is put in the building state. preinit
processes must finish before the vm will be allowed to start.

Specifically, the preinit command will be run after entering namespaces, and
mounting dependent filesystems, but before cgroups and root capabilities are
set, and before entering the chroot. This means that the preinit command is run
as root and can control the host.

For example, to run a script that enables ip forwarding, which is not allowed
during runtime because /proc is mounted read-only, add a preinit script:

	vm config preinit enable_ip_forwarding.sh`,
		Patterns: []string{
			"vm config preinit [preinit]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "preinit")
		}),
	},
	{ // vm config filesystem
		HelpShort: "set the filesystem for containers",
		HelpLong: `
Set the filesystem to use for launching a container. This should be a root
filesystem for a linux distribution (containing /dev, /proc, /sys, etc.)

This must be specified in order to launch a container.`,
		Patterns: []string{
			"vm config filesystem [filesystem]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "filesystem")
		}),
	},
	{ // vm config fifo
		HelpShort: "set the number of fifos for containers",
		HelpLong: `
Set the number of named pipes to include in the container for container-host
communication. Named pipes will appear on the host in the instance directory
for the container as fifoN, and on the container as /dev/fifos/fifoN.

Fifos are created using mkfifo() and have all of the same usage constraints.`,
		Patterns: []string{
			"vm config fifo [number]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command, resp *minicli.Response) error {
			return cliVmConfigField(c, resp, "fifo")
		}),
	},
	{ // clear vm config
		HelpShort: "reset vm config to the default value",
		HelpLong: `
Resets the configuration for a provided field (or the whole configuration) back
to the default value.`,
		// HACK: These patterns could be reduced to a single pattern with all
		// the different config fields as one multiple choice, however, to make
		// it easier to read, we split them into separate patterns. We could
		// use string literals for the field names but then we'd have to
		// process the Original string within the Command struct to figure out
		// what field we're supposed to clear. Instead, we can leverage the
		// magic of single-choice fields to set the field name in BoolArgs.
		Patterns: []string{
			"clear vm config",
			// VMConfig
			"clear vm config <cpu,>",
			"clear vm config <memory,>",
			"clear vm config <net,>",
			"clear vm config <vcpus,>",
			// KVMConfig
			"clear vm config <append,>",
			"clear vm config <cdrom,>",
			"clear vm config <migrate,>",
			"clear vm config <disk,>",
			"clear vm config <initrd,>",
			"clear vm config <kernel,>",
			"clear vm config <qemu,>",
			"clear vm config <qemu-append,>",
			"clear vm config <qemu-override,>",
			"clear vm config <snapshot,>",
			"clear vm config <uuid,>",
			"clear vm config <serial,>",
			"clear vm config <virtio-serial,>",
			// ContainerConfig
			"clear vm config <hostname,>",
			"clear vm config <filesystem,>",
			"clear vm config <init,>",
			"clear vm config <preinit,>",
		},
		Call: wrapSimpleCLI(cliClearVmConfig),
	},
	{ // clear vm config tag
		HelpShort: "remove tags for newly launched VMs",
		HelpLong: `
Remove tags in the same manner as "clear vm tag".`,
		Patterns: []string{
			"clear vm config tag [key]",
		},
		Call: wrapSimpleCLI(cliClearVmConfigTag),
	},
}

func cliVmConfig(c *minicli.Command, resp *minicli.Response) error {
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

func cliVmConfigField(c *minicli.Command, resp *minicli.Response, field string) error {
	// If there are no args it means that we want to display the current value
	nArgs := len(c.StringArgs) + len(c.ListArgs) + len(c.BoolArgs)

	var ok bool
	var fns VMConfigFns
	var config interface{}

	// Find the right config functions, baseConfigFns has highest priority
	if fns, ok = baseConfigFns[field]; ok {
		config = &vmConfig.BaseConfig
	} else if fns, ok = kvmConfigFns[field]; ok {
		config = &vmConfig.KVMConfig
	} else if fns, ok = containerConfigFns[field]; ok {
		config = &vmConfig.ContainerConfig
	} else {
		return fmt.Errorf("unknown config field: `%s`", field)
	}

	if nArgs == 0 {
		resp.Response = fns.Print(config)
		return nil
	}

	return fns.Update(config, c)
}

func cliVmConfigTag(c *minicli.Command, resp *minicli.Response) error {
	k := c.StringArgs["key"]

	if v, ok := c.StringArgs["value"]; ok {
		// Setting a new value
		vmConfig.Tags[k] = v
	} else if k != "" {
		// Printing a single tag
		resp.Response = vmConfig.Tags[k]
	} else {
		// Printing all configured tags
		resp.Response = vmConfig.TagsString()
	}

	return nil
}

func cliClearVmConfig(c *minicli.Command, resp *minicli.Response) error {
	var clearAll = len(c.BoolArgs) == 0
	var clearKVM = clearAll || (len(c.BoolArgs) == 1 && c.BoolArgs["kvm"])
	var clearContainer = clearAll || (len(c.BoolArgs) == 1 && c.BoolArgs["container"])
	var cleared bool

	for k, fns := range baseConfigFns {
		if clearAll || c.BoolArgs[k] {
			fns.Clear(&vmConfig.BaseConfig)
			cleared = true
		}
	}
	for k, fns := range kvmConfigFns {
		if clearKVM || c.BoolArgs[k] {
			fns.Clear(&vmConfig.KVMConfig)
			cleared = true
		}
	}
	for k, fns := range containerConfigFns {
		if clearContainer || c.BoolArgs[k] {
			fns.Clear(&vmConfig.ContainerConfig)
			cleared = true
		}
	}

	if !cleared {
		return errors.New("no callback defined for clear")
	}

	return nil
}

func cliClearVmConfigTag(c *minicli.Command, resp *minicli.Response) error {
	if k := c.StringArgs["key"]; k == "" || k == Wildcard {
		// Clearing all tags
		vmConfig.Tags = map[string]string{}
	} else {
		delete(vmConfig.Tags, k)
	}

	return nil
}
