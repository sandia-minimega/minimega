// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"math"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"ranges"
	"strconv"
	"strings"
)

var vmCLIHandlers = []minicli.Handler{
	{ // vm info
		HelpShort: "print information about VMs",
		HelpLong: `
Print information about VMs in tabular form. The .filter and .columns commands
can be used to subselect a set of rows and/or columns. See the help pages for
.filter and .columns, respectively, for their usage. Columns returned by VM
info include:

- host	    : the host that the VM is running on
- id	    : the VM ID, as an integer
- name	    : the VM name, if it exists
- state     : one of (building, running, paused, quit, error)
- type 		: one of (kvm)
- vcpus     : the number of allocated CPUs
- memory    : allocated memory, in megabytes
- vlan	    : vlan, as an integer
- bridge    : bridge name
- tap	    : tap name
- mac	    : mac address
- ip	    : IPv4 address
- ip6	    : IPv6 address
- bandwidth : stats regarding bandwidth usage
- tags      : any additional information attached to the VM

Additional fields are available for KVM-based VMs:

- append    : kernel command line string
- cdrom     : cdrom image
- disk      : disk image
- kernel    : kernel image
- initrd    : initrd image
- migrate   : qemu migration image
- uuid      : QEMU system uuid
- cc_active : whether cc is active

Examples:

Display a list of all IPs for all VMs:
	.columns ip,ip6 vm info

Display all information about KVM-based VMs with the disk image foo.qc2:
	.filter disk=foo.qc2 vm kvm info

Display information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info",
			"vm <kvm,> info",
		},
		Call: wrapSimpleCLI(cliVmInfo),
	},
	{ // vm save
		HelpShort: "save a vm configuration for later use",
		HelpLong: `
Saves the configuration of a running virtual machine or set of virtual machines
so that it/they can be restarted/recovered later, such as after a system crash.

This command does not store the state of the virtual machine itself, only its
launch configuration.`,
		Patterns: []string{
			"vm save <name> <vm id or name or all>...",
		},
		Call: wrapSimpleCLI(cliVmSave),
	},
	{ // vm launch
		HelpShort: "launch virtual machines in a paused state",
		HelpLong: `
Launch virtual machines in a paused state, using the parameters defined leading
up to the launch command. Any changes to the VM parameters after launching will
have no effect on launched VMs.

If you supply a name instead of a number of VMs, one VM with that name will be
launched. You may also supply a range expression to launch VMs with a specific
naming scheme:

	vm launch foo[0-9]

The optional 'noblock' suffix forces minimega to return control of the command
line immediately instead of waiting on potential errors from launching the
VM(s). The user must check logs or error states from vm info.`,
		Patterns: []string{
			"vm launch <name or count> [noblock,]",
		},
		Call: wrapSimpleCLI(cliVmLaunch),
	},
	{ // vm kill
		HelpShort: "kill running virtual machines",
		HelpLong: `
Kill a virtual machine by ID or name. Pass all to kill all virtual machines.`,
		Patterns: []string{
			"vm kill <vm id or name or all>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func() []error {
				return vms.kill(c.StringArgs["vm"])
			})
		}),
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: `
Start one or all paused virtual machines. Pass all to start all paused virtual
machines.

Calling vm start specifically on a quit VM will restart the VM. If the optional
'quit' suffix is used with the wildcard, then all virtual machines in the
paused *or* quit state will be restarted.`,
		Patterns: []string{
			"vm start <vm id or name or all> [quit,]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func() []error {
				return vms.start(c.StringArgs["vm"], c.BoolArgs["quit"])
			})
		}),
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop one or all running virtual machines. Pass all to stop all running virtual
machines.

Calling stop will put VMs in a paused state. Start stopped VMs with vm start.`,
		Patterns: []string{
			"vm stop <vm id or name or all>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func() []error {
				return vms.stop(c.StringArgs["vm"])
			})
		}),
	},
	{ // vm flush
		HelpShort: "discard information about quit or failed VMs",
		HelpLong: `
Discard information about VMs that have either quit or encountered an error.
This will remove any VMs with a state of "quit" or "error" from vm info. Names
of VMs that have been flushed may be reused.`,
		Patterns: []string{
			"vm flush",
		},
		Call: wrapSimpleCLI(cliVmFlush),
	},
	{ // vm hotplug
		HelpShort: "add and remove USB drives",
		HelpLong: `
Add and remove USB drives to a launched VM.

To view currently attached media, call vm hotplug with the 'show' argument and
a VM ID or name. To add a device, use the 'add' argument followed by the VM ID
or name, and the name of the file to add. For example, to add foo.img to VM 5:

	vm hotplug add 5 foo.img

The add command will assign a disk ID, shown in vm hotplug show. To remove
media, use the 'remove' argument with the VM ID and the disk ID. For example,
to remove the drive added above, named 0:

	vm hotplug remove 5 0

To remove all hotplug devices, use ID "all" for the disk ID.`,
		Patterns: []string{
			"vm hotplug <show,> <vm id or name>",
			"vm hotplug <add,> <vm id or name> <filename>",
			"vm hotplug <remove,> <vm id or name> <disk id or all>",
		},
		Call: wrapSimpleCLI(cliVmHotplug),
	},
	{ // vm net
		HelpShort: "disconnect or move network connections",
		HelpLong: `
Disconnect or move existing network connections on a running VM.

Network connections are indicated by their position in vm net (same order in vm
info) and are zero indexed. For example, to disconnect the first network
connection from a VM named vm-0 with 4 network connections:

	vm net disconnect vm-0 0

To disconnect the second connection:

	vm net disconnect vm-0 1

To move a connection, specify the new VLAN tag and bridge:

	vm net <vm name or id> 0 bridgeX 100`,
		Patterns: []string{
			"vm net <connect,> <vm id or name> <tap position> <bridge> <vlan>",
			"vm net <disconnect,> <vm id or name> <tap position>",
		},
		Call: wrapSimpleCLI(cliVmNetMod),
	},
	{ // vm qmp
		HelpShort: "issue a JSON-encoded QMP command",
		HelpLong: `
Issue a JSON-encoded QMP command. This is a convenience function for accessing
the QMP socket of a VM via minimega. vm qmp takes two arguments, a VM ID or
name, and a JSON string, and returns the JSON encoded response. For example:

	vm qmp 0 '{ "execute": "query-status" }'
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		Patterns: []string{
			"vm qmp <vm id or name> <qmp command>",
		},
		Call: wrapSimpleCLI(cliVmQmp),
	},
	{ // vm screenshot
		HelpShort: "take a screenshot of a running vm",
		HelpLong: `
Take a screenshot of the framebuffer of a running VM. The screenshot is saved
in PNG format as "screenshot.png" in the VM's runtime directory (by default
/tmp/minimega/<vm id>/screenshot.png).

