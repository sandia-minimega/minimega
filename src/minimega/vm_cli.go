// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
)

var vmCLIHandlers = []minicli.Handler{
	{ // vm info
		HelpShort: "print information about VMs",
		HelpLong: `
Print information about VMs in tabular form. The .filter and .columns commands
can be used to subselect a set of rows and/or columns. See the help pages for
.filter and .columns, respectively, for their usage. Columns returned by VM
info include:

- id	    : the VM ID, as an integer
- host	    : the host that the VM is running on
- name	    : the VM name, if it exists
- state     : one of (building, running, paused, quit, error)
- memory    : allocated memory, in megabytes
- vcpus     : the number of allocated CPUs
- migrate   : qemu migration image
- disk      : disk image
- initrd    : initrd image
- kernel    : kernel image
- cdrom     : cdrom image
- append    : kernel command line string
- bridge    : bridge name
- tap	    : tap name
- mac	    : mac address
- ip	    : IPv4 address
- ip6	    : IPv6 address
- vlan	    : vlan, as an integer
- uuid      : QEMU system uuid
- cc_active : whether cc is active
- tags      : any additional information attached to the VM

Examples:

Display a list of all IPs for all VMs:
	.columns ip,ip6 vm info

Display all information about VMs with the disk image foo.qc2:
	.filter disk=foo.qc2 vm info

Display all information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info",
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
Kill one or more running virtual machines. See "help vm target" for acceptable
targets.`,
		Patterns: []string{
			"vm kill <target>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func(target string) []error {
				return vms.kill(target)
			})
		}),
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: `
Start one or more paused virtual machines. See "help vm target" for allowed
targets.

Calling "vm start" on a specific list of VMs will cause them to be started if they
are in the building, paused, quit, or error states. When used with the wildcard, only
vms in the building or paused state will be started.`,
		Patterns: []string{
			"vm start <target>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func(target string) []error {
				return vms.start(target)
			})
		}),
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop one or more running virtual machines. See "help vm target" for acceptable
targets.

Calling stop will put VMs in a paused state. Use "vm start" to restart them.`,
		Patterns: []string{
			"vm stop <target>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmApply(c, func(target string) []error {
				return vms.stop(target)
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

	vm netmod disconnect vm-0 0

To disconnect the second connection:

	vm netmod disconnect vm-0 1

To move a connection, specify the new VLAN tag and bridge:

	vm netmod <vm name or id> 0 bridgeX 100`,
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
Display or set a tag for one or more virtual machines. See "help vm target" for
acceptable targets.

Tags are key-value pairs. A VM can have any number of tags associated with it.
They can be used to attach additional information to a virtual machine, for
example specifying a VM "group", or the correct rendering color for some
external visualization tool.

To set a tag "foo" to "bar" for VM 2:

        vm tag 2 foo bar

To read a tag:

        vm tag <target> <key or all>`,
		Patterns: []string{
			"vm tag <target> [key or all]",  // get
			"vm tag <target> <key> <value>", // set
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
			"vm config <save,> <name>",
			"vm config <restore,> [name]",
			"vm config <clone,> <vm id or name>",
		},
		Call: wrapSimpleCLI(cliVmConfig),
	},
	{ // vm config qemu
		HelpShort: "set the QEMU process to invoke. Relative paths are ok.",
		Patterns: []string{
			"vm config qemu [path to qemu]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu")
		}),
	},
	{ // vm config qemu-override
		HelpShort: "override parts of the QEMU launch string",
		HelpLong: `
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string.`,
		Patterns: []string{
			"vm config qemu-override",
			"vm config qemu-override add <match> <replacement>",
			"vm config qemu-override delete <id or all>",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu-override")
		}),
	},
	{ // vm config qemu-append
		HelpShort: "add additional arguments to the QEMU command",
		HelpLong: `
Add additional arguments to be passed to the QEMU instance. For example:

	vm config qemu-append -serial tcp:localhost:4001`,
		Patterns: []string{
			"vm config qemu-append [argument]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "qemu-append")
		}),
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
	{ // vm config migrate
		HelpShort: "set migration image for a saved VM",
		HelpLong: `
