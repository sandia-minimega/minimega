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
Print information about VMs. vm info allows searching for VMs based on any VM
parameter, and output some or all information about the VMs in question.
Additionally, you can display information about all running VMs.

A vm info command takes two optional arguments: a search term and an output
mask. If the search term is omitted, information about all VMs will be
displayed. If the output mask is omitted, all information about the VMs will be
displayed.

The search term uses a single key=value argument. For example, if you want all
information about VM 50:

	vm info search id=50

The output mask uses an ordered comma-seperated list of fields. For example, if
you want the ID and IPs for all VMs on vlan 100:

	vm info search vlan=100 mask id,ip

Searchable and maskable fields are:

- id	    : the VM ID, as an integer
- host	    : the host that the VM is running on
- name	    : the VM name, if it exists
- state     : one of (building, running, paused, quit, error)
- memory    : allocated memory, in megabytes
- vcpus     : the number of allocated CPUs
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

Examples:

Display a list of all IPs for all VMs:
	vm info masks ip,ip6

Display all information about VMs with the disk image foo.qc2:
	vm info search disk=foo.qc2

Display all information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info",
			"vm info search <terms>",
			"vm info search <terms> mask <masks>",
			"vm info mask <masks>",
		},
		Call: wrapSimpleCLI(cliVmInfo),
	},
	{ // vm save
		HelpShort: "save a vm configuration for later use",
		HelpLong: `
Saves the configuration of a running virtual machine or set of virtual machines
so that it/they can be restarted/recovered later, such as after a system crash.

If no VM name or ID is given, all VMs (including those in the quit and error
state) will be saved.

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

Calling vm start specifically on a quit VM will restart the VM. If the optional 'quit'
suffix is used with the wildcard, then all virtual machines in the paused *or* quit state
will be restarted.`,
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

To remove all hotplug devices, use ID * for the disk ID.`,
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

	minimega$ vm qmp 0 '{ "execute": "query-status" }'
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		Patterns: []string{
			"vm qmp <vm id or name> <qmp command>",
		},
		Call: wrapSimpleCLI(cliVmQmp),
	},
	{ // vm config
		HelpShort: "display, save, or restore the current VM configuration",
		HelpLong: `
Display, save, or restore the current VM configuration.

To display the current configuration, call vm config with no arguments.

List the current saved configurations with 'vm config show'

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
}

func init() {
	registerHandlers("vm", vmCLIHandlers)
}

func cliVmInfo(c *minicli.Command) *minicli.Response {
	var err error
	resp := &minicli.Response{Host: hostname}

	search := c.StringArgs["search"]

	masks := vmMasks
	if c.StringArgs["masks"] != "" {
		masks = strings.Split(c.StringArgs["masks"], ",")
	}

	log.Debug("vm info masks: %#v", masks)

	resp.Tabular, err = vms.info(masks, search)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	resp.Header = masks

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
		panic("someone goofed on the patterns")
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
		panic("no callback defined for clear")
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
				panic(err)
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

func cliVmSave(c *minicli.Command) *minicli.Response {
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
	if len(vm.taps) < pos {
		resp.Error = fmt.Sprintf("no such network %v, VM only has %v networks", pos, len(vm.taps))
		return resp
	}

	var b *bridge
	if c.StringArgs["bridge"] != "" {
		b, err = getBridge(c.StringArgs["bridge"])
	} else {
		b, err = getBridge(vm.bridges[pos])
	}
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if c.BoolArgs["disconnect"] {
		log.Debug("disconnect network connection: %v %v %v", vm.Id, pos, vm.Networks[pos])
		err = b.TapRemove(vm.Networks[pos], vm.taps[pos])
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
			log.Debug("moving network connection: %v %v %v -> %v %v", vm.Id, pos, vm.Networks[pos], b.Name, net)
			oldBridge, err := getBridge(vm.bridges[pos])
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			if vm.Networks[pos] != -1 {
				err := oldBridge.TapRemove(vm.Networks[pos], vm.taps[pos])
				if err != nil {
					resp.Error = err.Error()
					return resp
				}
			}

			err = b.TapAdd(net, vm.taps[pos], false)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			vm.Networks[pos] = net
			vm.bridges[pos] = b.Name
		} else {
			resp.Error = fmt.Sprintf("invalid vlan tag %v", net)
		}
	}

	return resp
}
