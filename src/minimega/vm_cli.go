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
	"ranges"
	"strconv"
	"strings"
	"sync"
)

var vmCLIHandlers = []minicli.Handler{
	{ // vm info
		HelpShort: "print information about VMs",
		HelpLong: `
Print information about VMs in tabular form. The .filter and .columns commands
can be used to subselect a set of rows and/or columns. See the help pages for
.filter and .columns, respectively, for their usage. Columns returned by VM
info include:

- host       : the host that the VM is running on
- id         : the VM ID, as an integer
- name       : the VM name, if it exists
- state      : one of (building, running, paused, quit, error)
- type       : one of (kvm)
- vcpus      : the number of allocated CPUs
- memory     : allocated memory, in megabytes
- vlan       : vlan, as an integer
- bridge     : bridge name
- tap        : tap name
- mac        : mac address
- ip         : IPv4 address
- ip6        : IPv6 address
- bandwidth  : stats regarding bandwidth usage
- tags       : any additional information attached to the VM
- uuid       : QEMU system uuid
- cc_active  : whether cc is active

Additional fields are available for KVM-based VMs:

- append     : kernel command line string
- cdrom      : cdrom image
- disk       : disk image
- kernel     : kernel image
- initrd     : initrd image
- migrate    : qemu migration image

Additional fields are available for container-based VMs:

- init	     : process to invoke as init
- preinit    : process to invoke at container launch before isolation
- filesystem : root filesystem for the container

Examples:

Display a list of all IPs for all VMs:
	.columns ip,ip6 vm info

Display information about all VMs:
	vm info`,
		Patterns: []string{
			"vm info",
		},
		Call: wrapBroadcastCLI(cliVmInfo),
	},
	{ // vm save
		HelpShort: "save a vm configuration for later use",
		HelpLong: `
Saves the configuration of a running virtual machine or set of virtual machines
so that it/they can be restarted/recovered later, such as after a system crash.

This command does not store the state of the virtual machine itself, only its
launch configuration.

See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"vm save <name> <target>",
		},
		Call: wrapVMTargetCLI(cliVmSave),
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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

If you supply a name instead of a number of VMs, one VM with that name will be
launched. You may also supply a range expression to launch VMs with a specific
naming scheme:

	vm launch foo[0-9]

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
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
	},
	{ // vm start
		HelpShort: "start paused virtual machines",
		HelpLong: fmt.Sprintf(`
Start one or more paused virtual machines. VMs may be selected by name, ID, range, or
wildcard. For example,

To start vm foo:

		vm start foo

To start vms foo and bar:

		vm start foo,bar

To start vms foo0, foo1, foo2, and foo5:

		vm start foo[0-2,5]

VMs can also be specified by ID, such as:

		vm start 0

Or, a range of IDs:

		vm start [2-4,6]

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
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, ^VM_RUNNING)
			} else {
				return nil
			}
		},
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
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_RUNNING)
			} else {
				return nil
			}
		},
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
		Call: wrapVMTargetCLI(cliVmHotplug),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else if val == "vlan" {
				return suggestVLAN(prefix)
			} else {
				return nil
			}
		},
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
		Call: wrapVMTargetCLI(cliVmQmp),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
			"vm screenshot <vm id or name> [maximum dimension]",
			"vm screenshot <vm id or name> file <filename> [maximum dimension]",
		},
		Call: wrapVMTargetCLI(cliVmScreenshot),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
		Call: wrapVMTargetCLI(cliVmMigrate),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
		Suggest: func(val, prefix string) []string {
			if val == "target" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
		Call: wrapVMTargetCLI(cliVmCdrom),
		Suggest: func(val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(prefix, VM_ANY_STATE)
			} else {
				return nil
			}
		},
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
}