An optional argument sets the maximum dimensions in pixels, while keeping the
aspect ratio. For example, to set either maximum dimension of the output image
to 100 pixels:

	vm screenshot foo 100`,
		Patterns: []string{
			"vm screenshot <vm id or name> [maximum dimension]",
		},
		Call: wrapSimpleCLI(cliVmScreenshot),
	},
	{ // vm migrate
		HelpShort: "write VM state to disk",
		HelpLong: `
Migrate runtime state of a VM to disk, which can later be booted with vm config
migrate.

Migration files are written to the files directory as specified with -filepath.
On success, a call to migrate a VM will return immediately. You can check the
status of in-flight migrations by invoking vm migrate with no arguments.`,
		Patterns: []string{
			"vm migrate",
			"vm migrate <vm id or name> <filename>",
		},
		Call: wrapSimpleCLI(cliVmMigrate),
	},
	{ // vm tag
		HelpShort: "display or set a tag for the specified VM",
		HelpLong: `
Display or set a tag for the specified VM.

Tags are key-value pairs. A VM can have any number of tags associated with it.
They can be used to attach additional information to a virtual machine, for
example specifying a VM "group", or the correct rendering color for some
external visualization tool.

To set a tag "foo" to "bar":

        vm tag <vm id or name> foo bar

To read a tag:

        vm tag <vm id or name> <key>`,
		Patterns: []string{
			"vm tag <vm id or name> <key> [value]",
		},
		Call: wrapSimpleCLI(cliVmTag),
	},
	{ // vm cdrom
		HelpShort: "eject or change an active VM's cdrom",
		HelpLong: `
