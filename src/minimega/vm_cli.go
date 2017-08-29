// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var vmCLIHandlers = []minicli.Handler{
	{ // vm info
		HelpShort: "print information about VMs",
		HelpLong: `
Print information about VMs in tabular form. The .filter and .columns commands
can be used to subselect a set of rows and/or columns. See the help pages for
.filter and .columns, respectively, for their usage. Columns returned by VM
info include:

- id*        : the VM ID, as an integer
- name*      : the VM name, if it exists
- state*     : one of (building, running, paused, quit, error)
- uptime     : amount of time since the VM was launched
- namespace* : namespace the VM belongs to
- type*      : one of (kvm, container)
- uuid*      : QEMU system uuid
- cc_active* : indicates whether cc is connected
- vcpus      : the number of allocated CPUs
- memory     : allocated memory, in megabytes
- vlan*      : vlan, as an integer
- bridge     : bridge name
- tap        : tap name
- mac        : mac address
- ip         : IPv4 address
- ip6        : IPv6 address
- bandwidth  : stats regarding bandwidth usage
- qos        : quality-of-service contraints on network interfaces
- tags       : any additional information attached to the VM

Additional fields are available for KVM-based VMs:

- append        : kernel command line string
- cdrom         : cdrom image
- disk          : disk image
- kernel        : kernel image
- initrd        : initrd image
- migrate       : qemu migration image
- pid           : pid of qemu process
- serial        : number of serial ports
- virtio-serial : number of virtio ports
- vnc_port      : port for VNC shim

Additional fields are available for container-based VMs:

- filesystem   : root filesystem for the container
- hostname     : hostname of the container
- init	       : process to invoke as init
- preinit      : process to invoke at container launch before isolation
- pid          : pid of container's init process
- fifo         : number of fifo devices
- console_port : port for console shim

The optional summary flag limits the columns to those denoted with a '*'.

Examples:

Display a list of all IPs for all VMs:
	.columns ip,ip6 vm info

Display information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info [summary,]",
		},
		Call: wrapBroadcastCLI(cliVmInfo),
	},
	{ // vm launch
		HelpShort: "launch virtual machines in a paused state",
		HelpLong: fmt.Sprintf(`
Launch virtual machines in a paused state, using the parameters defined leading
up to the launch command. Any changes to the VM parameters after launching will
have no effect on launched VMs.

When you launch a VM, you supply the type of VM in the launch command.
Currently, the supported VM types are:

- kvm : QEMU-based vms
- container: Linux containers

If you supply a name instead of a number of VMs, one VM with that name will be
launched. You may also supply a range expression to launch VMs with a specific
naming scheme:

	vm launch kvm foo[0-9]

Note: VM names cannot be integers or reserved words (e.g. "%[1]s").

The optional 'noblock' suffix forces minimega to return control of the command
line immediately instead of waiting on potential errors from launching the
VM(s). The user must check logs or error states from vm info.

The launch behavior changes when namespace are active. If a namespace is
active, invocations that include the VM type and the name or number of VMs will
be queue until a subsequent invocation that does not include any arguments.
This allows the scheduler to better allocate resources across the cluster. The
'noblock' suffix is ignored when namespaces are active.`, Wildcard),
		Patterns: []string{
			"vm launch",
			"vm launch <kvm,> <name or count> [noblock,]",
			"vm launch <container,> <name or count> [noblock,]",
		},
		Call: wrapSimpleCLI(cliVmLaunch),
	},
	{ // vm kill
		HelpShort: "kill running virtual machines",
		HelpLong: `
Kill one or more running virtual machines. See "vm start" for a full
description of allowable targets.`,
		Patterns: []string{
			"vm kill <target>",
		},
		Call: wrapVMTargetCLI(cliVmKill),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: fmt.Sprintf(`
Start one or more paused virtual machines. VMs may be selected by name, range, or
wildcard. For example,

To start vm foo:

		vm start foo

To start vms foo and bar:

		vm start foo,bar

To start vms foo0, foo1, foo2, and foo5:

		vm start foo[0-2,5]

There is also a wildcard (%[1]s) which allows the user to specify all VMs:

		vm start %[1]s

Note that including the wildcard in a list of VMs results in the wildcard
behavior (although a message will be logged).

Calling "vm start" on a specific list of VMs will cause them to be started if
they are in the building, paused, quit, or error states. When used with the
wildcard, only vms in the building or paused state will be started.`, Wildcard),
		Patterns: []string{
			"vm start <target>",
		},
		Call: wrapVMTargetCLI(cliVmStart),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, ^VM_RUNNING)
			}
			return nil
		}),
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop one or more running virtual machines. See "vm start" for a full
description of allowable targets.

Calling stop will put VMs in a paused state. Use "vm start" to restart them.`,
		Patterns: []string{
			"vm stop <target>",
		},
		Call: wrapVMTargetCLI(cliVmStop),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_RUNNING)
			}
			return nil
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
		Call: wrapBroadcastCLI(cliVmFlush),
	},
	{ // vm hotplug
		HelpShort: "add and remove USB drives",
		HelpLong: `
Add and remove USB drives to a launched VM.

To view currently attached media, call vm hotplug with the 'show' argument and
a VM name. To add a device, use the 'add' argument followed by the VM
name, and the name of the file to add. For example, to add foo.img to VM foo:

	vm hotplug add foo foo.img

The add command will assign a disk ID, shown in "vm hotplug". The optional
parameter allows you to specify whether the drive will appear on the 1.1 or 2.0
USB bus. For USB 1.1:

	vm hotplug add foo foo.img 1.1

For USB 2.0:

	vm hotplug add foo foo.img 2.0

To remove media, use the 'remove' argument with the VM name and the disk ID.
For example, to remove the drive added above, named 0:

	vm hotplug remove foo 0

To remove all hotplug devices, use ID "all" for the disk ID.

See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"vm hotplug",
			"vm hotplug <add,> <target> <filename> [version]",
			"vm hotplug <remove,> <target> <disk id or all>",
		},
		Call: wrapVMTargetCLI(cliVMHotplug),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
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

	vm net <vm name> 0 bridgeX 100`,
		Patterns: []string{
			"vm net <connect,> <vm name> <tap position> <bridge> <vlan>",
			"vm net <disconnect,> <vm name> <tap position>",
		},
		Call: wrapSimpleCLI(cliVmNetMod),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else if val == "vlan" {
				return cliVLANSuggest(prefix)
			}
			return nil
		}),
	},
	{ // vm qmp
		HelpShort: "issue a JSON-encoded QMP command",
		HelpLong: `
Issue a JSON-encoded QMP command. This is a convenience function for accessing
the QMP socket of a VM via minimega. vm qmp takes two arguments, a VM name,
and a JSON string, and returns the JSON encoded response. For example:

	vm qmp 0 '{ "execute": "query-status" }'
	{"return":{"running":false,"singlestep":false,"status":"prelaunch"}}`,
		Patterns: []string{
			"vm qmp <vm name> <qmp command>",
		},
		Call: wrapVMTargetCLI(cliVmQmp),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
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

	vm screenshot foo 100

The screenshot can be saved elsewhere like this:

        vm screenshot foo file /tmp/foo.png

You can also specify the maximum dimension:

        vm screenshot foo file /tmp/foo.png 100`,
		Patterns: []string{
			"vm screenshot <vm name> [maximum dimension]",
			"vm screenshot <vm name> file <filename> [maximum dimension]",
		},
		Call: wrapVMTargetCLI(cliVmScreenshot),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
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
			"vm migrate <vm name> <filename>",
		},
		Call: wrapVMTargetCLI(cliVmMigrate),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
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

"vm cdrom change" ejects the current ISO, if there is one.


See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"vm cdrom <eject,> <target>",
			"vm cdrom <change,> <target> <path>",
		},
		Call: wrapVMTargetCLI(cliVMCdrom),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
	},
	{ // vm tag
		HelpShort: "display or set a tag for the specified VM",
		HelpLong: `
Display or set a tag for one or more virtual machines. See "vm start" for a
full description of allowable targets.

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
		Call: wrapVMTargetCLI(cliVmTag),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
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
		Call: wrapVMTargetCLI(cliClearVmTag),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			}
			return nil
		}),
	},
	{ // vm top
		HelpShort: "view vm resource utilization",
		HelpLong: fmt.Sprintf(`
View system resource utilization per VM. This is measured from the host and may
differ from what is reported by the guest.

The optional duration specifies the length of the sampling window in seconds.
The command will block for at least this long while it measures usage. The
default duration is one second.

Returned columns include:
- name      : name of the VM
- namespace : namespace of the VM (when not in a namespace)
- virt      : virtual memory size (MB)
- res       : resident memory size (MB)
- shr       : shared memory size (MB)
- cpu       : host CPU usage (%%)
- vcpu      : guest CPU usage (%%) (KVM only)
- time      : total CPU time
- procs     : number of processes inspected (limited to %d)
- rx        : total received data rate (MB/s)
- tx        : total transmitted data rate (MB/s)`, ProcLimit),
		Patterns: []string{
			"vm top [duration]",
		},
		Call: wrapBroadcastCLI(cliVMTop),
	},
}

