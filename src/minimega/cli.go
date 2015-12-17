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

// Wrapper for minicli.ProcessCommand. Ensures that the command execution lock
// is acquired before running the command.
func runCommand(cmd *minicli.Command) chan minicli.Responses {
	cmdLock.Lock()

	// Forward the responses and unlock when all are passed through
	localChan := make(chan minicli.Responses)
	go func() {
		defer cmdLock.Unlock()

		cmd, err := cliPreprocessor(cmd)
		if err != nil {
			log.Errorln(err)
			localChan <- minicli.Responses{
				&minicli.Response{
					Host:  hostname,
					Error: err.Error(),
				},
			}
			close(localChan)
			return
		}

		for resp := range minicli.ProcessCommand(cmd) {
			localChan <- resp
		}

		close(localChan)
	}()

	return localChan
}

// Wrapper for minicli.ProcessCommand for commands that use meshage.
// Specifically, for `mesh send all ...`, runs the subcommand locally and
// across meshage, combining the results from the two channels into a single
// channel. This is useful if you want to get the output of a command from all
// nodes in the cluster without having to run a command locally and over
// meshage.
func runCommandGlobally(cmd *minicli.Command) chan minicli.Responses {
	// Keep the original CLI input
	original := cmd.Original
	record := cmd.Record

	cmd, err := minicli.Compilef("mesh send %s .record %t %s", Wildcard, record, original)
	if err != nil {
		log.Fatal("cannot run `%v` globally -- %v", original, err)
	}
	cmd.Record = record

	cmdLock.Lock()

	var wg sync.WaitGroup

	out := make(chan minicli.Responses)

	cmd, err = cliPreprocessor(cmd)
	if err != nil {
		log.Errorln(err)
		out <- minicli.Responses{
			&minicli.Response{
				Host:  hostname,
				Error: err.Error(),
			},
		}
		close(out)
		return out
	}

	// Run the command (should be `mesh send all ...` and the subcommand which
	// should run locally).
	ins := []chan minicli.Responses{
		minicli.ProcessCommand(cmd),
		minicli.ProcessCommand(cmd.Subcommand),
	}

	// De-mux ins into out
	for _, in := range ins {
		wg.Add(1)
		go func(in chan minicli.Responses) {
			defer wg.Done()
			for v := range in {
				out <- v
			}
		}(in)
	}

	// Wait until everything has been read before closing the chan and
	// releasing the lock.
	go func() {
		defer cmdLock.Unlock()
		defer close(out)

		wg.Wait()
	}()

	return out
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
