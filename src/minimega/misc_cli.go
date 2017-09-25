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
the file in manually. read stops if it reads an invalid command. read does not
stop if a command returns an error.

If the optional argument check is specified then read doesn't execute any of
the commands in the file. Instead, it checks that all the commands are
syntactically valid. This can identify mistyped commands in scripts before you
read them. It cannot check for semantic errors (e.g. killing a non-existent
VM). Stops on the first invalid command.`,
		Patterns: []string{
			"read <file> [check,]",
		},
		Call: cliRead,
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
		Call: wrapBroadcastCLI(cliVersion),
	},
	{ // echo
		HelpShort: "display input text after comment removal",
		Patterns: []string{
			"echo [args]...",
		},
		Call: wrapSimpleCLI(cliEcho),
	},
	{ // clear all
		HelpShort: "reset all resettable ",
		HelpLong: `
Runs all the "clear ..." handlers on the local instance -- as close to nuke as
you can get without restarting minimega. Restarting minimega is preferable.`,
		Patterns: []string{
			"clear all",
		},
		Call: cliClearAll,
	},
}

func cliQuit(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if v, ok := c.StringArgs["delay"]; ok {
		delay, err := strconv.Atoi(v)
		if err != nil {
			return err
		}

		go func() {
			time.Sleep(time.Duration(delay) * time.Second)
			Shutdown("quitting")
		}()

		resp.Response = fmt.Sprintf("quitting after %v seconds", delay)
		return nil
	}

	Shutdown("quitting")
	return errors.New("unreachable")
}

func cliHelp(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	input := ""
	if args, ok := c.ListArgs["command"]; ok {
		input = strings.Join(args, " ")
	}

	resp.Response = minicli.Help(input)
	return nil
}

func cliRead(c *minicli.Command, respChan chan<- minicli.Responses) {
	// HAX: prevent running as a subcommand
	if c.Source == SourceMeshage {
		err := fmt.Errorf("cannot run `%s` via meshage", c.Original)
		respChan <- errResp(err)
		return
	}

	resp := &minicli.Response{Host: hostname}

	check := c.BoolArgs["check"]

	file, err := os.Open(c.StringArgs["file"])
	if err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
		return
	}
	defer file.Close()

	// HACK: We *don't* want long-running read commands to cause all other
	// commands to block so we *unlock* the command lock here and *lock* it
	// again for each command that we read (well, `RunCommands` handles the
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

		// HAX: Make sure we don't have a recursive read command
		if hasCommand(cmd, "read") {
			err = errors.New("cannot run nested `read` commands")
			break
		}

		// If we're checking the syntax, don't actually execute the command
		if check {
			continue
		}

		forward(RunCommands(cmd), respChan)
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

func cliDebug(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// make sure path is relative to files if not absolute
	dst := c.StringArgs["file"]
	if !filepath.IsAbs(dst) {
		dst = path.Join(*f_iomBase, dst)
	}

	if c.BoolArgs["memory"] {
		log.Info("writing memory profile to %v", dst)

		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer f.Close()

		return pprof.WriteHeapProfile(f)
	} else if c.BoolArgs["cpu"] && c.BoolArgs["start"] {
		if cpuProfileOut != nil {
			return errors.New("CPU profile still running")
		}

		log.Info("writing cpu profile to %v", dst)

		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		cpuProfileOut = f

		return pprof.StartCPUProfile(cpuProfileOut)
	} else if c.BoolArgs["cpu"] && c.BoolArgs["stop"] {
		if cpuProfileOut == nil {
			return errors.New("CPU profile not running")
		}

		pprof.StopCPUProfile()
		if err := cpuProfileOut.Close(); err != nil {
			return err
		}

		cpuProfileOut = nil
		return nil
	}

	// Otherwise, return information about the runtime environment
	resp.Header = []string{"goversion", "goroutines", "cgocalls"}
	resp.Tabular = [][]string{
		[]string{
			runtime.Version(),
			strconv.Itoa(runtime.NumGoroutine()),
			strconv.FormatInt(runtime.NumCgoCall(), 10),
		},
	}

	return nil
}

func cliVersion(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Response = fmt.Sprintf("minimega %v %v", version.Revision, version.Date)
	return nil
}

func cliEcho(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Response = strings.Join(c.ListArgs["args"], " ")
	return nil
}

func cliClearAll(c *minicli.Command, respChan chan<- minicli.Responses) {
	all := []string{
		// clear non-namespaced things (except history)
		"clear deploy flags",
		"dnsmasq kill all",
		// clear all namespaced things
		"clear namespace all",
		// clear vlan blacklist
		"clear vlans all",
		// clear the history last
		"clear history",
	}

	// LOCK: this is a CLI hander so we already hold cmdLock (can call
	// runCommands instead of RunCommands).
	for _, v := range all {
		cmd := minicli.MustCompile(v)
		// keep the original source
		cmd.SetSource(c.Source)

		forward(runCommands(cmd), respChan)
	}
}
