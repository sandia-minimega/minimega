// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// command line interface for minimega
// 
// The command line interface wraps a number of commands listed in the 
// cliCommands map. Each entry to the map defines a function that is called
// when the command is invoked on the command line, as well as short and long
// form help. The record parameter instructs the cli to put the command in the
// command history.
//
// The cli uses the readline library for command history and tab completion.
// A separate command history is kept and used for writing the buffer out to
// disk. 
package main

// TODO: vm_info command to list current info
// TODO: bridge_info or something like it 

import (
	"bufio"
	"bytes"
	"fmt"
	"goreadline"
	"io"
	log "minilog"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	commandBuf []string // command history for the write command

	// incoming commands for the cli to parse. these can come from the cli
	// proper (readline), or from a network source, etc. the cli will parse
	// them all as if they were typed locally.
	commandChanLocal   chan cliCommand
	commandChanSocket  chan cliCommand
	commandChanMeshage chan cliCommand

	ackChanLocal   chan cliResponse // acknowledgements from the cli, one per incoming command
	ackChanSocket  chan cliResponse
	ackChanMeshage chan cliResponse

	cliCommands map[string]*command
)

type cliCommand struct {
	Command  string
	Args     []string
	ackChan chan cliResponse
	TID      int32
}

type cliResponse struct {
	Response string
	Error    string // because you can't gob/json encode an error type
	More     bool   // more is set if the called command will be sending multiple responses
	TID      int32
}

type command struct {
	Call      func(c cliCommand) cliResponse // callback function
	Helpshort string                           // short form help test, one line only
	Helplong  string                           // long form help text
	Record    bool                             // record in the command history
	Clear     func() error                     // clear/restore to default state
}