Eject or change an active VM's cdrom image.

Eject VM 0's cdrom:

        vm cdrom eject 0

Eject all VM cdroms:

        vm cdrom eject all

Change a VM to use a new ISO:

        vm cdrom change 0 /tmp/debian.iso

"vm cdrom change" implies that the current ISO will be ejected.`,
		Patterns: []string{
			"vm cdrom <eject,> <vm id or name>",
			"vm cdrom <change,> <vm id or name> <path>",
		},
		Call: wrapSimpleCLI(cliVmCdrom),
	},
	{ // vm config
		HelpShort: "display, save, or restore the current VM configuration",
		HelpLong: `
Display, save, or restore the current VM configuration.

To display the current configuration, call vm config with no arguments.

List the current saved configurations with 'vm config show'.

To save a configuration:

	vm config save <config name>

To restore a configuration:

	vm config restore <config name>

To clone the configuration of an existing VM:

	vm config clone <vm name or id>

Calling clear vm config will clear all VM configuration options, but will not
remove saved configurations.`,
		Patterns: []string{
			"vm config",
			"vm <kvm,> config",
			"vm config <save,> <name>",
			"vm config <restore,> [name]",
			"vm config <clone,> <vm id or name>",
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
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "memory")
		}),
	},
	{ // vm config vcpus
		HelpShort: "set the number of virtual CPUs for a VM",
		HelpLong: `
Set the number of virtual CPUs to allocate for a VM.`,
		Patterns: []string{
			"vm config vcpus [number of CPUs]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "vcpus")
		}),
	},
	{ // vm config net
		HelpShort: "specific the networks a VM is a member of",
		HelpLong: `
Specify the network(s) that the VM is a member of by VLAN. A corresponding VLAN
will be created for each network. Optionally, you may specify the bridge the
interface will be connected on. If the bridge name is omitted, minimega will
use the default 'mega_bridge'. You can also optionally specify the mac address
of the interface to connect to that network. If not specifed, the mac address
will be randomly generated. Additionally, you can optionally specify a driver
for qemu to use. By default, e1000 is used.

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

Calling vm net with no parameters will list the current networks for this VM.`,
		Patterns: []string{
			"vm config net [netspec]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "net")
		}),
	},
	{ // vm kvm config append
		HelpShort: "set an append string to pass to a kernel set with vm kernel",
		HelpLong: `
Add an append string to a kernel set with vm kernel. Setting vm append without
using vm kernel will result in an error.

For example, to set a static IP for a linux VM:

	vm kvm config append ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10`,
		Patterns: []string{
			"vm <kvm,> config append [arg]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "append")
		}),
	},
	{ // vm kvm config qemu
		HelpShort: "set the QEMU process to invoke. Relative paths are ok.",
		Patterns: []string{
			"vm <kvm,> config qemu [path to qemu]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu")
		}),
	},
	{ // vm kvm config qemu-override
		HelpShort: "override parts of the QEMU launch string",
		HelpLong: `
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string.`,
		Patterns: []string{
			"vm <kvm,> config qemu-override",
			"vm <kvm,> config qemu-override add <match> <replacement>",
			"vm <kvm,> config qemu-override delete <id or all>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu-override")
		}),
	},
	{ // vm kvm config qemu-append
		HelpShort: "add additional arguments to the QEMU command",
		HelpLong: `
Add additional arguments to be passed to the QEMU instance. For example:

	vm kvm config qemu-append -serial tcp:localhost:4001`,
		Patterns: []string{
			"vm <kvm,> config qemu-append [argument]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu-append")
		}),
	},
	{ // vm kvm config migrate
		HelpShort: "set migration image for a saved VM",
		HelpLong: `
