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
	"minipager"
	"os"
	"strings"
	"sync"
)

const (
	COMMAND_TIMEOUT = 10
)

var (
	// Prevents multiple commands from running at the same time
	cmdLock sync.Mutex
)

// cliSetup registers all the minimega handlers
func cliSetup() {
	registerHandlers("bridge", bridgeCLIHandlers)
	registerHandlers("capture", captureCLIHandlers)
	registerHandlers("cc", ccCLIHandlers)
	registerHandlers("deploy", deployCLIHandlers)
	registerHandlers("disk", diskCLIHandlers)
	registerHandlers("dnsmasq", dnsmasqCLIHandlers)
	registerHandlers("dot", dotCLIHandlers)
	registerHandlers("external", externalCLIHandlers)
	registerHandlers("history", historyCLIHandlers)
	registerHandlers("host", hostCLIHandlers)
	registerHandlers("io", ioCLIHandlers)
	registerHandlers("log", logCLIHandlers)
	registerHandlers("meshage", meshageCLIHandlers)
	registerHandlers("misc", miscCLIHandlers)
	registerHandlers("nuke", nukeCLIHandlers)
	registerHandlers("optimize", optimizeCLIHandlers)
	registerHandlers("shell", shellCLIHandlers)
	registerHandlers("vm", vmCLIHandlers)
	registerHandlers("vnc", vncCLIHandlers)
	registerHandlers("vyatta", vyattaCLIHandlers)
	registerHandlers("web", webCLIHandlers)
}

// forward receives minicli.Responses from in and forwards them to out.
func forward(in, out chan minicli.Responses) {
	for v := range in {
		out <- v
	}
}

// processCommands wraps minicli.ProcessCommand for multiple commands,
// combining their outputs into a single channel. This function does not
// acquire the cmdLock so it should only be called by functions that do.
func processCommands(cmd ...*minicli.Command) chan minicli.Responses {
	// Forward the responses and unlock when all are passed through
	out := make(chan minicli.Responses)

	var wg sync.WaitGroup

	// Preprocess all the commands so that if there's an error, we haven't
	// already started to run some of the commands.
	for i := range cmd {
		var err error

		cmd[i], err = cliPreprocessor(cmd[i])
		if err != nil {
			log.Errorln(err)

			// Send the error from a separate goroutine because nothing will
			// receive from this channel until processCommands returns and we
			// don't want to create a deadlock.
			go func() {
				out <- minicli.Responses{
					&minicli.Response{
						Host:  hostname,
						Error: err.Error(),
					},
				}

				close(out)
			}()

			return out
		}
	}

	// Completed preprocessing... start all the commands and forward all
	// responses to out.
	for _, c := range cmd {
		wg.Add(1)

		in := minicli.ProcessCommand(c)

		go func() {
			// Mark done after we have read all the responses from in
			defer wg.Done()

			forward(in, out)
		}()
	}

	// Wait for all the de-muxing goroutines to complete
	go func() {
		defer close(out)

		wg.Wait()
	}()

	return out
}

// runCommand wraps processCommands, ensuring that the command execution lock
// is acquired before running the command.
func runCommand(cmd ...*minicli.Command) chan minicli.Responses {
	cmdLock.Lock()

	out := make(chan minicli.Responses)
	go func() {
		// Unlock and close the channel after forwarding all the responses
		defer cmdLock.Unlock()
		defer close(out)

		forward(processCommands(cmd...), out)
	}()

	return out
}

// runCommandGlobally runs the given command across all nodes on meshage,
// including the local node and combines the results into a single channel.
func runCommandGlobally(cmd *minicli.Command) chan minicli.Responses {
	// Keep the original CLI input
	original := cmd.Original
	record := cmd.Record

	cmd, err := minicli.Compilef("mesh send %s .record %t %s", Wildcard, record, original)
	if err != nil {
		log.Fatal("cannot run `%v` globally -- %v", original, err)
	}
	cmd.Record = record

	return runCommand(cmd, cmd.Subcommand)
}

// runCommandHosts runs the given command on a set of hosts.
func runCommandHosts(hosts []string, cmd *minicli.Command) chan minicli.Responses {
	return runCommand(makeCommandHosts(hosts, cmd)...)
}

// makeCommandHosts creates commands to run the given command on a set of hosts
// handling the case where the local node is included in the list.
func makeCommandHosts(hosts []string, cmd *minicli.Command) []*minicli.Command {
	// filter out local node, if included
	var includeLocal bool
	var hosts2 []string

	for _, host := range hosts {
		if host == hostname {
			includeLocal = true
		} else {
			// Quote the hostname in case there are spaces
			hosts2 = append(hosts2, fmt.Sprintf("%q", host))
		}
	}

	targets := strings.Join(hosts2, ",")

	var cmds = []*minicli.Command{}

	if includeLocal {
		// Copy the command
		copied := new(minicli.Command)
		*copied = *cmd

		cmds = append(cmds, copied)
	}

	if len(hosts2) > 0 {
		// Keep the original CLI input
		original := cmd.Original
		record := cmd.Record

		cmd, err := minicli.Compilef("mesh send %s .record %t %s", targets, record, original)
		if err != nil {
			log.Fatal("cannot run `%v` on hosts -- %v", original, err)
		}
		cmd.Record = record

		cmds = append(cmds, cmd)
	}

	return cmds
}

// local command line interface, wrapping readline
func cliLocal() {
	goreadline.FilenameCompleter = iomCompleter

	prompt := "minimega$ "

	for {
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			break // EOF
		}
		command := string(line)
		log.Debug("got from stdin: `%v`", command)

		cmd, err := minicli.Compile(command)
		if err != nil {
			log.Error("%v", err)
			//fmt.Printf("closest match: TODO\n")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		// HAX: Don't record the read command
		if hasCommand(cmd, "read") {
			cmd.Record = false
		}

		for resp := range runCommand(cmd) {
			// print the responses
			minipager.DefaultPager.Page(resp.String())

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}

// cliPreprocessor allows modifying commands post-compile but pre-process.
// Currently the only preprocessor is the "file:" handler.
func cliPreprocessor(c *minicli.Command) (*minicli.Command, error) {
	return iomPreprocessor(c)
}