func init() {
	commandChanLocal = make(chan cliCommand)
	commandChanSocket = make(chan cliCommand)
	commandChanMeshage = make(chan cliCommand)
	ackChanLocal = make(chan cliResponse)
	ackChanSocket = make(chan cliResponse)
	ackChanMeshage = make(chan cliResponse)

	// list of commands the cli supports. some commands have small callbacks, which
	// are defined inline.
	cliCommands = map[string]*command{
		"rate": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: fmt.Sprintf("%v", launchRate),
					}
				} else if len(c.Args) != 1 {
					return cliResponse{
						Error: "rate takes one argument",
					}
				} else {
					r, err := strconv.Atoi(c.Args[0])
					if err != nil {
						return cliResponse{
							Error: err.Error(),
						}
					}
					launchRate = time.Millisecond * time.Duration(r)
				}
				return cliResponse{}
			},
			Helpshort: "set the launch/kill rate in milliseconds",
			Helplong: `
Set the launch and kill rate in milliseconds. Some calls to external tools can
take some time to respond, causing errors if you try to launch or kill VMs too
quickly. The default value is 100 milliseconds.`,
			Record: true,
			Clear: func() error {
				launchRate = time.Millisecond * 100
				return nil
			},
		},

		"log_level": &command{
			Call:      cliLogLevel,
			Helpshort: "set the log level",
			Helplong: `
Usage: log_level <level>
Set the log level to one of [debug, info, warn, error, fatal, off]. Log levels
inherit lower levels, so setting the level to error will also log fatal, and
setting the mode to debug will log everything.`,
			Record: true,
			Clear: func() error {
				*f_loglevel = "error"
				log.SetLevel("stdio", log.ERROR)
				log.SetLevel("file", log.ERROR)
				return nil
			},
		},

		"log_stderr": &command{
			Call:      cliLogStderr,
			Helpshort: "enable/disable logging to stderr",
			Helplong: `
Enable or disable logging to stderr. Valid options are [true, false].`,
			Record: true,
			Clear: func() error {
				_, err := log.GetLevel("stdio")
				if err == nil {
					log.DelLogger("stdio")
				}
				return nil
			},
		},

		"log_file": &command{
			Call:      cliLogFile,
			Helpshort: "enable logging to a file",
			Helplong: `
Usage log_file <filename>
Log to a file. To disable file logging, call "log_file false".`,
			Record: true,
			Clear: func() error {
				_, err := log.GetLevel("file")
				if err == nil {
					log.DelLogger("file")
				}
				return nil
			},
		},

		"check": &command{
			Call:      externalCheck,
			Helpshort: "check for the presence of all external executables minimega uses",
			Helplong: `
Minimega maintains a list of external packages that it depends on, such as qemu.
Calling check will attempt to find each of these executables in the avaiable
path, and returns an error on the first one not found.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"nuke": &command{
			Call:      nuke,
			Helpshort: "attempt to clean up after a crash",
			Helplong: `
After a crash, the VM state on the machine can be difficult to recover from.
Nuke attempts to kill all instances of QEMU, remove all taps and bridges, and
removes the temporary minimega state on the harddisk.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"write": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 1 {
					return cliResponse{
						Error: "write takes a single argument",
					}
				}
				file, err := os.Create(c.Args[0])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				for _, i := range commandBuf {
					_, err = file.WriteString(i + "\n")
					if err != nil {
						return cliResponse{
							Error: err.Error(),
						}
					}
				}
				return cliResponse{}
			},
			Helpshort: "write the command history to a file",
			Helplong: `
Usage: write <file>
Write the command history to <file>. This is useful for handcrafting configs
on the minimega command line and then saving them for later use. Argss that
failed, as well as some commands that do not impact the VM state, such as
'help', do not get recorded.`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"read": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 1 {
					return cliResponse{
						Error: "read takes a single argument",
					}
				}
				file, err := os.Open(c.Args[0])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				r := bufio.NewReader(file)
				for {
					l, _, err := r.ReadLine()
					if err != nil {
						if err == io.EOF {
							break
						}
						log.Error("%v", err)
					}
					log.Info("read command: %v", string(l))
					f := strings.Fields(string(l))
					var command string
					var args []string
					if len(f) > 0 {
						command = f[0]
					}
					if len(f) > 1 {
						args = f[1:]
					}
					resp := cliExec(cliCommand{
						Command: command,
						Args:    args,
					})
					resp.More = true
					c.ackChan <- resp
					if resp.Error != "" {
						log.Errorln(resp.Error)
						break // stop on errors
					}
				}
				return cliResponse{}
			},
			Helpshort: "read and execute a command file",
			Helplong: `
Usage: read <file>
Read a command file and execute it. This has the same behavior as if you typed
the file in manually.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_status": &command{
			Call: func(c cliCommand) cliResponse {
				return vms.status(c)
			},
			Helpshort: "print the status of each VM",
			Helplong: `
Usage: vm_status <optional VM id>
Print the status for all or one VM, depending on if you supply the optional VM
id field.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"quit": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 0 {
					return cliResponse{
						Error: "quit takes no arguments",
					}
				}
				teardown()
				return cliResponse{}
			},
			Helpshort: "quit",
			Helplong:  "Quit",
			Record:    true, // but how!?
			Clear: func() error {
				return nil
			},
		},

		"exit": &command{ // just an alias to quit
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 0 {
					return cliResponse{
						Error: "exit takes no arguments",
					}
				}
				teardown()
				return cliResponse{}
			},
			Helpshort: "exit",
			Helplong:  "Exit",
			Record:    true, // but how!?
			Clear: func() error {
				return nil
			},
		},

		"vm_launch": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 1 {
					return cliResponse{
						Error: "vm_launch takes one argument",
					}
				}
				a, err := strconv.Atoi(c.Args[0])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				ksmEnable()
				vms.launch(a)
				return cliResponse{}
			},
			Helpshort: "launch virtual machines in a paused state",
			Helplong: `
Usage: vm_launch <number of vms>
Launch <number of vms> virtual machines in a paused state, using the parameters
defined leading up to the launch command. Any changes to the VM parameters 
after launching will have no effect on launched VMs.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_kill": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) != 1 {
					return cliResponse{
						Error: "vm_kill takes one argument",
					}
				}
				a, err := strconv.Atoi(c.Args[0])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				vms.kill(a)
				return cliResponse{}
			},
			Helpshort: "kill running virtual machines",
			Helplong: `