Assign a migration image, generated by a previously saved VM to boot with.
Migration images should be booted with a kernel/initrd, disk, or cdrom. Use 'vm
migrate' to generate migration images from running VMs.`,
		Patterns: []string{
			"vm <kvm,> config migrate [path to migration image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "migrate")
		}),
	},
	{ // vm kvm config disk
		HelpShort: "set disk images to attach to a VM",
		HelpLong: `
Attach one or more disks to a vm. Any disk image supported by QEMU is a valid
parameter. Disk images launched in snapshot mode may safely be used for
multiple VMs.`,
		Patterns: []string{
			"vm <kvm,> config disk [path to disk image]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "disk")
		}),
	},
	{ // vm kvm config cdrom
		HelpShort: "set a cdrom image to attach to a VM",
		HelpLong: `
Attach a cdrom to a VM. When using a cdrom, it will automatically be set to be
the boot device.`,
		Patterns: []string{
			"vm <kvm,> config cdrom [path to cdrom image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "cdrom")
		}),
	},
	{ // vm kvm config kernel
		HelpShort: "set a kernel image to attach to a VM",
		HelpLong: `
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.`,
		Patterns: []string{
			"vm <kvm,> config kernel [path to kernel]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "kernel")
		}),
	},
	{ // vm kvm config initrd
		HelpShort: "set a initrd image to attach to a VM",
		HelpLong: `
Attach an initrd image to a VM. Passed along with the kernel image at boot
time.`,
		Patterns: []string{
			"vm <kvm,> config initrd [path to initrd]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "initrd")
		}),
	},
	{ // vm kvm config uuid
		HelpShort: "set the UUID for a VM",
		HelpLong: `
Set the UUID for a virtual machine. If not set, minimega will create a random
one when the VM is launched.`,
		Patterns: []string{
			"vm <kvm,> config uuid [uuid]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "uuid")
		}),
	},
	{ // vm kvm config snapshot
		HelpShort: "enable or disable snapshot mode when using disk images",
		HelpLong: `
Enable or disable snapshot mode when using disk images. When enabled, disks
images will be loaded in memory when run and changes will not be saved. This
allows a single disk image to be used for many VMs.`,
		Patterns: []string{
			"vm <kvm,> config snapshot [true,false]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "snapshot")
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
			"clear vm config <memory,>",
			"clear vm config <net,>",
			"clear vm config <vcpus,>",
			"clear vm <kvm,> config",
			"clear vm <kvm,> config <append,>",
			"clear vm <kvm,> config <cdrom,>",
			"clear vm <kvm,> config <migrate,>",
			"clear vm <kvm,> config <disk,>",
			"clear vm <kvm,> config <initrd,>",
			"clear vm <kvm,> config <kernel,>",
			"clear vm <kvm,> config <qemu,>",
			"clear vm <kvm,> config <qemu-append,>",
			"clear vm <kvm,> config <qemu-override,>",
			"clear vm <kvm,> config <snapshot,>",
			"clear vm <kvm,> config <uuid,>",
		},
		Call: wrapSimpleCLI(cliClearVmConfig),
	},
	{ // clear vm tag
		HelpShort: "remove tags from a VM",
		HelpLong: `
Clears one, many, or all tags from a virtual machine.

Clear the tag "foo" from VM 0:

        clear vm tag 0 foo

Clear the tag "foo" from all VMs:

        clear vm tag all foo

Clear all tags from VM 0:

        clear vm tag 0

Clear all tags from all VMs:

        clear vm tag all`,
		Patterns: []string{
			"clear vm tag <vm id or name or all> [tag]",
		},
		Call: wrapSimpleCLI(cliClearVmTag),
	},
}

func init() {
	registerHandlers("vm", vmCLIHandlers)

	// for vm info
	gob.Register(VMs{})
}

func cliVmInfo(c *minicli.Command) *minicli.Response {
	var err error
	resp := &minicli.Response{Host: hostname}

	vmType := ""
	if c.BoolArgs["kvm"] {
		vmType = "kvm"
	}

	resp.Header, resp.Tabular, err = vms.info(vmType)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	resp.Data = vms

	return resp
}

