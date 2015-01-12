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
	"bytes"
	"fmt"
	"gomacro"
	"goreadline"
	"minicli"
	log "minilog"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
)

const (
	COMMAND_TIMEOUT = 10
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

	macro *gomacro.Macro
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

	macro = gomacro.NewMacro()

	// list of commands the cli supports. some commands have small callbacks, which
	// are defined inline.
	cliCommands = map[string]*command{
		"vnc": &command{
			Call:      cliVNC,
			Helpshort: "record or playback VNC kbd/mouse input",
			Helplong: `
	Usage:
		vnc
		vnc [record <host> <vm id or name> <filename>, playback <host> <vm id or name> <filename>]
		vnc [norecord <host> <vm id or name>, noplayback <host> <vm id or name>]
		vnc clear

Record or playback keyboard and mouse events sent via the web interface to
the selected VM.

With no arguments, vnc will list currently recording or playing VNC sessions.

If record is selected, a file will be created containing a record of mouse and
keyboard actions by the user.

If playback is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.`,
			Record: false,
			Clear:  vncClear,
		},

		"mesh_msa_timeout": &command{
			Call:      meshageMSATimeout,
			Helpshort: "view or set the MSA timeout",
			Helplong: `
	Usage: mesh_msa_timeout [timeout]

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
	Usage: mesh_set [annotate] <recipients> <command>

Send a command to one or more connected clients.
For example, to get the vm_info from nodes kn1 and kn2:

	mesh_set kn[1-2] vm_info

Optionally, you can annotate the output with the hostname of all responders by
prepending the keyword 'annotate' to the command:

	mesh_set annotate kn[1-2] vm_info`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"mesh_broadcast": &command{
			Call:      meshageBroadcast,
			Helpshort: "send a command to all connected clients",
			Helplong: `
	Usage: mesh_broadcast [annotate] <command>

Send a command to all connected clients.
For example, to get the vm_info from all nodes:

	mesh_broadcast vm_info

Optionally, you can annotate the output with the hostname of all responders by
prepending the keyword 'annotate' to the command:

	mesh_broadcast annotate vm_info`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"file": &command{
			Call:      cliFile,
			Helpshort: "work with files served by minimega",
			Helplong: `
	Usage: file <list [path], get <file>, delete <file>, status>
file allows you to transfer and manage files served by minimega in the
directory set by the -filepath flag (default is 'base'/files).

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
	Usage: viz <filename>

Output the current experiment topology as a graphviz readable 'dot' file.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"vm_inject": &command{
			Call:      cliVMInject,
			Helpshort: "inject files into a qcow image",
			Helplong: `
	Usage: vm_inject <src qcow image>[:<partition>] [<dst qcow image name>] <src file1>:<dst file1> [<src file2>:<dst file2> ...]

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
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"define": &command{
			Call:      cliDefine,
			Helpshort: "define macros",
			Helplong: `
	Usage: define [macro[(<var1>[,<var2>...])] <command>]

Define literal and function like macros.

Macro keywords are in the form [a-zA-z0-9]+. When defining a macro, all text after the key is the macro expansion. For example:

	define key foo bar

Will replace "key" with "foo bar" in all command line arguments.

You can also specify function like macros in a similar way to function like macros in C. For example:

	define key(x,y) this is my x, this is my y

Will replace all instances of x and y in the expansion with the variable arguments. When used:

	key(foo,bar)

Will expand to:

	this is mbar foo, this is mbar bar

To show defined macros, invoke define with no arguments.`,
			Record: true,
			Clear: func() error {
				macro = gomacro.NewMacro()
				return nil
			},
		},

		"undefine": &command{
			Call:      cliUndefine,
			Helpshort: "undefine macros",
			Helplong: `
	Usage: undefine <macro>

Undefine macros by name.`,
			Record: true,
			Clear: func() error {
				return nil
			},
		},

		"capture": &command{
			Call:      cliCapture,
			Helpshort: "capture experiment data",
			Helplong: `
	Usage: capture [netflow <bridge> [file <filename> <raw,ascii> [gzip], socket <tcp,udp> <hostname:port> <raw,ascii>], clear <id, -1>]
	Usage: capture [pcap [bridge <bridge name> <filename>, vm <vm id or name> <vm interface index> <filename, clear <id, -1>]]

Capture experiment data including netflow and PCAP. Netflow capture obtains netflow data
from any local openvswitch switch, and can write to file, another socket, or
both. Netflow data can be written out in raw or ascii format, and file output
can be compressed on the fly. Multiple netflow writers can be configured.

PCAP capture can be from a bridge or VM interface. No filters are applied, and
all data seen on that interface is captured to file.

For example, to capture netflow data on bridge mega_bridge to file in ascii
mode and with gzip compression:

	minimega$ capture netflow mega_bridge file foo.netflow ascii gzip

You can change the active flow timeout with:

	minimega$ capture netflow mega_bridge timeout <timeout>

With <timeout> in seconds.

To capture pcap on bridge 'foo' to file 'foo.pcap':

	minimega$ capture pcap bridge foo foo.pcap

To capture pcap on VM 'foo' to file 'foo.pcap', using the 2nd interface on that
VM:

	minimega$ capture pcap vm foo 0 foo.pcap`,
			Record: true,
			Clear:  cliCaptureClear,
		},

		"cc": &command{
			Call:      cliCC,
			Helpshort: "command and control commands",
			Helplong: `
	Usage: cc [start [port]]
	Usage: cc filter [add <filter>=<arg> [<filter>=<arg>]..., delete <filter id>, clear]
	Usage: cc command [new [command=<command>] [filesend=<file>]... [filerecv=<file>]... [norecord] [background], delete <command id>]

Command and control virtual machines running the miniccc client. Commands may
include regular commands, backgrounded commands, and any number of sent and/or
received files. Commands will be executed in command creation order. For
example, to send a file 'foo' and display the contents on a remote VM:

	cc command new command="cat foo" filesend=foo

Responses are generated (unless the 'norecord' flag is set) and written out to
'<filebase>/miniccc_responses/<command id>/<client UUID>'. Files to be sent
must be in '<filebase>'.

Filters may be set to limit which clients may execute a posted command. Filters
are the logical sum of products of every filter added. That is, a single given
filter must match all given fields for the command to be executed. Multiple
filters are allowed, in which case any matched filter will allow the command to
execute. For example, to filter on VMs that are running windows AND have a
specific IP, OR nodes that have a range of IPs:

	cc filter add os=windows ip=10.0.0.1 cc filter add ip=12.0.0.0/24

New commands assign any current filters.
`,
			Record: true,
			Clear:  cliClearCC,
		},
	}

	var completionCandidates []string
	// set readline completion commands
	for k, _ := range cliCommands {
		completionCandidates = append(completionCandidates, k)
	}
	goreadline.SetCompletionCandidates(completionCandidates)
}

func (c cliCommand) String() string {
	args := strings.Join(c.Args, " ")
	return c.Command + " " + args
}

func cliDefine(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		m := macro.List()
		if len(m) == 0 {
			return cliResponse{}
		}

		// create output
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintln(&o, "macro\texpansion")
		for _, v := range m {
			k, e := macro.Macro(v)
			fmt.Fprintf(&o, "%v\t%v\n", k, e)
		}
		w.Flush()
		return cliResponse{
			Response: o.String(),
		}
	case 1:
		return cliResponse{
			Error: "define requires at least 2 arguments",
		}
	default:
		err := macro.Define(c.Args[0], strings.Join(c.Args[1:], " "))
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
	}
	return cliResponse{}
}

func cliUndefine(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "undefine takes exactly one argument",
		}
	}
	log.Debug("undefine %v", c.Args[0])
	macro.Undefine(c.Args[0])
	return cliResponse{}
}

