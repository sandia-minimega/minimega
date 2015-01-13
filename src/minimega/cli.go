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
	"fmt"
	"goreadline"
	"minicli"
	log "minilog"
	"os"
	"sort"
	"strings"
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
	}
}

func makeCommand(s string) cliCommand {
	return cliCommand{}
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