func init() {
	// Register these so we can serialize the VMs
	gob.Register(VMs{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

func cliVmStart(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Start(c.StringArgs["target"]))
}

func cliVmStop(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Stop(c.StringArgs["target"]))
}

func cliVmKill(c *minicli.Command, resp *minicli.Response) error {
	return makeErrSlice(vms.Kill(c.StringArgs["target"]))
}

func cliVmInfo(c *minicli.Command, resp *minicli.Response) error {
	fields := vmInfo
	if c.BoolArgs["summary"] {
		fields = vmInfoLite
	}

	vms.Info(fields, resp)
	return nil
}

func cliVMCdrom(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	if c.BoolArgs["eject"] {
		return makeErrSlice(vms.EjectCD(target))
	} else if c.BoolArgs["change"] {
		f := c.StringArgs["path"]
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return err
		}

		return makeErrSlice(vms.ChangeCD(target, f))
	}

	return errors.New("unreachable")
}

func cliVmTag(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	key := c.StringArgs["key"]
	if key == "" {
		// If they didn't specify a key then they probably want all the tags
		// for a given VM
		key = Wildcard
	}

	value, write := c.StringArgs["value"]
	if write {
		if key == Wildcard {
			return errors.New("cannot assign to wildcard")
		}

		vms.SetTag(target, key, value)

		return nil
	}

	if key == Wildcard {
		resp.Header = []string{"id", "tag", "value"}
	} else {
		resp.Header = []string{"id", "value"}
	}

	for _, tag := range vms.GetTags(target, key) {
		row := []string{strconv.Itoa(tag.ID)}

		if key == Wildcard {
			row = append(row, tag.Key)
		}

		row = append(row, tag.Value)
		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliClearVmTag(c *minicli.Command, resp *minicli.Response) error {
	// Get the specified tag name or use Wildcard if not provided
	key, ok := c.StringArgs["key"]
	if !ok {
		key = Wildcard
	}

	// Get the specified VM target or use Wildcard if not provided
	target, ok := c.StringArgs["target"]
	if !ok {
		target = Wildcard
	}

	vms.ClearTags(target, key)

	return nil
}

func cliVmLaunch(c *minicli.Command, resp *minicli.Response) error {
	ns := GetNamespace()

	if ns == nil && len(c.StringArgs) == 0 {
		return errors.New("invalid command when namespace is not active")
	}

	// create a local copy of the current VMConfig
	vmConfig := vmConfig.Copy()

	// namespace behavior
	if ns != nil {
		if len(c.StringArgs) > 0 {
			arg := c.StringArgs["name"]

			vmType, err := findVMType(c.BoolArgs)
			if err != nil {
				return err
			}

			return ns.Queue(arg, vmType, vmConfig)
		}

		return ns.Launch()
	}

	// non-namespace behavior (vm launch happens over meshage in namespaces)

	// expand the names to launch, scheduler should have ensured that they were
	// globally unique.
	names, err := ExpandLaunchNames(c.StringArgs["name"], vms)
	if err != nil {
		return err
	}

	if len(names) > 1 && vmConfig.UUID != "" {
		return errors.New("cannot launch multiple VMs with a pre-configured UUID")
	}

	for i, name := range names {
		if isReserved(name) {
			return fmt.Errorf("`%s` is a reserved word -- cannot use for vm name", name)
		}

		if _, err := strconv.Atoi(name); err == nil {
			return fmt.Errorf("`%s` is an integer -- cannot use for vm name", name)
		}

		// Check for conflicts within the provided names. Don't conflict with
		// ourselves or if the name is unspecified.
		for j, name2 := range names {
			if i != j && name == name2 && name != "" {
				return fmt.Errorf("`%s` is specified twice in VMs to launch", name)
			}
		}
	}

	noblock := c.BoolArgs["noblock"]

	vmType, err := findVMType(c.BoolArgs)
	if err != nil {
		return err
	}

	// default namespace: ""
	errChan := vms.Launch("", &QueuedVMs{names, vmType, vmConfig})

	// Collect all the errors from errChan and turn them into a string
	collectErrs := func() error {
		errs := []error{}
		for err := range errChan {
			errs = append(errs, err)
		}

		return makeErrSlice(errs)
	}

	if noblock {
		go func() {
			if err := collectErrs(); err != nil {
				log.Errorln(err)
			}
		}()

		return nil
	}

	return collectErrs()
}

func cliVmFlush(c *minicli.Command, resp *minicli.Response) error {
	vms.Flush()

	return nil
}

func cliVmQmp(c *minicli.Command, resp *minicli.Response) error {
	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	out, err := vm.QMPRaw(c.StringArgs["qmp"])
	if err != nil {
		return err
	}

	resp.Response = out
	return nil
}

func cliVmScreenshot(c *minicli.Command, resp *minicli.Response) error {
	file := c.StringArgs["filename"]

	var max int

	if arg := c.StringArgs["maximum"]; arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil {
			return err
		}
		max = v
	}

	vm := vms.FindVM(c.StringArgs["vm"])
	if vm == nil {
		return vmNotFound(c.StringArgs["vm"])
	}

	data, err := vm.Screenshot(max)
	if err != nil {
		return err
	}

	path := filepath.Join(*f_base, strconv.Itoa(vm.GetID()), "screenshot.png")
	if file != "" {
		path = file
	}

	// add user data in case this is going across meshage
	err = ioutil.WriteFile(path, data, os.FileMode(0644))
	if err != nil {
		return err
	}

	resp.Data = data

	return nil
}

func cliVmMigrate(c *minicli.Command, resp *minicli.Response) error {
	if _, ok := c.StringArgs["vm"]; !ok { // report current migrations
		resp.Header = []string{"id", "name", "status", "complete (%%)"}

		for _, vm := range vms.FindKvmVMs() {
			status, complete, err := vm.QueryMigrate()
			if err != nil {
				return err
			}
			if status == "" {
				continue
			}

			resp.Tabular = append(resp.Tabular, []string{
				fmt.Sprintf("%v", vm.GetID()),
				vm.GetName(),
				status,
				fmt.Sprintf("%.2f", complete)})
		}

		return nil
	}

	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	return vm.Migrate(c.StringArgs["filename"])
}

func cliVMHotplug(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	if c.BoolArgs["add"] {
		f := c.StringArgs["filename"]
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return err
		}

		version := c.StringArgs["version"]

		return makeErrSlice(vms.Hotplug(target, f, version))
	} else if c.BoolArgs["remove"] {
		disk := c.StringArgs["disk"]

		id, err := strconv.Atoi(disk)
		if err != nil && disk != Wildcard {
			return fmt.Errorf("invalid disk: `%v`", disk)
		}

		return makeErrSlice(vms.HotplugRemove(target, id, disk == Wildcard))
	}

	return makeErrSlice(vms.HotplugInfo(resp))
}