Usage: vm_kill <vm id>
Kill a virtual machine by ID. Pass -1 to kill all virtual machines.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_start": &command{
			Call: func(c cliCommand) cliResponse {
				return vms.start(c)
			},
			Helpshort: "start paused virtual machines",
			Helplong: `
Usage: vm_start <optional VM id>
Start all or one paused virtual machine. To start all paused virtual machines,
call start without the optional VM id.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_qemu": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: process("qemu"),
					}
				} else if len(c.Args) == 1 {
					externalProcesses["qemu"] = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_qemu takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set the qemu process to invoke",
			Helplong:  "Set the qemu process to invoke. Relative paths are ok.",
			Record:    true,
			Clear: func() error {
				externalProcesses["qemu"] = "qemu-system-x86_64"
				return nil
			},
		},

		"vm_memory": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.Memory,
					}
				} else if len(c.Args) == 1 {
					info.Memory = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_memory takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set the amount of physical memory for a VM",
			Helplong:  "Set the amount of physical memory to allocate in megabytes.",
			Record:    true,
			Clear: func() error {
				info.Memory = "512"
				return nil
			},
		},

		"vm_vcpus": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.Vcpus,
					}
				} else if len(c.Args) == 1 {
					info.Vcpus = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_vcpus takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set the number of virtual CPUs for a VM",
			Helplong:  "Set the number of virtual CPUs to allocate a VM.",
			Record:    true,
			Clear: func() error {
				info.Vcpus = "1"
				return nil
			},
		},

		"vm_disk": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.DiskPath,
					}
				} else if len(c.Args) == 1 {
					info.DiskPath = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_disk takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set a disk image to attach to a VM",
			Helplong: `
Attach a disk to a VM. Any disk image supported by QEMU is a valid parameter.
Disk images launched in snapshot mode may safely be used for multiple VMs.`,
			Record: true,
			Clear: func() error {
				info.DiskPath = ""
				return nil
			},
		},

		"vm_cdrom": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.CdromPath,
					}
				} else if len(c.Args) == 1 {
					info.CdromPath = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_cdrom takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set a cdrom image to attach to a VM",
			Helplong: `
Attach a cdrom to a VM. When using a cdrom, it will automatically be set
to be the boot device.`,
			Record: true,
			Clear: func() error {
				info.CdromPath = ""
				return nil
			},
		},

		"vm_kernel": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.KernelPath,
					}
				} else if len(c.Args) == 1 {
					info.KernelPath = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_kernel takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set a kernel image to attach to a VM",
			Helplong: `
Attach a kernel image to a VM. If set, QEMU will boot from this image instead
of any disk image.`,
			Record: true,
			Clear: func() error {
				info.KernelPath = ""
				return nil
			},
		},

		"vm_initrd": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.InitrdPath,
					}
				} else if len(c.Args) == 1 {
					info.InitrdPath = c.Args[0]
				} else {
					return cliResponse{
						Error: "vm_initrd takes only one argument",
					}
				}
				return cliResponse{}
			},
			Helpshort: "set a initrd image to attach to a VM",
			Helplong: `
Attach an initrd image to a VM. Passed along with the kernel image at boot time.`,
			Record: true,
			Clear: func() error {
				info.InitrdPath = ""
				return nil
			},
		},

		"vm_qemu_append": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: strings.Join(info.QemuAppend, " "),
					}
				} else {
					info.QemuAppend = c.Args
				}
				return cliResponse{}
			},
			Helpshort: "add additional arguments for the QEMU command",
			Helplong: `