Assign a migration image, generated by a previously saved VM to boot with.
Migration images should be booted with a kernel/initrd, disk, or cdrom. Use 'vm
migrate' to generate migration images from running VMs.`,
		Patterns: []string{
			"vm config migrate [path to migration image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "migrate")
		}),
	},
	{ // vm config disk
		HelpShort: "set disk images to attach to a VM",
		HelpLong: `
Attach one or more disks to a vm. Any disk image supported by QEMU is a valid
parameter. Disk images launched in snapshot mode may safely be used for
multiple VMs.`,
		Patterns: []string{
			"vm config disk [path to disk image]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "disk")
		}),
	},
	{ // vm config cdrom
		HelpShort: "set a cdrom image to attach to a VM",
		HelpLong: `
Attach a cdrom to a VM. When using a cdrom, it will automatically be set to be
the boot device.`,
		Patterns: []string{
			"vm config cdrom [path to cdrom image]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "cdrom")
		}),
	},
	{ // vm config kernel
		HelpShort: "set a kernel image to attach to a VM",
		HelpLong: `
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.`,
		Patterns: []string{
			"vm config kernel [path to kernel]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "kernel")
		}),
	},
	{ // vm config append
		HelpShort: "set an append string to pass to a kernel set with vm kernel",
		HelpLong: `
Add an append string to a kernel set with vm kernel. Setting vm append without
using vm kernel will result in an error.

For example, to set a static IP for a linux VM:

	vm config append ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10`,
		Patterns: []string{
			"vm config append [argument]...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "append")
		}),
	},
	{ // vm config uuid
		HelpShort: "set the UUID for a VM",
		HelpLong: `
Set the UUID for a virtual machine. If not set, minimega will create a random
one when the VM is launched.`,
		Patterns: []string{
			"vm config uuid [uuid]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "uuid")
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
	{ // vm config snapshot
		HelpShort: "enable or disable snapshot mode when using disk images",
		HelpLong: `
Enable or disable snapshot mode when using disk images. When enabled, disks
images will be loaded in memory when run and changes will not be saved. This
allows a single disk image to be used for many VMs.`,
		Patterns: []string{
			"vm config snapshot [true,false]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "snapshot")
		}),
	},
	{ // vm config initrd
		HelpShort: "set a initrd image to attach to a VM",
		HelpLong: `
Attach an initrd image to a VM. Passed along with the kernel image at boot
time.`,
		Patterns: []string{
			"vm config initrd [path to initrd]",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliVmConfigField(c, "initrd")
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
			"clear vm config <append,>",
			"clear vm config <cdrom,>",
			"clear vm config <migrate,>",
			"clear vm config <disk,>",
			"clear vm config <initrd,>",
			"clear vm config <kernel,>",
			"clear vm config <memory,>",
			"clear vm config <net,>",
			"clear vm config <qemu,>",
			"clear vm config <qemu-append,>",
			"clear vm config <qemu-override,>",
			"clear vm config <snapshot,>",
			"clear vm config <uuid,>",
			"clear vm config <vcpus,>",
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
			"clear vm tag",
			"clear vm tag <target> [tag]",
		},
		Call: wrapSimpleCLI(cliClearVmTag),
	},
	{ // vm target (dummy command)
		Doc:       true,
		HelpShort: "documentation for valid vm targets",
		HelpLong: fmt.Sprintf(`
VM targets are accepted by a number of different minimega commands to specify
one or more VMs. The VM target accepts a comma-seperated list of identifiers
for VMs including the VM name and ID.

To start vm foo:

		vm start foo

To start vms foo and bar:

		vm start foo,bar

To start vms foo0, foo1, foo2, and foo5:

		vm start foo[0-2,5]

There is also a wildcard (%[1]s) which allows the user to specify all VMs:

		vm start %[1]s

Note that including the wildcard in a list of VMs results in the wildcard
behavior (although a message will be logged).`, Wildcard),
		Patterns: []string{
			"vm target",
		},
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

	resp.Header, resp.Tabular, err = vms.info()
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
	doVms := make([]*vmInfo, 0)
	if vmstring == Wildcard {
		for _, v := range vms {
			doVms = append(doVms, v)
		}
	} else {
		vm := vms.findVm(vmstring)
		if vm == nil {
			resp.Error = vmNotFound(vmstring).Error()
			return resp
		}
		doVms = append(doVms, vm)
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

	key := c.StringArgs["key"]
	if key == "" {
		// If they didn't specify a key then they probably want all the tags
		// for a given VM
		key = Wildcard
	}

	value, setOp := c.StringArgs["value"]
	if setOp {
		if key == Wildcard {
			// Can't assign a value to wildcard!
			resp.Error = "cannot assign to wildcard"
			return resp
		}
	} else {
		if key == Wildcard {
			resp.Header = []string{"ID", "Tag", "Value"}
		} else {
			resp.Header = []string{"ID", "Value"}
		}

		resp.Tabular = make([][]string, 0)
	}

	target := c.StringArgs["target"]

	errs := expandVmTargets(target, false, func(vm *vmInfo, wild bool) (bool, error) {
		if setOp {
			vm.Tags[key] = value
		} else if key == Wildcard {
			for k, v := range vm.Tags {
				resp.Tabular = append(resp.Tabular, []string{
					strconv.Itoa(vm.ID),
					k, v,
				})
			}
		} else {
			resp.Tabular = append(resp.Tabular, []string{
				strconv.Itoa(vm.ID),
				vm.Tags[key],
			})
		}

		return true, nil
	})

	if len(errs) > 0 {
		resp.Error = errSlice(errs).String()
	}

	return resp
}

func cliClearVmTag(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	key := c.StringArgs["key"]
	if key == "" {
		// If they didn't specify a key then they probably want all the tags
		// for a given VM
		key = Wildcard
	}

	target, ok := c.StringArgs["target"]
	if !ok {
		// No target specified, must want to clear all
		target = Wildcard
	}

	errs := expandVmTargets(target, true, func(vm *vmInfo, wild bool) (bool, error) {
		if key == Wildcard {
			vm.Tags = make(map[string]string)
		} else {
			delete(vm.Tags, key)
		}

		return true, nil
	})

	if len(errs) > 0 {
		resp.Error = errSlice(errs).String()
	}

	return resp
}

func cliVmConfig(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["save"] {
		// Save the current config
		savedInfo[c.StringArgs["name"]] = info.Copy()
	} else if c.BoolArgs["restore"] {
		if name, ok := c.StringArgs["name"]; ok {
			// Try to restore an existing config
			if s, ok := savedInfo[name]; ok {
				info = s.Copy()
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
			info = vm.Copy()
		}
	} else {
		// Print the full config
		resp.Response = info.configToString()
	}

	return resp
}

func cliVmConfigField(c *minicli.Command, field string) *minicli.Response {
	var err error
	resp := &minicli.Response{Host: hostname}

	fns := vmConfigFns[field]

	// If there are no args it means that we want to display the current value
	if len(c.StringArgs) == 0 && len(c.ListArgs) == 0 && len(c.BoolArgs) == 0 {
		resp.Response = fns.Print(info)
		return resp
	}

	// We expect exactly one key in either the String, List, or Bool Args for
	// most configs. For some, there is more complex processing and they need
	// the whole command.
	if fns.UpdateCommand != nil {
		err = fns.UpdateCommand(c)
	} else if len(c.StringArgs) == 1 && fns.Update != nil {
		for _, arg := range c.StringArgs {
			err = fns.Update(info, arg)
		}
	} else if len(c.ListArgs) == 1 && fns.Update != nil {
		// Lists need to be cleared first since they process each arg
		// individually to build state
		fns.Clear(info)

		for _, args := range c.ListArgs {
			for _, arg := range args {
				if err = fns.Update(info, arg); err != nil {
					break
				}
			}
		}
	} else if len(c.BoolArgs) == 1 && fns.UpdateBool != nil {
		// Special case, look for key "true" (there should only be two options,
		// "true" or "false" and, therefore, not "true" implies "false").
		err = fns.UpdateBool(info, c.BoolArgs["true"])
	} else {
		log.Fatalln("someone goofed on the patterns")
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliClearVmConfig(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var clearAll = len(c.BoolArgs) == 0
	var cleared bool

	for k, fns := range vmConfigFns {
		if clearAll || c.BoolArgs[k] {
			fns.Clear(info)
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
	names := []string{}

	count, err := strconv.ParseInt(arg, 10, 32)
	if err != nil {
		names, err = expandListRange(arg)
	} else if count <= 0 {
		err = errors.New("invalid number of vms (must be > 0)")
	} else {
		for i := int64(0); i < count; i++ {
			names = append(names, "")
		}
	}

	if len(names) == 0 && err == nil {
		err = errors.New("no VMs to launch")
	}

	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	for _, name := range names {
		if isReserved(name) {
			resp.Error = fmt.Sprintf("`%s` is a reserved word -- cannot use for vm name", name)
			return resp
		}
	}

	log.Info("launching %v vms", len(names))

	ack := make(chan int)
	waitForAcks := func(count int) {
		// get acknowledgements from each vm
		for i := 0; i < count; i++ {
			log.Debug("launch ack from VM %v", <-ack)
		}
	}

	for i, vmName := range names {
		if err := vms.launch(vmName, ack); err != nil {
			resp.Error = err.Error()
			go waitForAcks(i)
			return resp
		}
	}

	if c.BoolArgs["noblock"] {
		go waitForAcks(len(names))
	} else {
		waitForAcks(len(names))
	}

	return resp
}

func cliVmApply(c *minicli.Command, fn func(string) []error) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	errs := fn(c.StringArgs["target"])
	if len(errs) > 0 {
		resp.Error = errSlice(errs).String()
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
			status, complete, err := vm.QueryMigrate()
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			if status == "" {
				continue
			}
			resp.Tabular = append(resp.Tabular, []string{fmt.Sprintf("%v", vm.ID), vm.Name, status, fmt.Sprintf("%.2f", complete)})
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

	if c.BoolArgs["add"] {
		// generate an id by adding 1 to the highest in the list for the
		// Hotplug devices, 0 if it's empty
		id := 0
		for k, _ := range vm.Hotplug {
			if k >= id {
				id = k + 1
			}
		}
		hid := fmt.Sprintf("hotplug%v", id)
		log.Debugln("hotplug generated id:", hid)

		r, err := vm.q.DriveAdd(hid, c.StringArgs["filename"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		log.Debugln("hotplug drive_add response:", r)
		r, err = vm.q.USBDeviceAdd(hid)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		log.Debugln("hotplug usb device add response:", r)
		vm.Hotplug[id] = c.StringArgs["filename"]
	} else if c.BoolArgs["remove"] {
		if c.StringArgs["disk"] == Wildcard {
			for k := range vm.Hotplug {
				if err := vm.hotplugRemove(k); err != nil {
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
		} else if err := vm.hotplugRemove(id); err != nil {
			resp.Error = err.Error()
		}
	} else if c.BoolArgs["show"] {
		if len(vm.Hotplug) > 0 {
			resp.Header = []string{"Hotplug ID", "File"}
			resp.Tabular = [][]string{}

			for k, v := range vm.Hotplug {
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
	if len(vm.Taps) < pos {
		resp.Error = fmt.Sprintf("no such network %v, VM only has %v networks", pos, len(vm.Taps))
		return resp
	}

	var b *bridge
	if c.StringArgs["bridge"] != "" {
		b, err = getBridge(c.StringArgs["bridge"])
	} else {
		b, err = getBridge(vm.Bridges[pos])
	}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if c.BoolArgs["disconnect"] {
		log.Debug("disconnect network connection: %v %v %v", vm.ID, pos, vm.Networks[pos])
		err = b.TapRemove(vm.Networks[pos], vm.Taps[pos])
		if err != nil {
			resp.Error = err.Error()
		} else {
			vm.Networks[pos] = -1
		}
	} else if c.BoolArgs["connect"] {
		net, err := strconv.Atoi(c.StringArgs["vlan"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		if net >= 0 && net < 4096 {
			// new network
			log.Debug("moving network connection: %v %v %v -> %v %v", vm.ID, pos, vm.Networks[pos], b.Name, net)
			oldBridge, err := getBridge(vm.Bridges[pos])
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			if vm.Networks[pos] != -1 {
				err := oldBridge.TapRemove(vm.Networks[pos], vm.Taps[pos])
				if err != nil {
					resp.Error = err.Error()
					return resp
				}
			}

			err = b.TapAdd(net, vm.Taps[pos], false)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			vm.Networks[pos] = net
			vm.Bridges[pos] = b.Name
		} else {
			resp.Error = fmt.Sprintf("invalid vlan tag %v", net)
		}
	}

	return resp
}