func makeCommand(s string) cliCommand {
	// macro expansion
	// special case - don't expand 'define' or 'undefine'
	var input string
	f := strings.Fields(s)
	if len(f) > 0 {
		if f[0] != "define" && f[0] != "undefine" {
			input = macro.Parse(s)
		} else {
			input = s
		}
	}
	log.Debug("macro expansion %v -> %v", s, input)
	f = strings.Fields(input)
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
func cliLocal() {
	for {
		prompt := "minimega$ "
		line, err := goreadline.Rlwrap(prompt)
		if err != nil {
			break // EOF
		}
		command := string(line)
		log.Debug("got from stdin:", command)

		r, err := minicli.ProcessString(command)
		if err != nil {
			log.Errorln(err)
			continue
		}

		// print the responses
		fmt.Println(r)
	}
}

// process commands from the command channel. each command is acknowledged with
// true/false success codes on commandAck.
func cliExec(c cliCommand) cliResponse {
	if c.Command == "" {
		return cliResponse{}
	}

	// super special case
	if c.Command == "vm_vince" {
		log.Fatalln(poeticDeath)
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

	// special case, catch "mesh_set" on localhost
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}
	if c.Command == "mesh_set" && (c.Args[0] == hostname || (c.Args[0] == "annotate" && c.Args[1] == hostname)) {
		log.Debug("rewriting mesh_set %v as local command", hostname)
		if c.Args[0] == "annotate" {
			if len(c.Args) > 2 {
				c.Command = c.Args[2]
				if len(c.Args) > 3 {
					c.Args = c.Args[3:]
				} else {
					c.Args = []string{}
				}
			}
		} else {
			if len(c.Args) > 1 {
				c.Command = c.Args[1]
				if len(c.Args) > 2 {
					c.Args = c.Args[2:]
				} else {
					c.Args = []string{}
				}
			}
		}
		log.Debug("new command is %v", c)
	}

	r := cliCommands[c.Command].Call(c)
	if r.Error == "" {
		if cliCommands[c.Command].Record {
			s := c.Command
			if len(c.Args) > 0 {
				// BUG: need quote unescape in the new cli
				s += " " + strings.Join(c.Args, " ")
			}
			// special case, don't record "clear history"
			if s != "clear history" {
				commandBuf = append(commandBuf, s)
			}
		}
	}
	return r
}

// sort and walk the api, emitting markdown for each entry
func docGen() {
	var keys []string
	for k, _ := range cliCommands {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	fmt.Println("# minimega API")

	for _, k := range keys {
		fmt.Printf("<h2 id=%v>%v</h2>\n", k, k)
		fmt.Println(cliCommands[k].Helplong)
	}
}

var poeticDeath = `
Willst du immer weiterschweifen?
Sieh, das Gute liegt so nah.
Lerne nur das Glück ergreifen,
denn das Glück ist immer da.
`
