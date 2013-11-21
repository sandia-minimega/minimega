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
	"version"
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
	Command string
	Args    []string
	ackChan chan cliResponse
	TID     int32
}

type cliResponse struct {
	Response string
	Error    string // because you can't gob/json encode an error type
	More     bool   // more is set if the called command will be sending multiple responses
	TID      int32
}

type command struct {
	Call      func(c cliCommand) cliResponse // callback function
	Helpshort string                         // short form help test, one line only
	Helplong  string                         // long form help text
	Record    bool                           // record in the command history
	Clear     func() error                   // clear/restore to default state
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
Log to a file. To disable file logging, call "clear log_file".`,
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

		"vm_save": &command{
			Call: func(c cliCommand) cliResponse {
				if len(c.Args) < 2 {
					return cliResponse{
						Error: "Usage: vm_save <save name> <vm id> [<vm id> ...]",
					}
				}

				file, err := os.Create("/etc/minimega/saved_vms/" + c.Args[0])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}

				for _, vmStr := range c.Args[1:] { // iterate over the vm id's specified
					vmId, err := strconv.Atoi(vmStr)
					if err != nil {
						return cliResponse{
							Error: err.Error(),
						}
					}

					vm, ok := vms.vms[vmId]
					if !ok {
						log.Error("Not a valid id: " + vmStr)
						continue
					}

					// build up the command list to re-launch this vm
					cmds := []string{}
					cmds = append(cmds, "vm_memory "+vm.Memory)
					cmds = append(cmds, "vm_vcpus "+vm.Vcpus)

					if vm.DiskPath != "" {
						cmds = append(cmds, "vm_disk "+vm.DiskPath)
					} else {
						cmds = append(cmds, "clear vm_disk")
					}

					if vm.CdromPath != "" {
						cmds = append(cmds, "vm_cdrom "+vm.CdromPath)
					} else {
						cmds = append(cmds, "clear vm_cdrom")
					}

					if vm.KernelPath != "" {
						cmds = append(cmds, "vm_kernel "+vm.KernelPath)
					} else {
						cmds = append(cmds, "clear vm_kernel")
					}

					if vm.InitrdPath != "" {
						cmds = append(cmds, "vm_initrd "+vm.InitrdPath)
					} else {
						cmds = append(cmds, "clear vm_initrd")
					}

					if vm.Append != "" {
						cmds = append(cmds, "vm_append "+vm.Append)
					} else {
						cmds = append(cmds, "clear vm_append")
					}

					if len(vm.QemuAppend) != 0 {
						cmds = append(cmds, "vm_qemu_append "+strings.Join(vm.QemuAppend, " "))
					} else {
						cmds = append(cmds, "clear vm_qemu_append")
					}

					cmds = append(cmds, "vm_snapshot "+strconv.FormatBool(vm.Snapshot))
					if len(vm.Networks) != 0 {
						netString := "vm_net "
						for i, vlan := range vm.Networks {
							netString += strconv.Itoa(vlan) + "," + vm.macs[i] + " "
						}
						cmds = append(cmds, strings.TrimSpace(netString))
					} else {
						cmds = append(cmds, "clear vm_net")
					}

					if vm.Name != "" {
						cmds = append(cmds, "vm_launch "+vm.Name)
					} else {
						cmds = append(cmds, "vm_launch 1")
					}

					// write commands to file
					for _, cmd := range cmds {
						_, err = file.WriteString(cmd + "\n")
						if err != nil {
							return cliResponse{
								Error: err.Error(),
							}
						}
					}
				}
				return cliResponse{}
			},
			Helpshort: "save a vm configuration for later use",
			Helplong: `
Usage: vm_save <save name> <vm id> [<vm id> ...]
Saves the configuration of a running virtual machine or set of virtual 
machines so that it/they can be restarted/recovered later, such as after 
a system crash.
This command does not store the state of the virtual machine itself, 
only its launch configuration.
			`,
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
						} else {
							return cliResponse{
								Error: err.Error(),
							}
						}
					}
					log.Debug("read command: %v", string(l)) // commands don't have their newlines removed
					resp := cliExec(makeCommand(string(l)))
					resp.More = true
					c.ackChan <- resp
					if resp.Error != "" {
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

		"vm_info": &command{
			Call: func(c cliCommand) cliResponse {
				return vms.info(c)
			},
			Helpshort: "print information about VMs",
			Helplong: `
Usage: vm_info <optional search term> <optional output mask>
Print information about VMs. vm_info allows searching for VMs based on any VM
parameter, and output some or all information about the VMs in question.
Additionally, you can display information about all running VMs. 

A vm_info command takes two optional arguments, a search term, and an output
mask. If the search term is omitted, information about all VMs will be
displayed. If the output mask is omitted, all information about the VMs will be
displayed.

The search term uses a single key=value argument. For example, if you want all
information about VM 50: 
	vm_info id=50

The output mask uses an ordered list of fields inside [] brackets. For example,
if you want the ID and IPs for all VMs on vlan 100: 
	vm_info vlan=100 [id,ip]

Searchable and maskable fields are:
	host	: The host that the VM is running on
	id	: The VM ID, as an integer
	name	: The VM name, if it exists
	memory  : Allocated memory, in megabytes
	disk    : disk image
	initrd  : initrd image
	kernel  : kernel image
	cdrom   : cdrom image
	state   : one of (building, running, paused, quit, error)
	tap	: tap name
	mac	: mac address
	ip	: IPv4 address
	ip6	: IPv6 address
	vlan	: vlan, as an integer

Examples:
Display a list of all IPs for all VMs:
	vm_info [ip,ip6]

Display all information about VMs with the disk image foo.qc2:
	vm_info disk=foo.qc2

Display all information about all VMs:
	vm_info

`,
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
				return vms.launch(c)
			},
			Helpshort: "launch virtual machines in a paused state",
			Helplong: `
Usage: vm_launch <number of vms or vm name>
Launch <number of vms or vm name> virtual machines in a paused state, using the parameters
defined leading up to the launch command. Any changes to the VM parameters 
after launching will have no effect on launched VMs.

If you supply a name instead of a number of VMs, one VM with that name will be launched.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_kill": &command{
			Call: func(c cliCommand) cliResponse {
				return vms.kill(c)
			},
			Helpshort: "kill running virtual machines",
			Helplong: `
Usage: vm_kill <vm id or name>
Kill a virtual machine by ID or name. Pass -1 to kill all virtual machines.`,
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
Usage: vm_start <optional VM id or name>
Start all or one paused virtual machine. To start all paused virtual machines,
call start without the optional VM ID or name.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_stop": &command{
			Call: func(c cliCommand) cliResponse {
				return vms.stop(c)
			},
			Helpshort: "stop/pause virtual machines",
			Helplong: `
Usage: vm_stop <optional VM id or name>
Stop all or one running virtual machine. To stop all running virtual machines,
call stop without the optional VM ID or name.

			Calling stop will put VMs in a paused state. Start stopped VMs with vm_start`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_qemu": &command{
			Call:      cliVMQemu,
			Helpshort: "set the qemu process to invoke",
			Helplong:  "Set the qemu process to invoke. Relative paths are ok.",
			Record:    true,
			Clear: func() error {
				externalProcesses["qemu"] = "qemu-system-x86_64"
				return nil
			},
		},

		"vm_memory": &command{
			Call:      cliVMMemory,
			Helpshort: "set the amount of physical memory for a VM",
			Helplong:  "Set the amount of physical memory to allocate in megabytes.",
			Record:    true,
			Clear: func() error {
				info.Memory = "512"
				return nil
			},
		},

		"vm_vcpus": &command{
			Call:      cliVMVCPUs,
			Helpshort: "set the number of virtual CPUs for a VM",
			Helplong:  "Set the number of virtual CPUs to allocate a VM.",
			Record:    true,
			Clear: func() error {
				info.Vcpus = "1"
				return nil
			},
		},

		"vm_disk": &command{
			Call:      cliVMDisk,
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
			Call:      cliVMCdrom,
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
			Call:      cliVMKernel,
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
			Call:      cliVMInitrd,
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
			Call:      cliVMQemuAppend,
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
			Call:      cliVMAppend,
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
			Call:      cliVMNet,
			Helpshort: "specify the networks the VM is a member of",
			Helplong: `
Usage: vm_net <id>[,<mac address>] [<id>[,<mac address] ...]
Specify the network(s) that the VM is a member of by id. A corresponding VLAN
will be created for each network. Optionally, you may specify the mac address 
of the interface to connect to that network. If not specifed, the mac address
will be randomly generated.

Examples:

To connect a VM to VLANs 1 and 5:
	vm_net 1 5
To connect a VM to VLANs 100, 101, and 102 with specific mac addresses:
	vm_net 100,00:00:00:00:00:00 101,00:00:00:00:01:00 102,00:00:00:00:02:00

			Calling vm_net with no parameters will list the current networks for this VM.`,
			Record: true,
			Clear: func() error {
				info.Networks = []int{}
				return nil
			},
		},

		"web": &command{
			Call:      WebCLI,
			Helpshort: "start the minimega web interface",
			Helplong: `
Usage: web [port, novnc <novnc path>]
Launch a webserver that allows you to browse the connected minimega hosts and 
VMs, and connect to any VM in the pool.

This command requires access to an installation of novnc. By default minimega
looks in 'pwd'/misc/novnc. To set a different path, invoke:

	web novnc <path to novnc>

To start the webserver on a specific port, issue the web command with the port:
			web 7000

8080 is the default port.`,
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
Shows the command history`,
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
			Call:      hostTap,
			Helpshort: "control host taps for communicating between hosts and VMs",
			Helplong: `
Control host taps on a named vlan for communicating between a host and any VMs
on that vlan. 

Calling host_tap with no arguments will list all created host_taps.

To create a host_tap on a particular vlan, invoke host_tap with the create
command:

	host_tap create <vlan> <ip/dhcp>

For example, to create a host tap with ip and netmask 10.0.0.1/24 on VLAN 5:

	host_tap create 5 10.0.0.1/24

Additionally, you can bring the tap up with DHCP by using "dhcp" instead of a
ip/netmask:

	host_tap create 5 dhcp

To delete a host tap, use the delete command and tap name from the host_tap list:

	host_tap delete <id>
	
To delete all host taps, use id -1, or 'clear host_tap':
	
	host_tap delete -1`,
			Record: true,
			Clear: func() error {
				resp := hostTapDelete("-1")
				if resp.Error == "" {
					return nil
				}
				return fmt.Errorf("%v", resp.Error)
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

		"mesh_timeout": &command{
			Call:      meshageTimeoutCLI,
			Helpshort: "view or set the mesh timeout",
			Helplong: `
View or set the timeout on sending mesh commands.

When a mesh command is issued, if a response isn't sent within mesh_timeout
seconds, the command will be dropped and any future response will be discarded.
Note that this does not cancel the outstanding command - the node receiving the
command may still complete - but rather this node will stop waiting on a
response.`,
			Record: true,
			Clear: func() error {
				meshageTimeout = meshageTimeoutDefault
				return nil
			},
		},

		"mesh_set": &command{
			Call:      meshageSet,
			Helpshort: "send a command to one or more connected clients",
			Helplong: `
Send a command to one or more connected clients.
For example, to get the vm_info from nodes kn1 and kn2:
	mesh_set kn[1-2] vm_info`,
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
For example, to get the vm_info from all nodes:
	mesh_broadcast vm_info`,
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
			Return the hostname.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"dnsmasq": &command{
			Call:      dnsmasqCLI,
			Helpshort: "start a dhcp/dns server on a specified ip",
			Helplong: `
Start a dhcp/dns server on a specified IP with a specified range.  For example,
to start a DHCP server on IP 10.0.0.1 serving the range 10.0.0.2 -
10.0.254.254:

	dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254

To list running dnsmasq servers, invoke dnsmasq with no arguments.  To kill a
running dnsmasq server, specify its ID from the list of running servers: For
example, to kill dnsmasq server 2:

	dnsmasq kill 2

To kill all running dnsmasq servers, pass -1 as the ID:

	dnsmasq kill -1

dnsmasq will provide DNS service from the host, as well as from /etc/hosts. 
You can specify an additional config file for dnsmasq by providing a file as an
additional argument.

	dnsmasq start 10.0.0.1 10.0.0.2 10.0.254.254 /tmp/dnsmasq-extra.conf

NOTE: If specifying an additional config file, you must provide the full path to
the file.
			`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"shell": &command{
			Call:      shellCLI,
			Helpshort: "execute a command",
			Helplong: `
Execute a command under the credentials of the running user. 

Commands run until they complete or error, so take care not to execute a command
that does not return.
			`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"background": &command{
			Call:      backgroundCLI,
			Helpshort: "execute a command in the background",
			Helplong: `
Execute a command under the credentials of the running user. 

Commands run in the background and control returns immediately. Any output is
logged.
			`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"host_stats": &command{
			Call:      hostStatsCLI,
			Helpshort: "report statistics about the host",
			Helplong: `
Report statistics about the host including hostname, load averages, total and
free memory, and current bandwidth usage",
`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_snapshot": &command{
			Call:      cliVMSnapshot,
			Helpshort: "enable or disable snapshot mode when using disk images",
			Helplong: `
Enable or disable snapshot mode when using disk images. When enabled, disks
images will be loaded in memory when run and changes will not be saved. This
allows a single disk image to be used for many VMs.
`,
			Record: true,
			Clear: func() error {
				info.Snapshot = true
				return nil
			},
		},

		"ksm": &command{
			Call:      ksmCLI,
			Helpshort: "enable or disable Kernel Samepage Merging",
			Helplong: `
Enable or disable Kernel Samepage Merging, which can vastly increase the
density of VMs a node can run depending on how similar the VMs are.
`,
			Record: true,
			Clear: func() error {
				ksmDisable()
				return nil
			},
		},

		"version": &command{
			Call: func(c cliCommand) cliResponse {
				return cliResponse{
					Response: fmt.Sprintf("minimega %v %v", version.Revision, version.Date),
				}
			},
			Helpshort: "display the version",
			Helplong: `
Display the version.
`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_config": &command{
			Call:      cliVMConfig,
			Helpshort: "display the current VM configuration",
			Helplong: `
Display the current VM configuration.
`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"debug": &command{
			Call:      cliDebug,
			Helpshort: "display internal debug information",
			Helplong: `
Display internal debug information
`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"bridge_info": &command{
			Call:      cliBridgeInfo,
			Helpshort: "display information about the virtual bridge",
			Helplong: `
Display information about the virtual bridge.
`,
			Record: false,
			Clear: func() error {
				return nil
			},
		},

		"vm_flush": &command{
			Call:      cliVMFlush,
			Helpshort: "discard information about quit or failed VMs",
			Helplong: `
Discard information about VMs that have either quit or encountered an error.
This will remove any VMs with a state of "quit" or "error" from vm_info. Names
of VMs that have been flushed may be reused.
`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"file": &command{
			Call:      cliFile,
			Helpshort: "work with files served by minimega",
			Helplong: `
file allows you to transfer and manage files served by minimega in the
directory set by the -filepath flag (default is <base directory>/files).

To list files currently being served, issue the list command with a directory
relative to the served directory:

	file list /foo

Issuing "file list /" will list the contents of the served directory.

Files can be deleted with the delete command:

	file delete /foo

If a directory is given, the directory will be recursively deleted.

Files are transferred using the get command. When a get command is issued, the
node will begin searching for a file matching the path and name within the
mesh. If the file exists, it will be transferred to the requesting node. If
multiple different files exist with the same name, the behavior is undefined.
When a file transfer begins, control will return to minimega while the
transfer completes.

To see files that are currently being transferred, use the status command:

	file status`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"viz": &command{
			Call:      cliDot,
			Helpshort: "visualize the current experiment as a graph",
			Helplong: `
viz outputs the current experiment topology as a graphviz readable 'dot' file.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vyatta": &command{
			Call:      cliVyatta,
			Helpshort: "define vyatta configuration images",
			Helplong: `
Define and write out vyatta router floppy disk images. 

vyatta takes a number of subcommands: 

	'dhcp': Add DHCP service to a particular network by specifying the
	network, default gateway, and start and stop addresses. For example, to
	serve dhcp on 10.0.0.0/24, with a default gateway of 10.0.0.1:
		
		vyatta dhcp add 10.0.0.0/24 10.0.0.1 10.0.0.2 10.0.0.254

		An optional DNS argument can be used to override the
		nameserver. For example, to do the same as above with a
		nameserver of 8.8.8.8:

		vyatta dhcp add 10.0.0.0/24 10.0.0.1 10.0.0.2 10.0.0.254 8.8.8.8

	'interfaces': Add IPv4 addresses using CIDR notation. Optionally,
	'dhcp' or 'none' may be specified. The order specified matches the
	order of VLANs used in vm_net. This number of arguments must either be
	0 or equal to the number of arguments in 'interfaces6' For example:

		vyatta interfaces 10.0.0.1/24 dhcp

	'interfaces6': Add IPv6 addresses similar to 'interfaces'. The number
	of arguments must either be 0 or equal to the number of arguments in
	'interfaces'.

	'rad': Enable router advertisements for IPv6. Valid arguments are IPv6
	prefixes or "none". Order matches that of interfaces6. For example:

		vyatta rad 2001::/64 2002::/64

	'ospf': Route networks using OSPF. For example:

		vyatta ospf 10.0.0.0/24 12.0.0.0/24

	'ospf3': Route IPv6 interfaces using OSPF3. For example:

		vyatta ospf3 eth0 eth1

	'routes': Set static routes. Routes are specified as <network>,<next-hop> ...
	For example:

		vyatta routes 2001::0/64,123::1 10.0.0.0/24,12.0.0.1

	'write': Write the current configuration to file. If a filename is
	omitted, a random filename will be used and the file placed in the path
	specified by the -filepath flag. The filename will be returned.`,
			Record: true,
			Clear:  cliVyattaClear,
		},

		"vm_hotplug": &command{
			Call:      cliVMHotplug,
			Helpshort: "add and remove USB drives",
			Helplong: `
Add and remove USB drives to a launched VM. 

To view currently attached media, call vm_hotplug with the 'show' argument and
a VM ID or name. To add a device, use the 'add' argument followed by the VM ID
or name, and the name of the file to add. For example, to add foo.img to VM 5:

	vm_hotplug add 5 foo.img

The add command will return a disk ID. To remove media, use the 'remove'
argument with the VM ID and the disk ID. For example, to remove the drive added
above, named 0:

	vm_hotplug remove 5 0

To remove all hotplug devices, use ID -1.`,
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