Add additional arguments to be passed to the QEMU instance. For example,
"-serial tcp:localhost:4001".
`,
			Record: true,
			Clear: func() error {
				info.QemuAppend = nil
				return nil
			},
		},

		"vm_append": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) == 0 {
					return cliResponse{
						Response: info.Append,
					}
				} else {
					info.Append = strings.Join(c.Args, " ")
				}
				return cliResponse{}
			},
			Helpshort: "set an append string to pass to a kernel set with vm_kernel",
			Helplong: `
Add an append string to a kernel set with vm_kernel. Setting vm_append without
using vm_kernel will result in an error.

For example, to set a static IP for a linux VM:

vm_append "ip=10.0.0.5 gateway=10.0.0.1 netmask=255.255.255.0 dns=10.10.10.10"
`,
			Record: true,
			Clear: func() error {
				info.Append = ""
				return nil
			},
		},

		"vm_net": &command{
			Call: func(c cliCommand) cliResponse {
				r := cliResponse{}
				if len(c.Args) == 0 {
					return cliResponse{
						Response: fmt.Sprintf("%v\n", info.Networks),
					}
				} else {
					info.Networks = []int{}
					for _, lan := range c.Args {
						val, err := strconv.Atoi(lan)
						if err != nil {
							return cliResponse{
								Error: err.Error(),
							}
						}
						err, ok := currentBridge.LanCreate(val)
						if !ok {
							return cliResponse{
								Error: err.Error(),
							}
						}
						if err == nil {
							r.Response = fmt.Sprintln("creating new lan:", val)
						}
						info.Networks = append(info.Networks, val)
					}
				}
				return r
			},
			Helpshort: "specify the networks the VM is a member of",
			Helplong: `
Usage: vm_net <name> <optional addtional names>
Specify the network(s) that the VM is a member of by name. A corresponding VLAN
will be created for each named network. For example, to connect a VM to VLAN 1
and 5:

vm_net 1 5

Calling vm_net with no parameters will list the current networks for this VM.`,
			Record: true,
			Clear: func() error {
				info.Networks = []int{}
				return nil
			},
		},

		"vnc": &command{
			Call:      cliVnc,
			Helpshort: "invoke a vnc viewer on a VM or start a vnc pool server",
			Helplong: `
Usage: vnc [serve <host:port>, novnc <novnc path>]
Launch a webserver that allows you to browse the connected minimega hosts and 
VMs, and connect to any VM in the pool.

This command requires access to an installation of novnc. By default minimega
looks in 'pwd'/misc/novnc. To set a different path, invoke:

vnc novnc <path to novnc>

To start the vnc webserver, issue the vnc serve command with a host and port. 
For example, if you wanted to serve on localhost, port 8080, invoke:

vnc serve :8080

:8080 is the default port.`,
			Record: true,
			Clear: func() error {
				vncNovnc = "misc/novnc"
				return nil
			},
		},

		"history": &command{
			Call: func(c cliCommand) cliResponse {
				r := cliResponse{}
				if len(c.Args) != 0 {
					r.Error = "history takes no arguments"
				} else {
					r.Response = strings.Join(commandBuf, "\n")

				}
				return r
			},
			Helpshort: "shows the command history",
			Helplong: `
shows the command history`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"clear": &command{
			Call: func(c cliCommand) cliResponse {
				var r cliResponse
				if len(c.Args) != 1 {
					return cliResponse{
						Error: "clear takes one argument",
					}
				}
				cc := c.Args[0]
				if cliCommands[cc] == nil {
					e := fmt.Sprintf("invalid command: %v", cc)
					r.Error = e
				} else {
					e := cliCommands[cc].Clear()
					if e != nil {
						r.Error = e.Error()
					}
				}
				return r
			},
			Helpshort: "restore a variable to its default state",
			Helplong: `
