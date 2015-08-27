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
	"miniclient"
	log "minilog"
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

type CLIFunc func(*minicli.Command) *minicli.Response

const (
	NamespaceBroadcast = iota // simply broadcast the command to hosts
	NamespaceBroadcastVmTarget
)

// cliSetup registers all the minimega handlers
func cliSetup() {
	registerHandlers("bridge", bridgeCLIHandlers)
	registerHandlers("capture", captureCLIHandlers)
	registerHandlers("cc", ccCLIHandlers)
	registerHandlers("deploy", deployCLIHandlers)
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
	registerHandlers("qcow", qcowCLIHandlers)
	registerHandlers("shell", shellCLIHandlers)
	registerHandlers("vm", vmCLIHandlers)
	registerHandlers("vnc", vncCLIHandlers)
	registerHandlers("vyatta", vyattaCLIHandlers)
	registerHandlers("web", webCLIHandlers)
}

// wrapCLI wraps handlers with some extra handling for namespacing. options
// dictate what generic cases should be applied to the handler.
func wrapCLI(fn CLIFunc, options int) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan minicli.Responses) {
		// No namespace specified, just invoke the handler
		if namespace == "" {
			resp := fn(c)
			respChan <- minicli.Responses{resp}
			return
		}

		hosts := namespaces[namespace].Hosts

		// Clear namespace so subcommands don't use -- revert afterwards
		defer func(old string) {
			namespace = old
		}(namespace)
		namespace = ""

		cmds := makeCommandHosts(hosts, c)

		switch options {
		case NamespaceBroadcast:
			// Simple case, just broadcast and then collect the responses
			forward(processCommands(cmds...), respChan)
		case NamespaceBroadcastVmTarget:
			var ok bool
			var notFound string
			resps := minicli.Responses{}

			// Broadcast to all machines, collecting errors and forwarding
			// successful commands.
			for resp := range processCommands(cmds...) {
				if len(resp) > 1 {
					log.Error("unsure how to process multiple responses!!")
				}

				if resp[0].Error == "" {
					// Keep all responses without an error
					resps = append(resps, resp[0])
					ok = true
				} else if !isVmNotFound(resp[0].Error) {
					// Record errors that aren't not found
					resps = append(resps, resp[0])
				} else {
					// Record not found in case the VM doesn't exist anywhere
					notFound = resp[0].Error
				}
			}

			if !ok && notFound != "" {
				// Didn't find any responses without errors so create a new
				// response with the not found error
				resps = append(resps, &minicli.Response{
					Host:  hostname,
					Error: notFound,
				})
			}

			respChan <- resps
		}
	}
}

// wrapSimpleCLI wraps handlers that return a single response. This greatly
// reduces boilerplate code with minicli handlers.
func wrapSimpleCLI(fn func(*minicli.Command) *minicli.Response) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan minicli.Responses) {
		resp := fn(c)
		respChan <- minicli.Responses{resp}
	}
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
	ins := []chan minicli.Responses{}

	for _, c := range cmd {
		ins = append(ins, minicli.ProcessCommand(c))
	}

	var wg sync.WaitGroup

	// De-mux ins into out
	for _, in := range ins {
		wg.Add(1)

		go func(in chan minicli.Responses) {
			// Mark done after we have read all the responses from in
			defer wg.Done()

			forward(in, out)
		}(in)
	}

	go func() {
		// Close after all de-muxing goroutines have completed
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
		cmds = append(cmds, cmd)
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
	prompt := "minimega$ "

	for {
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			break // EOF
		}
		command := string(line)
		log.Debug("got from stdin:", command)

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
			miniclient.Pager(resp.String())

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}