func cliVmCdrom(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vmstring := c.StringArgs["vm"]
	doVms := make([]*vmKVM, 0)
	if vmstring == Wildcard {
		for _, vm := range vms {
			switch vm := vm.(type) {
			case *vmKVM:
				doVms = append(doVms, vm)
			default:
				// TODO: Do anything?
			}
		}
	} else {
		vm := vms.findVm(vmstring)
		if vm == nil {
			resp.Error = vmNotFound(vmstring).Error()
			return resp
		}
		if vm, ok := vm.(*vmKVM); ok {
			doVms = append(doVms, vm)
		} else {
			resp.Error = "cdrom commands are only supported for kvm vms"
			return resp
		}
	}

	if c.BoolArgs["eject"] {
		for _, v := range doVms {
			err := v.q.BlockdevEject("ide0-cd1")
			v.CdromPath = ""
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
		}
	} else if c.BoolArgs["change"] {
		for _, v := range doVms {
			// First eject it, then change it
			err := v.q.BlockdevEject("ide0-cd1")
			v.CdromPath = ""
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			err = v.q.BlockdevChange("ide0-cd1", c.StringArgs["path"])
			v.CdromPath = c.StringArgs["path"]
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
		}

	}

	return resp
}

func cliVmTag(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := vms.findVm(c.StringArgs["vm"])
	if vm == nil {
		resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		return resp
	}

	key := c.StringArgs["key"]
	if value, ok := c.StringArgs["value"]; ok {
		// Set a tag
		vm.SetTag(key, value)
	} else {
		// Get a tag
		if v := vm.Tag(key); v != "" {
			resp.Response = v
		} else {
			resp.Error = fmt.Sprintf("tag %v not set for vm %v", key, vm.Name())
		}
	}

	return resp
}

func cliClearVmTag(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vmstring := c.StringArgs["vm"]
	clearVms := make([]VM, 0)
	if vmstring == Wildcard {
		for _, v := range vms {
			clearVms = append(clearVms, v)
		}
	} else {
		vm := vms.findVm(vmstring)
		if vm == nil {
			resp.Error = vmNotFound(vmstring).Error()
			return resp
		}
		clearVms = append(clearVms, vm)
	}

	if tag := c.StringArgs["tag"]; tag != "" {
		for _, vm := range clearVms {
			vm.ClearTags()
		}
	} else {
		for _, vm := range clearVms {
			vm.ClearTag(tag)
		}
	}

	return resp
}

func cliVmConfig(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["save"] {
		// Save the current config
		savedInfo[c.StringArgs["name"]] = kvmConfig.Copy()
	} else if c.BoolArgs["restore"] {
		if name, ok := c.StringArgs["name"]; ok {
			// Try to restore an existing config
			if s, ok := savedInfo[name]; ok {
				// TODO: copy vmConfig too
				kvmConfig = s.Copy()
			} else {
				resp.Error = fmt.Sprintf("config %v does not exist", name)
			}
		} else if len(savedInfo) == 0 {
			resp.Error = "no vm configs saved"
		} else {
			// List the save configs
			for k := range savedInfo {
				resp.Response += fmt.Sprintln(k)
			}
		}
	} else if c.BoolArgs["clone"] {
		// Clone the config of an existing vm
		vm := vms.findVm(c.StringArgs["vm"])
		if vm == nil {
			resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		} else {
			// TODO: Copy kvmConfig?
			vmConfig = vm.Config()
		}
	} else if c.BoolArgs["kvm"] {
		// Print the kvm config
		resp.Response = vmConfig.configToString() + kvmConfig.configToString()
	} else {
		// Print the vm config
		resp.Response = vmConfig.configToString()
	}

	return resp
}