Restores a variable to its default state or clears it. For example, 'clear net'
will clear the list of associated networks.`,
			Record: true,
			Clear: func() error {
				return fmt.Errorf("it's unclear how to clear clear")
			},
		},

		"help": &command{
			Call: func(c cliCommand) cliResponse {
				r := cliResponse{}
				if len(c.Args) == 0 { // display help on help, and list the short helps
					r.Response = "Display help on a command. Here is a list of commands:\n"
					var sortedNames []string
					for c, _ := range cliCommands {
						sortedNames = append(sortedNames, c)
					}
					sort.Strings(sortedNames)
					w := new(tabwriter.Writer)
					buf := bytes.NewBufferString(r.Response)
					w.Init(buf, 0, 8, 0, '\t', 0)
					for _, c := range sortedNames {
						fmt.Fprintln(w, c, "\t", ":\t", cliCommands[c].Helpshort, "\t")
					}
					w.Flush()
					r.Response = buf.String()
				} else if len(c.Args) == 1 { // try to display help on args[0]
					if cliCommands[c.Args[0]] != nil {
						r.Response = fmt.Sprintln(c.Args[0], ":", cliCommands[c.Args[0]].Helpshort)
						r.Response += fmt.Sprintln(cliCommands[c.Args[0]].Helplong)
					} else {
						e := fmt.Sprintf("no help on command: %v", c.Args[0])
						r.Error = e
					}
				} else {
					r.Error = "help takes one argument"
				}
				return r
			},
			Helpshort: "show this help message",
			Helplong:  ``,
			Record:    false,
			Clear: func() error {
				return nil
			},
		},

		"host_tap": &command{
			Call:      hostTapCreate,
			Helpshort: "create a host tap for communicating between hosts and VMs",
			Helplong: `
Create host tap on a named vlan for communicating between a host and any VMs on
that vlan. host_tap takes two arguments, the named vlan to tap and an 
ip/netmask. It returns the name of the created tap if successful.

For example, to create a host tap with ip and netmask 10.0.0.1/24 on VLAN 5:

host_tap 5 10.0.0.1/24`,
			Record: true,
			Clear: func() error {
				return nil //perhaps calling this should remove all host taps
			},
		},

		"mesh_degree": &command{
			Call:      meshageDegree,
			Helpshort: "view or set the current degree for this mesh node",
			Helplong: `
View or set the current degree for this mesh node.`,
			Record: true,
			Clear: func() error {
				meshageNode.SetDegree(0)
				return nil
			},
		},

		"mesh_dial": &command{
			Call:      meshageDial,
			Helpshort: "connect this node to another",
			Helplong: `
