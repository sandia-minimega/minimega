// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"
	"version"
)

// cpuProfileOut is the output file for the active CPU profile. This is created
// by `debug cpu start ...` and closed by `debug cpu stop` and teardown.
var cpuProfileOut io.WriteCloser

var miscCLIHandlers = []minicli.Handler{
	{ // quit
		HelpShort: "quit minimega",
		HelpLong: `
Quit minimega. An optional integer argument X allows deferring the quit call
for X seconds. This is useful for telling a mesh of minimega nodes to quit.

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
the file in manually except that it stops after the first error.`,
		Patterns: []string{
			"read <file>",
		},
		Call: cliRead,
	},
	{ // validate
		HelpShort: "read and valid a command file",
		HelpLong: `
Read a command file and check that all the commands are syntactically valid.
This can identify mistyped commands in scripts before you read them. It cannot
check for semantic errors (e.g. killing a non-existent VM). Stops on the first
invalid command.`,
		Patterns: []string{
			"validate <file>",
		},
		Call: wrapSimpleCLI(cliValidate),
	},
	{ // debug
		HelpShort: "display internal debug information",
		Patterns: []string{
			"debug",
			"debug <memory,> <file>",
			"debug <cpu,> <start,> <file>",
			"debug <cpu,> <stop,>",
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
		HelpShort: "display input text after comment removal",
		Patterns: []string{
			"echo [args]...",
		},
		Call: wrapSimpleCLI(cliEcho),
	},
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

	// HACK: We *don't* want long-running read commands to cause all other
	// commands to block so we *unlock* the command lock here and *lock* it
	// again for each command that we read (well, `runCommand` handles the
	// locking for us).
	cmdLock.Unlock()
	defer cmdLock.Lock()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		var cmd *minicli.Command

		command := scanner.Text()
		log.Debug("read command: %v", command)

		cmd, err = minicli.Compile(command)
		if err != nil {
			break
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		cmd.SetSource(SourceRead)

		// HAX: Make sure we don't have a recursive read command
		if hasCommand(cmd, "read") {
			err = errors.New("cannot run nested `read` commands")
			break
		}

		for resp := range runCommand(cmd) {
			respChan <- resp

			// Stop processing if any of the responses have an error.
			for _, r := range resp {
				if r.Error != "" {
					break
				}
			}
		}
	}

	if err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
	}

	if err := scanner.Err(); err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
	}
}

func cliValidate(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	file, err := os.Open(c.StringArgs["file"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		command := scanner.Text()
		log.Debug("read command: %v", command)

		if _, err := minicli.Compile(command); err != nil {
			resp.Error = err.Error()
			break
		}
	}

	if err := scanner.Err(); err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliDebug(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["memory"] {
		dst := c.StringArgs["file"]
		if !filepath.IsAbs(dst) {
			dst = path.Join(*f_iomBase, dst)
		}

		log.Info("writing memory profile to %v", dst)

		f, err := os.Create(dst)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		defer f.Close()

		pprof.WriteHeapProfile(f)

		return resp
	} else if c.BoolArgs["cpu"] && c.BoolArgs["start"] {
		if cpuProfileOut != nil {
			resp.Error = "old CPU profile still running"
			return resp
		}

		dst := c.StringArgs["file"]
		if !filepath.IsAbs(dst) {
			dst = path.Join(*f_iomBase, dst)
		}

		log.Info("writing cpu profile to %v", dst)

		f, err := os.Create(dst)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		cpuProfileOut = f

		pprof.StartCPUProfile(cpuProfileOut)

		return resp
	} else if c.BoolArgs["cpu"] && c.BoolArgs["stop"] {
		if cpuProfileOut == nil {
			resp.Error = "CPU profile is not running"
			return resp
		}

		pprof.StopCPUProfile()
		cpuProfileOut.Close()

		cpuProfileOut = nil

		return resp
	}

	// Otherwise, return information about the runtime environment
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