func cliVmConfigField(c *minicli.Command, field string) *minicli.Response {
	var err error
	resp := &minicli.Response{Host: hostname}

	// If there are no args it means that we want to display the current value
	nArgs := len(c.StringArgs) + len(c.ListArgs) + len(c.BoolArgs)

	kvm := c.BoolArgs["kvm"]
	if kvm {
		nArgs -= 1
	}

	if fns, ok := kvmConfigFns[field]; kvm && ok {
		if nArgs == 0 {
			resp.Response = fns.Print(kvmConfig)
		} else {
			err = fns.Update(kvmConfig, c)
		}
	} else if fns, ok := vmConfigFns[field]; !kvm && ok {
		if nArgs == 0 {
			resp.Response = fns.Print(vmConfig)
		} else {
			err = fns.Update(vmConfig, c)
		}
	} else {
		log.Fatalln("unknown config field: `%s`", field)
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliClearVmConfig(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var clearAll = len(c.BoolArgs) == 0
	var clearKVM = clearAll || (len(c.BoolArgs) == 1 && c.BoolArgs["kvm"])
	var cleared bool

	for k, fns := range vmConfigFns {
		if clearAll || c.BoolArgs[k] {
			fns.Clear(vmConfig)
			cleared = true
		}
	}
	for k, fns := range kvmConfigFns {
		if clearKVM || c.BoolArgs[k] {
			fns.Clear(kvmConfig)
			cleared = true
		}
	}

	if !cleared {
		log.Fatalln("no callback defined for clear")
	}

	return resp
}

func cliVmLaunch(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	arg := c.StringArgs["name"]
	vmNames := []string{}

	count, err := strconv.ParseInt(arg, 10, 32)
	if err == nil {
		if count <= 0 {
			resp.Error = "invalid number of vms (must be >= 1)"
			return resp
		}

		for i := int64(0); i < count; i++ {
			vmNames = append(vmNames, "")
		}
	} else {
		index := strings.IndexRune(arg, '[')
		if index == -1 {
			vmNames = append(vmNames, arg)
		} else {
			r, err := ranges.NewRange(arg[:index], 0, int(math.MaxInt32))
			if err != nil {
				log.Fatalln(err)
			}

			names, err := r.SplitRange(arg)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			vmNames = append(vmNames, names...)
		}
	}

	if len(vmNames) == 0 {
		resp.Error = "no VMs to launch"
		return resp
	}

	for _, name := range vmNames {
		if isReserved(name) {
			resp.Error = fmt.Sprintf("`%s` is a reserved word -- cannot use for vm name", name)
			return resp
		}

		for _, vm := range vms {
			if vm.Name() == name {
				resp.Error = fmt.Sprintf("`%s` is already the name of a VM", name)
				return resp
			}
		}
	}

	log.Info("launching %v vms", len(vmNames))

	ack := make(chan int)
	waitForAcks := func(count int) {
		// get acknowledgements from each vm
		for i := 0; i < count; i++ {
			log.Debug("launch ack from VM %v", <-ack)
		}
	}

	for i, vmName := range vmNames {
		if err := vms.launch(vmName, ack); err != nil {
			resp.Error = err.Error()
			go waitForAcks(i)
			return resp
		}
	}

	if c.BoolArgs["noblock"] {
		go waitForAcks(len(vmNames))
	} else {
		waitForAcks(len(vmNames))
	}

	return resp
}

func cliVmApply(c *minicli.Command, fn func() []error) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	for _, err := range fn() {
		if err != nil {
			resp.Error += fmt.Sprintln(err)
		}
	}

	return resp
}

func cliVmFlush(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vms.flush()

	return resp
}

func cliVmQmp(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var err error
	resp.Response, err = vms.qmp(c.StringArgs["vm"], c.StringArgs["qmp"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliVmScreenshot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := c.StringArgs["vm"]
	maximum := c.StringArgs["maximum"]

	var max int
	var err error
	if maximum != "" {
		max, err = strconv.Atoi(maximum)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	}

	v := vms.findVm(vm)
	if v == nil {
		resp.Error = vmNotFound(vm).Error()
		return resp
	}

	path := filepath.Join(*f_base, fmt.Sprintf("%v", v.ID), "screenshot.png")

	err = vms.screenshot(vm, path, max)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	// add user data in case this is going across meshage
	f, err := os.Open(path)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	var buf bytes.Buffer
	io.Copy(&buf, f)
	resp.Data = buf.Bytes()

	return resp
}

func cliVmMigrate(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var err error

	if _, ok := c.StringArgs["vm"]; !ok { // report current migrations
		// tabular data is
		// 	vm id, vm name, migrate status, % complete
		for _, vm := range vms {
			vm, ok := vm.(*vmKVM)
			if !ok {
				// TODO: remove?
				continue
			}

			status, complete, err := vm.QueryMigrate()
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			if status == "" {
				continue
			}
			resp.Tabular = append(resp.Tabular, []string{
				fmt.Sprintf("%v", vm.ID()),
				vm.Name(),
				status,
				fmt.Sprintf("%.2f", complete)})
		}
		if len(resp.Tabular) != 0 {
			resp.Header = []string{"vm id", "vm name", "status", "%% complete"}
		}
		return resp
	}

	err = vms.migrate(c.StringArgs["vm"], c.StringArgs["filename"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliVmSave(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	path := filepath.Join(*f_base, "saved_vms")
	err := os.MkdirAll(path, 0775)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	name := c.StringArgs["name"]
	file, err := os.Create(filepath.Join(path, name))
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	err = vms.save(file, c.ListArgs["vm"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliVmHotplug(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := vms.findVm(c.StringArgs["vm"])
	if vm == nil {
		resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		return resp
	}
	kvm, ok := vm.(*vmKVM)
	if !ok {
		resp.Error = fmt.Sprintf("`%s` is not a kvm vm -- command unsupported", vm.Name())
		return resp
	}

	if c.BoolArgs["add"] {
		// generate an id by adding 1 to the highest in the list for the
		// hotplug devices, 0 if it's empty
		id := 0
		for k, _ := range kvm.hotplug {
			if k >= id {
				id = k + 1
			}
		}
		hid := fmt.Sprintf("hotplug%v", id)
		log.Debugln("hotplug generated id:", hid)

		r, err := kvm.q.DriveAdd(hid, c.StringArgs["filename"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		log.Debugln("hotplug drive_add response:", r)
		r, err = kvm.q.USBDeviceAdd(hid)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		log.Debugln("hotplug usb device add response:", r)
		kvm.hotplug[id] = c.StringArgs["filename"]
	} else if c.BoolArgs["remove"] {
		if c.StringArgs["disk"] == Wildcard {
			for k := range kvm.hotplug {
				if err := kvm.hotplugRemove(k); err != nil {
					resp.Error = err.Error()
					// TODO: try to remove the rest if there's an error?
					break
				}
			}

			return resp
		}

		id, err := strconv.Atoi(c.StringArgs["disk"])
		if err != nil {
			resp.Error = err.Error()
		} else if err := kvm.hotplugRemove(id); err != nil {
			resp.Error = err.Error()
		}
	} else if c.BoolArgs["show"] {
		if len(kvm.hotplug) > 0 {
			resp.Header = []string{"hotplug ID", "File"}
			resp.Tabular = [][]string{}

			for k, v := range kvm.hotplug {
				resp.Tabular = append(resp.Tabular, []string{strconv.Itoa(k), v})
			}
		}
	}

	return resp
}

func cliVmNetMod(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := vms.findVm(c.StringArgs["vm"])
	if vm == nil {
		resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		return resp
	}

	pos, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	config := vm.Config()

	if len(config.Networks) < pos {
		resp.Error = fmt.Sprintf("no such network %v, VM only has %v networks", pos, len(config.Networks))
		return resp
	}

	// TODO: this isn't right... need Config to return *vmConfig
	net := &config.Networks[pos]

	var b *bridge
	if c.StringArgs["bridge"] != "" {
		b, err = getBridge(c.StringArgs["bridge"])
	} else {
		b, err = getBridge(net.Bridge)
	}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if c.BoolArgs["disconnect"] {
		log.Debug("disconnect network connection: %v %v %v", vm.ID, pos, net)
		err = b.TapRemove(net.VLAN, net.Tap)
		if err != nil {
			resp.Error = err.Error()
		} else {
			net.VLAN = -1
		}
	} else if c.BoolArgs["connect"] {
		vlan, err := strconv.Atoi(c.StringArgs["vlan"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		if vlan >= 0 && vlan < 4096 {
			// new network
			log.Debug("moving network connection: %v %v %v -> %v %v", vm.ID, pos, net.VLAN, b.Name, vlan)
			oldBridge, err := getBridge(net.Bridge)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			if net.VLAN != -1 {
				err := oldBridge.TapRemove(net.VLAN, net.Tap)
				if err != nil {
					resp.Error = err.Error()
					return resp
				}
			}

			err = b.TapAdd(vlan, net.Tap, false)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			net.VLAN = vlan
			net.Bridge = b.Name
		} else {
			resp.Error = fmt.Sprintf("invalid vlan tag %v", net)
		}
	}

	return resp
}