Attempt to connect to another listening node.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"mesh_dot": &command{
			Call:      meshageDot,
			Helpshort: "output a graphviz formatted dot file",
			Helplong: `
Output a graphviz formatted dot file representing the connected topology.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"mesh_status": &command{
			Call:      meshageStatus,
			Helpshort: "display a short status report of the mesh",
			Helplong: `
Display a short status report of the mesh.`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"mesh_list": &command{
			Call:      meshageList,
			Helpshort: "display the mesh adjacency list",
			Helplong: `
Display the mesh adjacency list.`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"mesh_hangup": &command{
			Call:      meshageHangup,
			Helpshort: "disconnect from a client",
			Helplong: `
Disconnect from a client.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"mesh_msa_timeout": &command{
			Call:      meshageMSATimeout,
			Helpshort: "view or set the MSA timeout",
			Helplong: `
View or the the Meshage State Announcement timeout.`,
			Record: true,
			Clear: func() error {
				meshageNode.SetMSATimeout(60)
				return nil
			},
		},

		"mesh_set": &command{
			Call:      meshageSet,
			Helpshort: "send a command to one or more connected clients",
			Helplong: `
Send a command to one or more connected clients.
For example, to get the vm_status from nodes kn1 and kn2:
	mesh_set kn[1-2] vm_status`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"mesh_broadcast": &command{
			Call:      meshageBroadcast,
			Helpshort: "send a command to all connected clients",
			Helplong: `
Send a command to all connected clients.
For example, to get the vm_status from all nodes:
	mesh_broadcast vm_status`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"hostname": &command{
			Call: func(c cliCommand) cliResponse {
				host, err := os.Hostname()
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
				return cliResponse{
					Response: host,
				}
			},
			Helpshort: "return the hostname",
			Helplong: `
Return the hostname`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"dnsmasq": &command{
			Call:      dnsmasqCLI,
			Helpshort: "start a dhcp/dns server on a specified ip",
			Helplong: `
Start a dhcp/dns server on a specified IP with a specified range.
For example, to start a DHCP server on IP 10.0.0.1 serving the range 10.0.0.2 - 10.0.254.254:

dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254

To list running dnsmasq servers, invoke dnsmasq with no arguments.
To kill a running dnsmasq server, specify its ID from the list of running servers:
For example, to kill dnsmasq server 2:

dnsmasq kill 2

To kill all running dnsmasq servers, pass -1 as the ID:

dnsmasq kill -1

dnsmasq will provide DNS service from the host, as well as from /etc/hosts. You can specify
an additional hosts file to serve by providing a file as an additional argument. For example,
to start a dnsmasq server serving the range as above and serving dns for a list of hosts in
the file "addn-hosts":

dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254 /tmp/addn-hosts

NOTE: If specifying an additional hosts file, you must provide the full path to the file.
`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},
	}
}

func makeCommand(s string) cliCommand {
	f := strings.Fields(s)
	var command string
	var args []string
	if len(f) > 0 {
		command = f[0]
	}
	if len(f) > 1 {
		args = f[1:]
	}
	return cliCommand{
		Command: command,
		Args:    args,
	}
}

// local command line interface, wrapping readline
func cli() {
	for {
		prompt := "minimega$ "
		line, err := goreadline.Rlwrap(prompt)
		if err != nil {
			break // EOF
		}
		log.Debug("got from stdin:", line)

		c := makeCommand(string(line))

		commandChanLocal <- c
		for {
			r := <-ackChanLocal
			if r.Error != "" {
				log.Errorln(r.Error)
			}
			if r.Response != "" {
				if strings.HasSuffix(r.Response, "\n") {
					fmt.Print(r.Response)
				} else {
					fmt.Println(r.Response)
				}
			}
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}
}

func cliMux() {
	for {
		select {
		case c := <-commandChanLocal:
			c.ackChan = ackChanLocal
			ackChanLocal <- cliExec(c)
		case c := <-commandChanSocket:
			c.ackChan = ackChanSocket
			ackChanSocket <- cliExec(c)
		case c := <-commandChanMeshage:
			c.ackChan = ackChanMeshage
			ackChanMeshage <- cliExec(c)
		}
	}
}

// process commands from the command channel. each command is acknowledged with
// true/false success codes on commandAck.
func cliExec(c cliCommand) cliResponse {
	if c.Command == "" {
		return cliResponse{}
	}

	// special case, comments. Any line starting with # is a comment and WILL be 
	// recorded.
	if strings.HasPrefix(c.Command, "#") {
		log.Debugln("comment:", c.Command, c.Args)
		s := c.Command
		if len(c.Args) > 0 {
			s += " " + strings.Join(c.Args, " ")
		}
		commandBuf = append(commandBuf, s)
		return cliResponse{}
	}

	if cliCommands[c.Command] == nil {
		e := fmt.Sprintf("invalid command: %v", c.Command)
		return cliResponse{
			Error: e,
		}
	}
	r := cliCommands[c.Command].Call(c)
	if r.Error == "" {
		if cliCommands[c.Command].Record {
			s := c.Command
			if len(c.Args) > 0 {
				s += " " + strings.Join(c.Args, " ")
			}
			commandBuf = append(commandBuf, s)
		}
	}
	return r
}
