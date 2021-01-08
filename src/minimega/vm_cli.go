// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
		Call: wrapBroadcastCLI(cliVMInfo),
	},
	{ // vm launch
		HelpShort: "launch virtual machines in a paused state",
		HelpLong: `
Launch virtual machines in a paused state, using the parameters defined leading
up to the launch command. Any changes to the VM parameters after launching will
have no effect on launched VMs.

When you launch a VM, you supply the type of VM in the launch command. The
supported VM types are:

- kvm : QEMU-based vms
- container: Linux containers

If you supply a name instead of a number of VMs, one VM with that name will be
launched. You may also supply a range expression to launch VMs with a specific
naming scheme:

	vm launch kvm foo[0-9]

Note: VM names cannot be integers or reserved words (e.g. "all").

Users may specify a saved config explicity rather than use the current one, for
example:

	vm config save endpoint
	[other commands]
	vm launch kvm 5 endpoint

If queueing is enabled (see "ns"), VMs will be queued for launching until "vm
launch" is called with no additional arguments. This allows the scheduler to
better allocate resources across the cluster.`,
		Patterns: []string{
			"vm launch",
			"vm launch <kvm,> <name or count> [config]",
			"vm launch <container,> <name or count> [config]",
		},
		Call: wrapSimpleCLI(cliVMLaunch),
	},
	{ // vm kill
		HelpShort: "kill running virtual machines",
		HelpLong: `
Kill one or more running virtual machines. See "vm start" for a full
description of allowable targets.`,
		Patterns: []string{
			"vm <kill,> <vm target>",
		},
		Call:    wrapVMTargetCLI(cliVMApply),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
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
			"vm <start,> <vm target>",
		},
		Call:    wrapVMTargetCLI(cliVMApply),
		Suggest: wrapVMSuggest(^VM_RUNNING, true),
	},
	{ // vm stop
		HelpShort: "stop/pause virtual machines",
		HelpLong: `
Stop one or more running virtual machines. See "vm start" for a full
description of allowable targets.

Calling stop will put VMs in a paused state. Use "vm start" to restart them.`,
		Patterns: []string{
			"vm <stop,> <vm target>",
		},
		Call:    wrapVMTargetCLI(cliVMApply),
		Suggest: wrapVMSuggest(VM_RUNNING, true),
	},
	{ // vm flush
		HelpShort: "discard information about quit or failed VMs",
		HelpLong: `
Flush one or more virtual machines. Discard information about VMs that
have either quit or encountered an error. This will remove VMs with a state of
"quit" or "error" from vm info. Names of VMs that have been flushed may be
reused.

Note running without arguments results in the same behavior as using the "all"
target. See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"vm <flush,>",
			"vm <flush,> <vm target>",
		},
		Call:    wrapBroadcastCLI(cliVMApply),
		Suggest: wrapVMSuggest((VM_QUIT | VM_ERROR), true),
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
			"vm hotplug <add,> <vm target> <filename> [version]",
			"vm hotplug <add,> <vm target> <filename> serial <serial> [version]",
			"vm hotplug <remove,> <vm target> <disk id or all>",
		},
		Call:    wrapVMTargetCLI(cliVMHotplug),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
	},
	{ // vm net
		HelpShort: "add, disconnect, or move network connections",
		HelpLong: `
Add, disconnect, or move existing network connections for one or more VMs. See "vm
start" for a full description of allowable targets.

To add a network connection, you can specify the same options as you do when you add
connections via vm config when launching VMs. See "vm config net" for more details.

You will need to specify the VLAN of which the interface is a member. Optionally, you may
specify the brige the interface will be connected on. You may also specify a MAC address for
the interface. Finally, you may also specify the network device for qemu to use. By default, 
"e1000" is used. The order is:

	<bridge>,<VLAN>,<MAC>,<driver>

So to add an interface to a vm called vm-0 that is a member of VLAN 100, with a specified MAC
address, you can use:

	vm net add vm-0 100,00:00:00:00:00:00

Network connections are indicated by their position in vm net (same order in vm
info) and are zero indexed. For example, to disconnect the first network
connection from a VM named vm-0:

	vm net disconnect vm-0 0

To disconnect the second interface:

	vm net disconnect vm-0 1

To move a connection, specify the interface number, the new VLAN tag and
optional bridge:

	vm net vm-0 0 100 mega_bridge

If the bridge name is omitted, the interface will be reconnected to the same
bridge that it is already on. If the interface is not connected to a bridge, it
will be connected to the default bridge, "mega_bridge".`,
		Patterns: []string{
			"vm net <add,> <vm target> [netspec]...",
			"vm net <connect,> <vm target> <tap position> <vlan> [bridge]",
			"vm net <disconnect,> <vm target> <tap position>",
		},
		Call: wrapVMTargetCLI(cliVMNetMod),
		Suggest: wrapSuggest(func(ns *Namespace, val, prefix string) []string {
			if val == "vm" {
				return cliVMSuggest(ns, prefix, VM_ANY_STATE, false)
			} else if val == "vlan" {
				return cliVLANSuggest(ns, prefix)
			} else if val == "bridge" {
				return cliBridgeSuggest(ns, prefix)
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
		Call:    wrapVMTargetCLI(cliVMQmp),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
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
		Call:    wrapVMTargetCLI(cliVMScreenshot),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
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
		Call:    wrapVMTargetCLI(cliVMMigrate),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
	},
	{ // vm cdrom
		HelpShort: "eject or change an active VM's cdrom",
		HelpLong: `
Eject or change an active VM's cdrom image.

Eject VM 0's cdrom:

        vm cdrom eject 0

Eject all VM cdroms:

        vm cdrom eject all

If the cdrom is "locked" by the guest, the force option can be used to override
the lock:

        vm cdrom eject 0 force

Change a VM to use a new ISO:

        vm cdrom change 0 /tmp/debian.iso

"vm cdrom change" ejects the current ISO, if there is one.

See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"vm cdrom <eject,> <vm target> [force,]",
			"vm cdrom <change,> <vm target> <path> [force,]",
		},
		Call:    wrapVMTargetCLI(cliVMCdrom),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
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

        vm tag <vm target> <key or all>`,
		Patterns: []string{
			"vm tag <vm target> [key or all]",  // get
			"vm tag <vm target> <key> <value>", // set
		},
		Call:    wrapVMTargetCLI(cliVMTag),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
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
			"clear vm tag <vm target> [tag]",
		},
		Call:    wrapVMTargetCLI(cliClearVMTag),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
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

func cliVMApply(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["start"]:
		return ns.Start(c.StringArgs["vm"])
	case c.BoolArgs["stop"]:
		return ns.VMs.Stop(c.StringArgs["vm"])
	case c.BoolArgs["kill"]:
		return ns.VMs.Kill(c.StringArgs["vm"])
	case c.BoolArgs["flush"]:
		if len(c.StringArgs["vm"]) == 0 {
			return ns.VMs.FlushAll(ns.ccServer)
		} else {
			return ns.VMs.Flush(c.StringArgs["vm"], ns.ccServer)
		}
	}

	return unreachable()
}

func cliVMInfo(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	fields := vmInfo
	if c.BoolArgs["summary"] {
		fields = vmInfoLite
	}

	ns.VMs.Info(fields, resp)
	return nil
}

func cliVMCdrom(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	force := c.BoolArgs["force"]
	target := c.StringArgs["vm"]

	if c.BoolArgs["eject"] {
		return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
			kvm, ok := vm.(*KvmVM)
			if !ok {
				return false, nil
			}

			err := kvm.EjectCD(force)
			if wild && err != nil && err.Error() == "no cdrom inserted" {
				// suppress error if more than one target
				err = nil
			}

			return true, nil
		})
	} else if c.BoolArgs["change"] {
		f := c.StringArgs["path"]
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return err
		}

		return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
			kvm, ok := vm.(*KvmVM)
			if !ok {
				return false, nil
			}

			return true, kvm.ChangeCD(f, force)
		})
	}

	return unreachable()
}

func cliVMTag(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["vm"]

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

		return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
			vm.SetTag(key, value)

			return true, nil
		})
	}

	if key == Wildcard {
		resp.Header = []string{"name", "tag", "value"}
	} else {
		resp.Header = []string{"name", "value"}
	}

	// synchronizes appends to resp.Tabular
	var mu sync.Mutex

	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		mu.Lock()
		defer mu.Unlock()

		name := vm.GetName()

		if key == Wildcard {
			for k, v := range vm.GetTags() {
				resp.Tabular = append(resp.Tabular, []string{
					name, k, v,
				})
			}

			return true, nil
		}

		// TODO: return false if tag not set?
		resp.Tabular = append(resp.Tabular, []string{
			name, vm.Tag(key),
		})

		return true, nil
	})
}

func cliClearVMTag(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Get the specified tag name or use Wildcard if not provided
	key, ok := c.StringArgs["key"]
	if !ok {
		key = Wildcard
	}

	// Get the specified VM target or use Wildcard if not provided
	target, ok := c.StringArgs["vm"]
	if !ok {
		target = Wildcard
	}

	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		vm.ClearTag(key)

		return true, nil
	})
}

func cliVMLaunch(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// HAX: prevent running as a subcommand
	if c.Source == SourceMeshage {
		return fmt.Errorf("cannot run `%s` via meshage", c.Original)
	}

	// adding VM to queue
	if len(c.StringArgs) > 0 {
		// create a local copy of the current or specified VMConfig
		var vmConfig VMConfig

		if name := c.StringArgs["config"]; name != "" {
			if _, ok := ns.savedVMConfig[name]; !ok {
				return fmt.Errorf("config %v does not exist", name)
			}
			vmConfig = ns.savedVMConfig[name].Copy()
		} else {
			vmConfig = ns.vmConfig.Copy()
		}

		vmType, err := findVMType(c.BoolArgs)
		if err != nil {
			return err
		}

		err = ns.Queue(c.StringArgs["name"], vmType, vmConfig)

		if err == nil && !ns.QueueVMs {
			// no error queueing and user has disabled queueing -- launch now!
			return ns.Schedule(false)
		}

		return err
	}

	return ns.Schedule(false)
}

func cliVMQmp(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	vm, err := ns.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(c.StringArgs["qmp"]), &m); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	out, err := vm.QMPRaw(c.StringArgs["qmp"])
	if err != nil {
		return err
	}

	resp.Response = out
	return nil
}

func cliVMScreenshot(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	file := c.StringArgs["filename"]

	var max int

	if arg := c.StringArgs["maximum"]; arg != "" {
		v, err := strconv.Atoi(arg)
		if err != nil {
			return err
		}
		max = v
	}

	vm := ns.FindVM(c.StringArgs["vm"])
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

func cliVMMigrate(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if _, ok := c.StringArgs["vm"]; !ok { // report current migrations
		resp.Header = []string{"id", "name", "status", "complete (%)"}

		for _, vm := range ns.FindKvmVMs() {
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

	vm, err := ns.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	fname := c.StringArgs["filename"]

	if !filepath.IsAbs(fname) {
		// TODO: should we write to the VM directory instead?
		fname = filepath.Join(*f_iomBase, fname)
	}

	if _, err := os.Stat(filepath.Dir(fname)); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(fname), 0755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return vm.Migrate(fname)
}

func cliVMHotplug(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["vm"]

	if c.BoolArgs["add"] {
		f := c.StringArgs["filename"]
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return err
		}

		version := c.StringArgs["version"]
		serial := c.StringArgs["serial"]

		return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
			if kvm, ok := vm.(*KvmVM); ok {
				return true, kvm.Hotplug(f, version, serial)
			}

			return false, nil
		})
	} else if c.BoolArgs["remove"] {
		disk := c.StringArgs["disk"]

		id, err := strconv.Atoi(disk)
		if err != nil && disk != Wildcard {
			return fmt.Errorf("invalid disk: `%v`", disk)
		}

		return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
			kvm, ok := vm.(*KvmVM)
			if !ok {
				return false, nil
			}

			if disk == Wildcard {
				err := kvm.HotplugRemoveAll()
				if wild && err != nil && err.Error() == "no hotplug devices to remove" {
					// suppress error if more than one target
					err = nil
				}
				return true, err
			}

			err := kvm.HotplugRemove(id)
			if wild && err != nil && err.Error() == "no such hotplug device" {
				// suppress error if more than one target
				err = nil
			}

			return true, err
		})
	}

	resp.Header = []string{"name", "id", "file", "version"}

	// synchronizes appends to resp.Tabular
	var mu sync.Mutex

	return ns.VMs.Apply(Wildcard, func(vm VM, wild bool) (bool, error) {
		kvm, ok := vm.(*KvmVM)
		if !ok {
			return false, nil
		}

		name := vm.GetName()
		res := kvm.HotplugInfo()

		mu.Lock()
		defer mu.Unlock()

		for k, v := range res {
			resp.Tabular = append(resp.Tabular, []string{
				name, strconv.Itoa(k), v.Disk, v.Version,
			})
		}

		return true, nil
	})
}

func cliVMNetMod(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["vm"]
	var pos int
	var err error
	if !c.BoolArgs["add"] {
		pos, err = strconv.Atoi(c.StringArgs["tap"])
		if err != nil {
			return err
		}
	}

	var vlan int
	if !c.BoolArgs["disconnect"] && !c.BoolArgs["add"] {
		vlan, err = lookupVLAN(ns.Name, c.StringArgs["vlan"])
		if err != nil {
			return err
		}
	}

	bridge := c.StringArgs["bridge"]

	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		var err error

		log.Info("vm networks: %v", vm.GetNetworks())

		if c.BoolArgs["add"] {
			// This will do the work of adding the interface to the vm
			nics, err := ns.parseVMNets(c.ListArgs["netspec"])
			if err != nil {
				return true, err
			}
			kvm, ok := vm.(*KvmVM)
			if !ok {
				return true, fmt.Errorf("Unable to get Kvm")
			}
			for _, n := range nics {
				err = kvm.AddNIC(n)
			}
		} else if c.BoolArgs["disconnect"] {
			err = vm.NetworkDisconnect(pos)
		} else {
			err = vm.NetworkConnect(pos, vlan, bridge)
		}

		if err != nil {
			return true, err
		}

		log.Info("vm networks: %v", vm.GetNetworks())

		if err := writeVMConfig(vm); err != nil {
			// don't propagate this error
			log.Warn("unable to update vm config for %v: %v", vm.GetID(), err)
		}

		return true, nil
	})
}

func cliVMTop(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	d := time.Second
	if c.StringArgs["duration"] != "" {
		v, err := strconv.Atoi(c.StringArgs["duration"])
		if err != nil {
			return err
		}

		d = time.Duration(v) * time.Second
	}

	resp.Header = []string{
		"name",
		"virt",
		"res",
		"shr",
		"cpu",
		"vcpu",
		"time",
		"procs",
		"rx",
		"tx",
	}

	fmtMB := func(i uint64) string {
		return strconv.FormatUint(i/(uint64(1)<<20), 10)
	}

	for _, s := range ns.ProcStats(d) {
		row := []string{
			s.Name,
			fmtMB(s.Size()),
			fmtMB(s.Resident()),
			fmtMB(s.Share()),
			fmt.Sprintf("%.2f", s.CPU()*100),
			fmt.Sprintf("%.2f", s.GuestCPU()*100),
			s.Time().String(),
			strconv.Itoa(s.Count()),
			fmt.Sprintf("%.2f", s.RxRate),
			fmt.Sprintf("%.2f", s.TxRate),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

// cliVMSuggest takes a prefix that could be the start of a VM name
// and makes suggestions for VM names that have a common prefix. mask
// can be used to only complete for VMs that are in a particular state (e.g.
// running). Returns a list of suggestions.
func cliVMSuggest(ns *Namespace, prefix string, mask VMState, wild bool) []string {
	res := []string{}

	if strings.HasPrefix(Wildcard, prefix) && wild {
		res = append(res, Wildcard)
	}

	for _, vm := range GlobalVMs(ns) {
		if vm.GetState()&mask == 0 {
			continue
		}

		if strings.HasPrefix(vm.GetName(), prefix) {
			res = append(res, vm.GetName())
		}
	}

	return res
}