func init() {
	// Register these so we can serialize the VMs
	gob.Register(VMs{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

func cliVmStart(c *minicli.Command) *minicli.Response {
	errs := LocalVMs().start(c.StringArgs["target"])

	return &minicli.Response{
		Host:  hostname,
		Error: errSlice(errs).String(),
	}
}

func cliVmStop(c *minicli.Command) *minicli.Response {
	errs := LocalVMs().stop(c.StringArgs["target"])

	return &minicli.Response{
		Host:  hostname,
		Error: errSlice(errs).String(),
	}
}

func cliVmKill(c *minicli.Command) *minicli.Response {
	errs := LocalVMs().kill(c.StringArgs["target"])

	return &minicli.Response{
		Host:  hostname,
		Error: errSlice(errs).String(),
	}
}

func cliVmInfo(c *minicli.Command) *minicli.Response {
	var err error
	resp := &minicli.Response{Host: hostname}

	// Create locally scoped copy of vms in current namespace
	vms := LocalVMs()

	// Populate "dynamic" fields for all VMs, when running outside of the
	// namespace environment.
	for _, vm := range vms {
		vm.UpdateBW()
		vm.UpdateCCActive()
	}

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
	doVms := make([]*KvmVM, 0)
	if vmstring == Wildcard {
		for _, vm := range LocalVMs() {
			switch vm := vm.(type) {
			case *KvmVM:
				doVms = append(doVms, vm)
			default:
				// TODO: Do anything?
			}
		}
	} else {
		vm := LocalVMs().findVm(vmstring)
		if vm == nil {
			resp.Error = vmNotFound(vmstring).Error()
			return resp
		}
		if vm, ok := vm.(*KvmVM); ok {
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

	target := c.StringArgs["target"]

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

	// For each VM, get or set tags based on key/value/setOp. Should not be run
	// in parallel since it updates resp.Tabular.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		if setOp {
			vm.GetTags()[key] = value
		} else if key == Wildcard {
			for k, v := range vm.GetTags() {
				resp.Tabular = append(resp.Tabular, []string{
					strconv.Itoa(vm.GetID()),
					k, v,
				})
			}
		} else {
			// TODO: return false if tag not set?
			resp.Tabular = append(resp.Tabular, []string{
				strconv.Itoa(vm.GetID()),
				vm.GetTags()[key],
			})
		}

		return true, nil
	}

	errs := LocalVMs().apply(target, false, applyFunc)
	resp.Error = errSlice(errs).String()

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

	// For each VM, clear the appropriate tag. Can be run in parallel.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		if key == Wildcard {
			vm.ClearTags()
		} else {
			delete(vm.GetTags(), key)
		}

		return true, nil
	}

	errs := LocalVMs().apply(target, true, applyFunc)
	resp.Error = errSlice(errs).String()

	return resp
}

func cliVmLaunch(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if namespace == "" && len(c.StringArgs) == 0 {
		resp.Error = "invalid command when namespace is not active"
		return resp
	}

	if namespace != "" && isUserSource(c.Source) {
		if len(c.StringArgs) > 0 {
			namespaceQueue(c, resp)
		} else {
			namespaceLaunch(c, resp)
		}

		return resp
	}

	// Only need to check collisions with VMs running locally, scheduler
	// *should* have done checks to make sure that the VMs it was launching we
	// globally unique.
	names, err := expandVMLaunchNames(c.StringArgs["name"], LocalVMs())

	if len(names) > 1 && vmConfig.UUID != "" {
		err = errors.New("cannot launch multiple VMs with a pre-configured UUID")
	}

	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	for i, name := range names {
		if isReserved(name) {
			resp.Error = fmt.Sprintf("`%s` is a reserved word -- cannot use for vm name", name)
			return resp
		}

		if _, err := strconv.Atoi(name); err == nil {
			resp.Error = fmt.Sprintf("`%s` is an integer -- cannot use for vm name", name)
			return resp
		}

		// Check for conflicts within the provided names. Don't conflict with
		// ourselves or if the name is unspecified.
		for j, name2 := range names {
			if i != j && name == name2 && name != "" {
				resp.Error = fmt.Sprintf("`%s` is specified twice in VMs to launch", name)
				return resp
			}
		}
	}

	noblock := c.BoolArgs["noblock"]

	vmType, err := findVMType(c.BoolArgs)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	log.Info("launching %v %v vms", len(names), vmType)

	errChan := make(chan error)

	var wg sync.WaitGroup

	for _, name := range names {
		wg.Add(1)

		var vm VM
		switch vmType {
		case KVM:
			vm = NewKVM(name)
		case CONTAINER:
			vm = NewContainer(name)
		default:
			// TODO
		}

		go func(name string) {
			defer wg.Done()

			errChan <- vms.launch(vm)
		}(name)
	}

	go func() {
		defer close(errChan)

		wg.Wait()
	}()

	// Collect all the errors from errChan and turn them into a string
	collectErrs := func() string {
		errs := []error{}
		for err := range errChan {
			errs = append(errs, err)
		}
		return errSlice(errs).String()
	}

	if noblock {
		go func() {
			if err := collectErrs(); err != "" {
				log.Errorln(err)
			}
		}()
	} else {
		resp.Error = collectErrs()
	}

	return resp
}

func cliVmFlush(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// See VMs.flush for why we don't use LocalVMs
	vms.flush()

	return resp
}

func cliVmQmp(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var err error
	resp.Response, err = LocalVMs().qmp(c.StringArgs["vm"], c.StringArgs["qmp"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliVmScreenshot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := c.StringArgs["vm"]
	maximum := c.StringArgs["maximum"]
	file := c.StringArgs["filename"]

	var max int
	var err error
	if maximum != "" {
		max, err = strconv.Atoi(maximum)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	}

	pngData, err := LocalVMs().screenshot(vm, max)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	// VM has to exist if we got pngData without an error
	id := LocalVMs().findVm(vm).GetID()

	path := filepath.Join(*f_base, strconv.Itoa(id), "screenshot.png")
	if file != "" {
		path = file
	}

	// add user data in case this is going across meshage
	err = ioutil.WriteFile(path, pngData, os.FileMode(0644))
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	resp.Data = pngData

	return resp
}

func cliVmMigrate(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var err error

	if _, ok := c.StringArgs["vm"]; !ok { // report current migrations
		// tabular data is
		// 	vm id, vm name, migrate status, % complete
		for _, vm := range LocalVMs() {
			vm, ok := vm.(*KvmVM)
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
				fmt.Sprintf("%v", vm.GetID()),
				vm.GetName(),
				status,
				fmt.Sprintf("%.2f", complete)})
		}
		if len(resp.Tabular) != 0 {
			resp.Header = []string{"vm id", "vm name", "status", "%% complete"}
		}
		return resp
	}

	err = LocalVMs().migrate(c.StringArgs["vm"], c.StringArgs["filename"])
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
	defer file.Close()

	if err := LocalVMs().save(file, c.StringArgs["target"]); err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliVmHotplug(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	vm := LocalVMs().findVm(c.StringArgs["vm"])
	if vm == nil {
		resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		return resp
	}
	kvm, ok := vm.(*KvmVM)
	if !ok {
		resp.Error = vmNotKVM(c.StringArgs["vm"]).Error()
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

	vm := LocalVMs().findVm(c.StringArgs["vm"])
	if vm == nil {
		resp.Error = vmNotFound(c.StringArgs["vm"]).Error()
		return resp
	}

	pos, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if c.BoolArgs["disconnect"] {
		err = vm.NetworkDisconnect(pos)
	} else {
		vlan := 0

		arg := c.StringArgs["vlan"]
		vlan, err = allocatedVLANs.ParseVLAN(namespace, arg, true)
		if err == nil {
			err = vm.NetworkConnect(pos, c.StringArgs["bridge"], vlan)
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

// cliVMSuggest takes a prefix that could be the start of a VM name or a VM ID
// and makes suggestions for VM names (or IDs) that have a common prefix. mask
// can be used to only complete for VMs that are in a particular state (e.g.
// running). Returns a list of suggestions.
func cliVMSuggest(prefix string, mask VMState) []string {
	var isID bool
	res := []string{}

	if _, err := strconv.Atoi(prefix); err == nil {
		isID = true
	}

	for _, vm := range GlobalVMs() {
		if vm.GetState()&mask == 0 {
			continue
		}

		if isID {
			id := strconv.Itoa(vm.GetID())

			if strings.HasPrefix(id, prefix) {
				res = append(res, id)
			}
		} else if strings.HasPrefix(vm.GetName(), prefix) {
			res = append(res, vm.GetName())
		}
	}

	return res
}

// expandVMLaunchNames takes a VM name, range, or count and expands the list of
// names of VMs that should be launch. Does several sanity checks on the names
// to make sure that they aren't reserved words and don't collide with existing
// VM names (as supplied via the vms argument).
func expandVMLaunchNames(arg string, vms VMs) ([]string, error) {
	names := []string{}

	count, err := strconv.ParseInt(arg, 10, 32)
	if err != nil {
		names, err = ranges.SplitList(arg)
	} else if count <= 0 {
		err = errors.New("invalid number of vms (must be > 0)")
	} else {
		names = make([]string, count)
	}

	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		return nil, errors.New("no VMs to launch")
	}

	for _, name := range names {
		if isReserved(name) {
			return nil, fmt.Errorf("invalid vm name, `%s` is a reserved word", name)
		}

		if _, err := strconv.Atoi(name); err == nil {
			return nil, fmt.Errorf("invalid vm name, `%s` is an integer", name)
		}

		for _, vm := range vms {
			if vm.GetName() == name {
				return nil, fmt.Errorf("vm already exists with name `%s`", name)
			}
		}
	}

	return names, nil
}