func cliVmNetMod(c *minicli.Command, resp *minicli.Response) error {
	vm := vms.FindVM(c.StringArgs["vm"])
	if vm == nil {
		return vmNotFound(c.StringArgs["vm"])
	}

	pos, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		return err
	}

	if c.BoolArgs["disconnect"] {
		return vm.NetworkDisconnect(pos)
	}

	vlan, err := lookupVLAN(c.StringArgs["vlan"])
	if err != nil {
		return err
	}

	return vm.NetworkConnect(pos, c.StringArgs["bridge"], vlan)
}

func cliVMTop(c *minicli.Command, resp *minicli.Response) error {
	d := time.Second
	if c.StringArgs["duration"] != "" {
		v, err := strconv.Atoi(c.StringArgs["duration"])
		if err != nil {
			return err
		}

		d = time.Duration(v) * time.Second
	}

	ns := GetNamespace()

	resp.Header = []string{"name"}
	if ns == nil {
		resp.Header = append(resp.Header, "namespace")
	}
	resp.Header = append(resp.Header,
		"virt",
		"res",
		"shr",
		"cpu",
		"vcpu",
		"time",
		"procs",
		"rx",
		"tx",
	)

	fmtMB := func(i uint64) string {
		return strconv.FormatUint(i/(uint64(1)<<20), 10)
	}

	for _, s := range vms.ProcStats(d) {
		row := []string{s.Name}
		if ns == nil {
			row = append(row, s.Namespace)
		}

		row = append(row,
			fmtMB(s.Size()),
			fmtMB(s.Resident()),
			fmtMB(s.Share()),
			fmt.Sprintf("%.2f", s.CPU()*100),
			fmt.Sprintf("%.2f", s.GuestCPU()*100),
			s.Time().String(),
			strconv.Itoa(s.Count()),
			fmt.Sprintf("%.2f", s.RxRate),
			fmt.Sprintf("%.2f", s.TxRate),
		)

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

// cliVMSuggest takes a prefix that could be the start of a VM name
// and makes suggestions for VM names that have a common prefix. mask
// can be used to only complete for VMs that are in a particular state (e.g.
// running). Returns a list of suggestions.
func cliVMSuggest(prefix string, mask VMState) []string {
	res := []string{}

	if strings.HasPrefix(Wildcard, prefix) {
		res = append(res, Wildcard)
	}

	for _, vm := range GlobalVMs() {
		if vm.GetState()&mask == 0 {
			continue
		}

		if strings.HasPrefix(vm.GetName(), prefix) {
			res = append(res, vm.GetName())
		}
	}

	return res
}
