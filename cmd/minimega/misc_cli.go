// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/version"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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
stop if a command returns an error. Nested reads are not permitted.

Because reading and executing long files can take a while, the read command
releases the command lock that it holds so commands from other clients
(including miniweb) can be interleaved. To prevent issues with another script
changing the namespace and commands being run in a different namespace than
originally intended, read records the active namespace when it starts and
prepends that namespace to all commands that it reads from the file. If it
reads a command that would change the active namespace, read updates its state
so that the new namespace is prepended instead.

If the optional argument check is specified then read doesn't execute any of
the commands in the file. Instead, it checks that all the commands are
syntactically valid. This can identify mistyped commands in scripts before you
read them. It cannot check for semantic errors (e.g. killing a non-existent
VM). The check stops at the first invalid command.`,
		Patterns: []string{
			"read <file> [check,]",
		},
		Call: cliRead,
	},
	{ // debug
		HelpShort: "display internal debug information",
		HelpLong: `
debug can help find and resolve issues with minimega. Without arguments, debug
prints the go version, the number of goroutines, and the number of cgo calls.

With arguments, debug writes files that can be read using "go tool pprof":

- memory: sampling of all heap allocations
- cpu: starts CPU profiling (must be stopped before read)
- goroutine: stack traces of all current goroutines`,
		Patterns: []string{
			"debug",
			"debug <memory,> <file>",
			"debug <cpu,> <start,> <file>",
			"debug <cpu,> <stop,>",
			"debug <goroutine,> <file>",
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
		Call: wrapSimpleCLI(cliClearAll),
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
	return unreachable()
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

	ns := GetNamespace()

	resp := &minicli.Response{Host: hostname}

	fname := c.StringArgs["file"]
	check := c.BoolArgs["check"]

	file, err := os.Open(fname)
	if err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
		return
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	// line number
	var line int

	for scanner.Scan() {
		line += 1
		var cmd *minicli.Command

		command := scanner.Text()
		log.Debug("read command: %v", command)

		cmd, err = minicli.Compile(command)
		if err != nil {
			break
		}

		// Must have been a blank line. Don't try to run.
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

		// HAX: check to see if the command that we're about to run changes the
		// namespace. If it does, we need to adjust the namespace that we
		// prepend to all commands.
		var namespace string

		if !cmd.Nop {
			for cmd := cmd; cmd != nil; cmd = cmd.Subcommand {
				// found command to change namespace
				if strings.HasPrefix(cmd.Original, "namespace") && cmd.Subcommand == nil {
					namespace = cmd.StringArgs["name"]
				}
			}

			if namespace == "" {
				// no change in namespace so recompile the command to execute in
				// the original namespace
				cmd = minicli.MustCompilef("namespace %q %v", ns.Name, command)
			}
		}

		forward(runCommands(cmd), respChan)

		if namespace != "" {
			log.Info("read switching to namespace `%v`", namespace)

			// update the namespace that we prepend to match the newly
			// activated namespace
			ns = GetOrCreateNamespace(namespace)
		}
	}

	if err != nil {
		resp.Error = fmt.Sprintf("%v:%v %v", filepath.Base(fname), line, err)
		respChan <- minicli.Responses{resp}
	}

	if err := scanner.Err(); err != nil {
		resp.Error = err.Error()
		respChan <- minicli.Responses{resp}
	}
}

func cliDebug(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	dst := c.StringArgs["file"]

	var f *os.File
	if dst != "" {
		// make sure path is relative to files if not absolute
		if !filepath.IsAbs(dst) {
			dst = path.Join(*f_iomBase, dst)
		}

		log.Info("writing debug info to %v", dst)

		var err error
		if f, err = os.Create(dst); err != nil {
			return err
		}
	}

	if c.BoolArgs["memory"] {
		defer f.Close()

		return pprof.Lookup("heap").WriteTo(f, 0)
	} else if c.BoolArgs["goroutine"] {
		defer f.Close()

		return pprof.Lookup("goroutine").WriteTo(f, 2)
	} else if c.BoolArgs["cpu"] && c.BoolArgs["start"] {
		if cpuProfileOut != nil {
			return errors.New("CPU profile still running")
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

func cliClearAll(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	all := []string{
		// clear non-namespaced things (except history)
		"clear deploy flags",
		"dnsmasq kill all",
		// clear all namespaced things
		"clear namespace all",
		// clear vlan blacklist
		"clear vlans all",
		// clear plumbing and pipes
		"clear plumb",
		"clear pipe",
		// clear the history last
		"clear history",
	}

	var cmds []*minicli.Command

	for _, v := range all {
		cmd := minicli.MustCompile(v)
		// keep the original source
		cmd.SetSource(c.Source)
		cmds = append(cmds, cmd)
	}

	return consume(runCommands(cmds...))
}
