package main

import (
	"log"
	"minicli"
)

var vmCommands = []struct {
	HelpShort string
	HelpLong  string
	Patterns  []string
	Call      func(*minicli.Command) *minicli.Responses
}{
	{ // vm info
		"print information about VMs",
		`
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
		[]string{
			"vm info",
			"vm info search <terms>",
			"vm info search <terms> mask <masks>",
			"vm info mask <masks>",
		},
		nil, // TODO
	},
	{ // vm save
		"save a vm configuration for later use",
		`
Saves the configuration of a running virtual machine or set of virtual
machines so that it/they can be restarted/recovered later, such as after
a system crash.

If no VM name or ID is given, all VMs (including those in the quit and error state) will be saved.

This command does not store the state of the virtual machine itself,
only its launch configuration.`,
		[]string{
			"vm save <name> <vm id or name>",
		},
		nil, // TODO
	},
	{ // vm launch
		"launch virtual machines in a paused state",
		`
Launch virtual machines in a paused state, using the parameters defined
leading up to the launch command. Any changes to the VM parameters after
launching will have no effect on launched VMs.

If you supply a name instead of a number of VMs, one VM with that name
will be launched.

The optional 'noblock' suffix forces minimega to return control of the
command line immediately instead of waiting on potential errors from
launching the VM(s). The user must check logs or error states from
vm_info.`,
		[]string{
			"vm launch name <namespec> [noblock,]",
			"vm launch count <count> [noblock,]",
		},
		nil, // TODO
	},
	{ // vm kill
		"kill running virtual machines",
		`
Kill a virtual machine by ID or name. Pass -1 to kill all virtual machines.`,
		[]string{
			"vm kill <vm id or name or *>",
		},
		nil, // TODO
	},
	{ // vm start
		"start paused virtual machines",
		`
Start all or one paused virtual machine. To start all paused virtual machines,
call start without the optional VM ID or name.

Calling vm_start specifically on a quit VM will restart the VM. If the
'quit=true' argument is passed when using vm_start with no specific VM, all VMs
in the quit state will also be restarted.`,
		[]string{
			"vm start <vm id or name or *>",
		},
		nil, // TODO
	},
	{ // vm stop
		"stop/pause virtual machines",
		`
Stop all or one running virtual machine. To stop all running virtual machines,
call stop without the optional VM ID or name.

Calling stop will put VMs in a paused state. Start stopped VMs with vm_start.`,
		[]string{
			"vm stop <vm id or name or *>",
		},
		nil, // TODO
	},
	{ // vm flush
		"discard information about quit or failed VMs",
		`
Discard information about VMs that have either quit or encountered an error.
This will remove any VMs with a state of "quit" or "error" from vm_info. Names
of VMs that have been flushed may be reused.`,
		[]string{
			"vm flush",
		},
		nil, // TODO
	},
	{ // vm hotplug
		"add and remove USB drives",
		`
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
		[]string{
			"vm hotplug show <vm id or name>",
			"vm hotplug add <vm id or name> <filename>",
			"vm hotplug remove <vm id or name> <disk id>",
			"clear vm hotplug", // TODO: where does this belong?
		},
		nil, // TODO
	},
	{ // vm net
		"disconnect or move network connections",
		`
Disconnect or move existing network connections on a running VM.

Network connections are indicated by their position in vm_net (same order in
vm_info) and are zero indexed. For example, to disconnect the first network
connection from a VM with 4 network connections:

	vm_netmod <vm name or id> 0 disconnect

To disconnect the second connection:

	vm_netmod <vm name or id> 1 disconnect

To move a connection, specify the new VLAN tag and bridge:

	vm_netmod <vm name or id> 0 bridgeX 100`,
		[]string{
			"vm net <vm id or name>",
			"vm net connect <vm id or name> <tap position> <bridge> <vlan>",
			"vm net disconnect <vm id or name> <tap position>",
			"clear vm net", // TODO: where does this belong?
		},
		nil, // TODO
	},
	{ // vm inject
		"inject files into a qcow image",
		`
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
		[]string{
			`vm inject src <srcimg> <files like /path/to/src:/path/to/dst>...`,
			`vm inject dst <dstimg> src <srcimg> <files like /path/to/src:/path/to/dst>...`,
		},
		nil, // TODO
	},
	{ // vm qmp
		"issue a JSON-encoded QMP command",
		`
Issue a JSON-encoded QMP command. This is a convenience function for accessing
the QMP socket of a VM via minimega. vm_qmp takes two arguments, a VM ID or
name, and a JSON string, and returns the JSON encoded response. For example:

	minimega$ vm_qmp 0 { "execute": "query-status" }
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		[]string{
			"vm qmp <qmp command>",
		},
		nil, // TODO
	},
	{ // vm config
		"display, save, or restore the current VM configuration",
		`
Display, save, or restore the current VM configuration.

To display the current configuration, call vm_config with no arguments.

List the current saved configurations with 'vm_config show'

To save a configuration:

	vm_config save <config name>

To restore a configuration:

	vm_config restore <config name>

Calling clear vm_config will clear all VM configuration options, but will not
remove saved configurations.`,
		[]string{
			"vm config",
			"vm config save <name>",
			"vm config restore <name>",
			"vm config clone <vm id or name>",
		},
		nil, // TODO
	},
	{ // vm config qemu
		"set the QEMU process to invoke. Relative paths are ok.",
		"Set the QEMU process to invoke. Relative paths are ok.",
		[]string{
			"vm config qemu [path to qemu]",
		},
		nil, // TODO
	},
	{ // vm config qemu-override
		"override parts of the QEMU launch string",
		`
Override parts of the QEMU launch string by supplying a string to match, and a
replacement string.`,
		[]string{
			"vm config qemu-override",
			"vm config qemu-override add <match> <replacement>",
			"vm config qemu-override delete <id or *>",
		},
		nil, // TODO
	},
	{ // vm config qemu-append
		"add additional arguments to the QEMU command",
		`
Add additional arguments to be passed to the QEMU instance. For example:
	vm config qemu-append -serial tcp:localhost:4001`,
		[]string{
			"vm config qemu-append <argument>...",
		},
		nil, // TODO
	},
	{ // vm config memory
		"set the amount of physical memory for a VM",
		`
Set the amount of physical memory to allocate in megabytes.`,
		[]string{
			"vm config memory [memory in megabytes]",
		},
		nil, // TODO
	},
	{ // vm config vcpus
		"set the number of virtual CPUs for a VM",
		`
Set the number of virtual CPUs to allocate for a VM.`,
		[]string{
			"vm config vcpus [number of CPUs]",
		},
		nil, // TODO
	},
	{ // vm config disk
		"set disk images to attach to a VM",
		`
Attach one or more disks to a vm. Any disk image supported by QEMU is a valid
parameter.  Disk images launched in snapshot mode may safely be used for
multiple VMs.`,
		[]string{
			"vm config disk [path to disk image]...",
		},
		nil, // TODO
	},
	{ // vm config cdrom
		"set a cdrom image to attach to a VM",
		`
Attach a cdrom to a VM. When using a cdrom, it will automatically be set
to be the boot device.`,
		[]string{
			"vm config cdrom [path to cdrom image]",
		},
		nil, // TODO
	},
	{ // vm config kernel
		"set a kernel image to attach to a VM",
		`
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.`,
		[]string{
			"vm config kernel [path to kernel]",
		},
		nil, // TODO
	},
	{ // vm config append
		"set an append string to pass to a kernel set with vm kernel",
		`
Add an append string to a kernel set with vm kernel. Setting vm append without
using vm kernel will result in an error.

For example, to set a static IP for a linux VM:
	vm append ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10`,
		[]string{
			"vm config append <argument>...",
		},
		nil, // TODO
	},
	{ // vm config uuid
		"set the UUID for a VM",
		`
Set the UUID for a virtual machine. If not set, minimega will create a random
one when the VM is launched.`,
		[]string{
			"vm config uuid [uuid]",
		},
		nil, // TODO
	},
	{ // vm config net
		"specific the networks a VM is a member of",
		`
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
		[]string{
			"vm config net <netspec>...",
		},
		nil, // TODO
	},
	{ // vm config snapshot
		"enable or disable snapshot mode when using disk images",
		`
Enable or disable snapshot mode when using disk images. When enabled, disks
images will be loaded in memory when run and changes will not be saved. This
allows a single disk image to be used for many VMs.`,
		[]string{
			"vm config snapshot [true,false]",
		},
		nil, // TODO
	},
	{ // clear vm config
		"reset vm config to the default value",
		`
Resets the configuration for a provided field (or the whole configuration) back
to the default value.`,
		[]string{
			"clear vm config",
			"clear vm config qemu",
			"clear vm config qemu-override",
			"clear vm config qemu-append",
			"clear vm config memory",
			"clear vm config vcpus",
			"clear vm config disk",
			"clear vm config cdrom",
			"clear vm config kernel",
			"clear vm config append",
			"clear vm config uuid",
			"clear vm config net",
			"clear vm config snapshot",
		},
		nil, // TODO
	},
}

func init() {
	for _, cmd := range vmCommands {
		for _, pattern := range cmd.Patterns {
			handler := &minicli.Handler{
				Pattern:   pattern,
				HelpShort: cmd.HelpShort,
				HelpLong:  cmd.HelpLong,
				Call:      cmd.Call}

			err := minicli.Register(handler)
			if err != nil {
				log.Fatalf("invalid pattern: %s -- %s", pattern, err.Error())
			}
		}
	}
}
