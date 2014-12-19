package main

import (
	"fmt"
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
Print information about VMs. vm_info allows searching for VMs based on any VM
parameter, and output some or all information about the VMs in question.
Additionally, you can display information about all running VMs.

A vm_info command takes three optional arguments, an output mode, a search
term, and an output mask. If the search term is omitted, information about all
VMs will be displayed. If the output mask is omitted, all information about the
VMs will be displayed.

The output mode has two options - quiet and json. Two use either, set the output using the following syntax:

	vm_info output=quiet ...

If the output mode is set to 'quiet', the header and "|" characters in the output formatting will be removed. The output will consist simply of tab delimited lines of VM info based on the search and mask terms.

If the output mode is set to 'json', the output will be a json formatted string containing info on all VMs, or those matched by the search term. The mask will be ignored - all fields will be populated.

The search term uses a single key=value argument. For example, if you want all
information about VM 50:

	vm_info id=50

The output mask uses an ordered list of fields inside [] brackets. For example,
if you want the ID and IPs for all VMs on vlan 100:

	vm_info vlan=100 [id,ip]

Searchable and maskable fields are:

- host	  : the host that the VM is running on
- id	  : the VM ID, as an integer
- name	  : the VM name, if it exists
- memory  : allocated memory, in megabytes
- vcpus   : the number of allocated CPUs
- disk    : disk image
- initrd  : initrd image
- kernel  : kernel image
- cdrom   : cdrom image
- state   : one of (building, running, paused, quit, error)
- tap	  : tap name
- mac	  : mac address
- ip	  : IPv4 address
- ip6	  : IPv6 address
- vlan	  : vlan, as an integer
- bridge  : bridge name
- append  : kernel command line string

Examples:

Display a list of all IPs for all VMs:
	vm_info [ip,ip6]

Display all information about VMs with the disk image foo.qc2:
	vm_info disk=foo.qc2

Display all information about all VMs:
	vm_info`,
		Patterns: []string{
			"vm info",
			"vm info search <terms>",
			"vm info search <terms> mask <masks>",
			"vm info mask <masks>",
		},
		Call: cliVmInfo, // TODO
	},
	{ // vm save
		HelpShort: "save a vm configuration for later use",
		HelpLong: `
Saves the configuration of a running virtual machine or set of virtual
machines so that it/they can be restarted/recovered later, such as after
a system crash.

If no VM name or ID is given, all VMs (including those in the quit and error state) will be saved.

This command does not store the state of the virtual machine itself,
only its launch configuration.`,
		Patterns: []string{
			"vm save <name> <vm id or name or *>...",
		},
		Call: cliVmSave,
	},
	{ // vm launch
		HelpShort: "launch virtual machines in a paused state",
		HelpLong: `
Launch virtual machines in a paused state, using the parameters defined
leading up to the launch command. Any changes to the VM parameters after
launching will have no effect on launched VMs.

If you supply a name instead of a number of VMs, one VM with that name
will be launched.

The optional 'noblock' suffix forces minimega to return control of the
command line immediately instead of waiting on potential errors from
launching the VM(s). The user must check logs or error states from
vm_info.`,
		Patterns: []string{
			"vm launch name <namespec> [noblock,]",
			"vm launch count <count> [noblock,]",
		},
		Call: cliVmLaunch,
	},
	{ // vm kill
		HelpShort: "kill running virtual machines",
		HelpLong: `
Kill a virtual machine by ID or name. Pass -1 to kill all virtual machines.`,
		Patterns: []string{
			"vm kill <vm id or name or *>",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmApply(c, func() []error {
				return vms.kill(c.StringArgs["vm"])
			})
		},
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: `
Start all or one paused virtual machine. To start all paused virtual machines,
call start without the optional VM ID or name.

Calling vm_start specifically on a quit VM will restart the VM. If the
'quit=true' argument is passed when using vm_start with no specific VM, all VMs
in the quit state will also be restarted.`,
		Patterns: []string{
			"vm start <vm id or name or *> [quit,]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmApply(c, func() []error {
				return vms.start(c.StringArgs["vm"], c.BoolArgs["quit"])
			})
		},
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop all or one running virtual machine. To stop all running virtual machines,
call stop without the optional VM ID or name.

Calling stop will put VMs in a paused state. Start stopped VMs with vm_start.`,
		Patterns: []string{
			"vm stop <vm id or name or *>",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmApply(c, func() []error {
				return vms.stop(c.StringArgs["vm"])
			})
		},
	},
	{ // vm flush
		HelpShort: "discard information about quit or failed VMs",
		HelpLong: `
Discard information about VMs that have either quit or encountered an error.
This will remove any VMs with a state of "quit" or "error" from vm_info. Names
of VMs that have been flushed may be reused.`,
		Patterns: []string{
			"vm flush",
		},
		Call: cliVmFlush,
	},
	{ // vm hotplug
		HelpShort: "add and remove USB drives",
		HelpLong: `
Add and remove USB drives to a launched VM.

To view currently attached media, call vm_hotplug with the 'show' argument and
a VM ID or name. To add a device, use the 'add' argument followed by the VM ID
or name, and the name of the file to add. For example, to add foo.img to VM 5:

	vm_hotplug add 5 foo.img

The add command will assign a disk ID, shown in vm_hotplug show. To remove
media, use the 'remove' argument with the VM ID and the disk ID. For example,
to remove the drive added above, named 0:

	vm_hotplug remove 5 0

To remove all hotplug devices, use ID -1.`,
		Patterns: []string{
			"vm hotplug show <vm id or name>",
			"vm hotplug add <vm id or name> <filename>",
			"vm hotplug remove <vm id or name> <disk id>",
			"clear vm hotplug", // TODO: where does this belong?
		},
		Call: nil, // TODO
	},
	{ // vm net
		HelpShort: "disconnect or move network connections",
		HelpLong: `
Disconnect or move existing network connections on a running VM.

Network connections are indicated by their position in vm_net (same order in
vm_info) and are zero indexed. For example, to disconnect the first network
connection from a VM with 4 network connections:

	vm_netmod <vm name or id> 0 disconnect

To disconnect the second connection:

	vm_netmod <vm name or id> 1 disconnect

To move a connection, specify the new VLAN tag and bridge:

	vm_netmod <vm name or id> 0 bridgeX 100`,
		Patterns: []string{
			"vm net <vm id or name>",
			"vm net connect <vm id or name> <tap position> <bridge> <vlan>",
			"vm net disconnect <vm id or name> <tap position>",
			"clear vm net", // TODO: where does this belong?
		},
		Call: nil, // TODO
	},
	{ // vm inject
		HelpShort: "inject files into a qcow image",
		HelpLong: `
Create a backed snapshot of a qcow2 image and injects one or more files into
the new snapshot.

src qcow image - the name of the qcow to use as the backing image file.

partition - The optional partition number in which the files should be
injected. Partition defaults to 1, but if multiple partitions exist and
partition is not explicitly specified, an error is thrown and files are not
injected.

dst qcow image name - The optional name of the snapshot image. This should be a
name only, if any extra path is specified, an error is thrown. This file will
be created at 'base'/files. A filename will be generated if this optional
parameter is omitted.

src file - The local file that should be injected onto the new qcow2 snapshot.

dst file - The path where src file should be injected in the new qcow2 snapshot.

If the src file or dst file contains spaces, use double quotes (" ") as in the
following example:

	vm_inject src.qc2 dst.qc2 "my file":"Program Files/my file"

Alternatively, when given a single argument, this command supplies the name of
the backing qcow image for a snapshot image.`,
		Patterns: []string{
			`vm inject src <srcimg> <files like /path/to/src:/path/to/dst>...`,
			`vm inject dst <dstimg> src <srcimg> <files like /path/to/src:/path/to/dst>...`,
		},
		Call: nil, // TODO
	},
	{ // vm qmp
		HelpShort: "issue a JSON-encoded QMP command",
		HelpLong: `
Issue a JSON-encoded QMP command. This is a convenience function for accessing
the QMP socket of a VM via minimega. vm_qmp takes two arguments, a VM ID or
name, and a JSON string, and returns the JSON encoded response. For example:

	minimega$ vm_qmp 0 '{ "execute": "query-status" }'
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		Patterns: []string{
			"vm qmp <vm id or name> <qmp command>",
		},
		Call: cliVmQmp,
	},
	{ // vm config
		HelpShort: "display, save, or restore the current VM configuration",
		HelpLong: `
Display, save, or restore the current VM configuration.

To display the current configuration, call vm_config with no arguments.

List the current saved configurations with 'vm_config show'

To save a configuration:

	vm_config save <config name>

To restore a configuration:

	vm_config restore <config name>

Calling clear vm_config will clear all VM configuration options, but will not
remove saved configurations.`,
		Patterns: []string{
			"vm config",
			"vm config <save,> <name>",
			"vm config <restore,> [name]",
			"vm config <clone,> <vm id or name>",
		},
		Call: cliVmConfig,
	},
	{ // vm config qemu
		HelpShort: "set the QEMU process to invoke. Relative paths are ok.",
		HelpLong:  "Set the QEMU process to invoke. Relative paths are ok.",
		Patterns: []string{
			"vm config qemu [path to qemu]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "qemu")
		},
	},
	{ // vm config qemu-override
		HelpShort: "override parts of the QEMU launch string",
		HelpLong: `
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string.`,
		Patterns: []string{
			"vm config qemu-override",
			"vm config qemu-override add <match> <replacement>",
			"vm config qemu-override delete <id or *>",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "qemu-override")
		},
	},
	{ // vm config qemu-append
		HelpShort: "add additional arguments to the QEMU command",
		HelpLong: `
Add additional arguments to be passed to the QEMU instance. For example:
	vm config qemu-append -serial tcp:localhost:4001`,
		Patterns: []string{
			"vm config qemu-append [argument]...",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "qemu-append")
		},
	},
	{ // vm config memory
		HelpShort: "set the amount of physical memory for a VM",
		HelpLong: `
Set the amount of physical memory to allocate in megabytes.`,
		Patterns: []string{
			"vm config memory [memory in megabytes]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "memory")
		},
	},
	{ // vm config vcpus
		HelpShort: "set the number of virtual CPUs for a VM",
		HelpLong: `
Set the number of virtual CPUs to allocate for a VM.`,
		Patterns: []string{
			"vm config vcpus [number of CPUs]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "vcpus")
		},
	},
	{ // vm config disk
		HelpShort: "set disk images to attach to a VM",
		HelpLong: `
Attach one or more disks to a vm. Any disk image supported by QEMU is a valid
parameter.  Disk images launched in snapshot mode may safely be used for
multiple VMs.`,
		Patterns: []string{
			"vm config disk [path to disk image]...",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "disk")
		},
	},
	{ // vm config cdrom
		HelpShort: "set a cdrom image to attach to a VM",
		HelpLong: `
Attach a cdrom to a VM. When using a cdrom, it will automatically be set
to be the boot device.`,
		Patterns: []string{
			"vm config cdrom [path to cdrom image]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "cdrom")
		},
	},
	{ // vm config kernel
		HelpShort: "set a kernel image to attach to a VM",
		HelpLong: `
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.`,
		Patterns: []string{
			"vm config kernel [path to kernel]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "kernel")
		},
	},
	{ // vm config append
		HelpShort: "set an append string to pass to a kernel set with vm kernel",
		HelpLong: `
Add an append string to a kernel set with vm kernel. Setting vm append without
using vm kernel will result in an error.

For example, to set a static IP for a linux VM:
	vm append ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10`,
		Patterns: []string{
			"vm config append [argument]...",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "append")
		},
	},
	{ // vm config uuid
		HelpShort: "set the UUID for a VM",
		HelpLong: `
Set the UUID for a virtual machine. If not set, minimega will create a random
one when the VM is launched.`,
		Patterns: []string{
			"vm config uuid [uuid]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "uuid")
		},
	},
	{ // vm config net
		HelpShort: "specific the networks a VM is a member of",
		HelpLong: `
Specify the network(s) that the VM is a member of by VLAN. A corresponding VLAN
will be created for each network. Optionally, you may specify the bridge the
interface will be connected on. If the bridge name is omitted, minimega will
use the default 'mega_bridge'. You can also optionally specify the mac
address of the interface to connect to that network. If not specifed, the mac
address will be randomly generated. Additionally, you can optionally specify a
driver for qemu to use. By default, e1000 is used.

Examples:

To connect a VM to VLANs 1 and 5:
	vm_net 1 5
To connect a VM to VLANs 100, 101, and 102 with specific mac addresses:
	vm_net 100,00:00:00:00:00:00 101,00:00:00:00:01:00 102,00:00:00:00:02:00
To connect a VM to VLAN 1 on bridge0 and VLAN 2 on bridge1:
	vm_net bridge0,1 bridge1,2
To connect a VM to VLAN 100 on bridge0 with a specific mac:
	vm_net bridge0,100,00:11:22:33:44:55
To specify a specific driver, such as i82559c:
	vm_net 100,i82559c

Calling vm_net with no parameters will list the current networks for this VM.`,
		Patterns: []string{
			"vm config net [netspec]...",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "net")
		},
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
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "snapshot")
		},
	},
	{ // vm config initrd
		HelpShort: "set a initrd image to attach to a VM",
		HelpLong: `
Attach an initrd image to a VM. Passed along with the kernel image at
boot time.`,
		Patterns: []string{
			"vm config initrd [path to initrd]",
		},
		Call: func(c *minicli.Command) minicli.Responses {
			return cliVmConfigField(c, "initrd")
		},
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
		Call: cliClearVmConfig,
	},
}

func init() {
	for i := range vmCLIHandlers {
		if err := minicli.Register(&vmCLIHandlers[i]); err != nil {
			fmt.Println("invalid handler: %#v -- %s", vmCLIHandlers[i], err.Error())
		}
	}
}

func cliVmInfo(c *minicli.Command) minicli.Responses {
	var err error
	resp := &minicli.Response{Host: hostname}

	search := c.StringArgs["search"]
	mask := c.StringArgs["mask"]

	// output mask
	if mask != "" {
		d := strings.Split(mask, ",")
		for _, j := range d {
			name := strings.ToLower(j)
			if _, ok := vmMasks[name]; ok {
				resp.Header = append(resp.Header, name)
			} else {
				resp.Error = fmt.Sprintf("invalid output mask: %v", j)
				resp.Header = nil
				return minicli.Responses{resp}
			}
		}
	} else { // print everything
		for name := range vmMasks {
			resp.Header = append(resp.Header, name)
		}
	}

	resp.Tabular, err = vms.info(resp.Header, search)
	if err != nil {
		resp.Error = err.Error()
		resp.Header = nil
		return minicli.Responses{resp}
	}

	return minicli.Responses{resp}
}

func cliVmConfig(c *minicli.Command) minicli.Responses {
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
		name := c.StringArgs["vm"]

		id, err := strconv.Atoi(name)
		if err != nil {
			id = vms.findByName(name)
		}

		if vm, ok := vms.vms[id]; ok {
			info = vm.Copy()
		} else {
			resp.Error = fmt.Sprintf("vm %v not found", name)
		}
	} else {
		// Print the full config
		resp.Response = info.configToString()
	}

	return minicli.Responses{resp}
}

func cliVmConfigField(c *minicli.Command, field string) minicli.Responses {
	var err error
	resp := &minicli.Response{Host: hostname}

	fns := vmConfigFns[field]

	// If there are no args it means that we want to display the current value
	if len(c.StringArgs) == 0 && len(c.ListArgs) == 0 && len(c.BoolArgs) == 0 {
		resp.Response = fns.Print(info)
		return minicli.Responses{resp}
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
		panic("someone goofed on the patterns")
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return minicli.Responses{resp}
}

func cliClearVmConfig(c *minicli.Command) minicli.Responses {
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
		panic("no callback defined for clear")
	}

	return minicli.Responses{resp}
}

func cliVmLaunch(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	vmNames := []string{}

	if namespec, ok := c.StringArgs["namespec"]; ok {
		index := strings.IndexRune(namespec, '[')
		if index == -1 {
			vmNames = append(vmNames, namespec)
		} else {
			r, err := ranges.NewRange(namespec[:index], 0, int(math.MaxInt32))
			if err != nil {
				panic(err)
			}

			names, err := r.SplitRange(namespec)
			if err != nil {
				resp.Error = err.Error()
				return minicli.Responses{resp}
			}
			vmNames = append(vmNames, names...)
		}
	} else if countStr, ok := c.StringArgs["count"]; ok {
		count, err := strconv.ParseUint(countStr, 10, 32)
		if err != nil {
			resp.Error = err.Error()
			return minicli.Responses{resp}
		}

		for i := uint64(0); i < count; i++ {
			vmNames = append(vmNames, "")
		}
	}

	if len(vmNames) == 0 {
		resp.Error = "No VMs to launch"
		return minicli.Responses{resp}
	}

	log.Info("launching %v vms", len(vmNames))

	ack := make(chan int)
	waitForAcks := func(count int) {
		// get acknowledgements from each vm
		for i := 0; i < count; i++ {
			log.Debug("launch ack from VM %v", <-ack)
		}
	}

	numVMs := len(vmNames)
	for _, vmName := range vmNames {
		if err := vms.launch(vmName, ack); err != nil {
			resp.Error += fmt.Sprintln(err)
			numVMs -= 1
		}
	}

	resp.Response = fmt.Sprintf("launching %d vms", numVMs)

	if c.BoolArgs["noblock"] {
		go waitForAcks(numVMs)
	} else {
		waitForAcks(numVMs)
	}

	return minicli.Responses{resp}
}

func cliVmApply(c *minicli.Command, fn func() []error) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	for _, err := range fn() {
		if err != nil {
			resp.Error += fmt.Sprintln(err)
		}
	}

	return minicli.Responses{resp}
}

func cliVmFlush(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	vms.flush()

	return minicli.Responses{resp}
}

func cliVmQmp(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	var err error
	resp.Response, err = vms.qmp(c.StringArgs["vm"], c.StringArgs["qmp"])
	if err != nil {
		resp.Error = err.Error()
	}

	return minicli.Responses{resp}
}

func cliVmSave(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	path := filepath.Join(*f_base, "saved_vms")
	err := os.MkdirAll(path, 0775)
	if err != nil {
		log.Error("mkdir: %v", err)
		// TODO: do we really want to teardown minimega?
		teardown()
	}

	name := c.StringArgs["name"]
	file, err := os.Create(filepath.Join(path, name))
	if err != nil {
		resp.Error = err.Error()
		return minicli.Responses{resp}
	}

	err = vms.save(file, c.ListArgs["vm"])
	if err != nil {
		resp.Error = err.Error()
	}

	return minicli.Responses{resp}
}
