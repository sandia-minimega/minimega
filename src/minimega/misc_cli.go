// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
	"version"
)

var miscCLIHandlers = []minicli.Handler{
	{ // quit
		HelpShort: "quit minimega",
		HelpLong: `
Quit. An optional integer argument X allows deferring the quit call for X
seconds. This is useful for telling a mesh of minimega nodes to quit.

quit will not return a response to the cli, control socket, or meshage, it will
simply exit. meshage connected nodes catch this and will remove the quit node
from the mesh. External tools interfacing minimega must check for EOF on stdout
or the control socket as an indication that minimega has quit.`,
		Patterns: []string{
			"quit [delay]",
		},
		Call: wrapSimpleCLI(cliQuit),
	},
	{ // help
		HelpShort: "show command help",
		HelpLong: `
Show help on a command. If called with no arguments, show a summary of all
commands.`,
		Patterns: []string{
			"help [command]...",
		},
		Call: wrapSimpleCLI(cliHelp),
	},
	{ // read
		HelpShort: "read and execute a command file",
		HelpLong: `
Read a command file and execute it. This has the same behavior as if you typed
the file in manually.`,
		Patterns: []string{
			"read <file>",
		},
		Call: cliRead,
	},
	{ // debug
		HelpShort: "display internal debug information",
		Patterns: []string{
			"debug",
		},
		Call: wrapSimpleCLI(cliDebug),
	},
	{ // version
		HelpShort: "display the minimega version",
		Patterns: []string{
			"version",
		},
		Call: wrapSimpleCLI(cliVersion),
	},
	{ // echo
		HelpShort: "display text after macro expansion and comment removal",
		Patterns: []string{
			"echo [args]...",
		},
		Call: wrapSimpleCLI(cliEcho),
	},
}

func init() {
	registerHandlers("misc", miscCLIHandlers)
}

func cliQuit(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if v, ok := c.StringArgs["delay"]; ok {
		delay, err := strconv.Atoi(v)
		if err != nil {
			resp.Error = err.Error()
		} else {
			go func() {
				time.Sleep(time.Duration(delay) * time.Second)
				teardown()
			}()
			resp.Response = fmt.Sprintf("quitting after %v seconds", delay)
		}
	} else {
		teardown()
	}

	return resp
}

func cliHelp(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	input := ""
	if args, ok := c.ListArgs["command"]; ok {
		input = strings.Join(args, " ")
	}

	resp.Response = minicli.Help(input)
	return resp
}

func cliRead(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	file, err := os.Open(c.StringArgs["file"])
	if err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
		return
	}
	defer file.Close()

	count := 0

	r := bufio.NewReader(file)
	for ; ; count++ {
		line, _, err := r.ReadLine()
		if err == io.EOF {
			break
		} else if err != nil {
			resp.Error = err.Error()
			break
		}
		command := string(line)
		log.Debug("read command: %v", command) // commands don't have their newlines removed

		// HAX: Make sure we don't have a recursive read command
		if strings.HasPrefix(command, "read") {
			resp.Error = "read cannot run other read commands"
			break
		}

		// TODO: Should we run a write command?

		cmd, err := minicli.CompileCommand(command)
		if err != nil {
			resp.Error = err.Error()
			break
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		for resp := range minicli.ProcessCommand(cmd, true) {
			respChan <- resp

			// Stop processing at the first error if there is one response.
			// TODO: What should we do if the command was mesh send and there
			// is a mixture of success and failure?
			if len(resp) == 1 && resp[0].Error != "" {
				break
			}
		}
	}

	// TODO: Should we send the response from read?
	resp.Response = fmt.Sprintf("read processed %d commands", count)
	respChan <- minicli.Responses{resp}
}

func cliDebug(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:   hostname,
		Header: []string{"Go version", "Goroutines", "CGO calls"},
		Tabular: [][]string{
			[]string{
				runtime.Version(),
				strconv.Itoa(runtime.NumGoroutine()),
				strconv.FormatInt(runtime.NumCgoCall(), 10),
			},
		},
	}
}

func cliVersion(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:     hostname,
		Response: fmt.Sprintf("minimega %v %v", version.Revision, version.Date),
	}
}

func cliEcho(c *minicli.Command) *minicli.Response {
	return &minicli.Response{
		Host:     hostname,
		Response: strings.Join(c.ListArgs["args"], " "),
	}
}
