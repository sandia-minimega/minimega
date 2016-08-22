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
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
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

// Sources of minicli.Commands. If minicli.Command.Source is not set, then we
// generated the Command programmatically.
var (
	SourceMeshage   = "meshage"
	SourceLocalCLI  = "local"
	SourceAttachCLI = "attach"
	SourceRead      = "read"
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
	registerHandlers("qos", qosCLIHandlers)
	registerHandlers("router", routerCLIHandlers)
	registerHandlers("shell", shellCLIHandlers)
	registerHandlers("vlans", vlansCLIHandlers)
	registerHandlers("vm", vmCLIHandlers)
	registerHandlers("vm limit", vmLimiterCLIHanders)
	registerHandlers("vmconfig", vmconfigCLIHandlers)
	registerHandlers("vnc", vncCLIHandlers)
	registerHandlers("vyatta", vyattaCLIHandlers)
	registerHandlers("web", webCLIHandlers)
}

// wrapSimpleCLI wraps handlers that return a single response. This greatly
// reduces boilerplate code with minicli handlers.
func wrapSimpleCLI(fn func(*minicli.Command, *minicli.Response) error) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		resp := &minicli.Response{Host: hostname}
		if err := fn(c, resp); err != nil {
			resp.Error = err.Error()
		}
		respChan <- minicli.Responses{resp}
	}
}

// errResp creates a minicli.Responses from a single error.
func errResp(err error) minicli.Responses {
	resp := &minicli.Response{
		Host:  hostname,
		Error: err.Error(),
	}

	return minicli.Responses{resp}
}

// wrapBroadcastCLI is a namespace-aware wrapper for VM commands that
// broadcasts the command to all hosts in the namespace and collects all the
// responses together.
func wrapBroadcastCLI(fn func(*minicli.Command, *minicli.Response) error) minicli.CLIFunc {
	// for the `local` behavior
	localFunc := wrapSimpleCLI(fn)

	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		ns := GetNamespace()

		log.Debug("namespace: %v, command: %#v", ns, c)

		// Wrapped commands have two behaviors:
		//   `fan out` -- send the command to all hosts in the active namespace
		//   `local`   -- invoke the underlying handler
		// We use the source field to track whether we have already performed
		// the `fan out` phase for this command. By default, the source is the
		// empty string, so when a namespace is not active, we will always have
		// the `local` behavior. When a namespace is active, the source will
		// not match the active namespace so we will perform the `fan out`
		// phase. We immediately set the source to the active namespace so that
		// when we send the command via mesh, the source will be propagated and
		// the remote nodes will execute the `local` behavior rather than
		// trying to `fan out`.
		if ns == nil || c.Source == ns.Name {
			localFunc(c, respChan)
			return
		}
		c.SetSource(ns.Name)

		hosts := ns.hostSlice()

		cmds := makeCommandHosts(hosts, c)
		for _, cmd := range cmds {
			cmd.SetRecord(false)
		}

		res := minicli.Responses{}

		// Broadcast to all machines, collecting errors and forwarding
		// successful commands.
		//
		// LOCK: this is a CLI handler so we already hold the cmdLock.
		for resps := range runCommands(cmds...) {
			// TODO: we are flattening commands that return multiple responses
			// by doing this... should we implement proper buffering? Only a
			// problem if commands that return multiple responses are wrapped
			// by this (which *currently* is not the case).
			for _, resp := range resps {
				res = append(res, resp)
			}
		}

		respChan <- res
	}
}

// wrapVMTargetCLI is a namespace-aware wrapper for VM commands that target one
// or more VMs. This is used by commands like `vm start` and `vm kill`.
func wrapVMTargetCLI(fn func(*minicli.Command, *minicli.Response) error) minicli.CLIFunc {
	// for the `local` behavior
	localFunc := wrapSimpleCLI(fn)

	return func(c *minicli.Command, respChan chan<- minicli.Responses) {
		ns := GetNamespace()

		log.Debug("namespace: %v, source: %v", ns, c.Source)

		// See note in wrapBroadcastCLI.
		if ns == nil || c.Source == ns.Name {
			localFunc(c, respChan)
			return
		}
		c.SetSource(ns.Name)

		hosts := ns.hostSlice()

		cmds := makeCommandHosts(hosts, c)
		for _, cmd := range cmds {
			cmd.SetRecord(false)
		}

		res := minicli.Responses{}
		var ok bool

		// Broadcast to all machines, collecting errors and forwarding
		// successful commands.
		//
		// LOCK: this is a CLI handler so we already hold the cmdLock.
		for resps := range runCommands(cmds...) {
			for _, resp := range resps {
				ok = ok || (resp.Error == "")

				if resp.Error == "" || !isVmNotFound(resp.Error) {
					// Record successes and unexpected errors
					res = append(res, resp)
				}
			}
		}

		if !ok && len(res) == 0 {
			// Presumably, we weren't able to find the VM
			res = append(res, &minicli.Response{
				Host:  hostname,
				Error: vmNotFound(c.StringArgs["target"]).Error(),
			})
		}

		respChan <- res
	}
}

// forward receives minicli.Responses from in and forwards them to out.
func forward(in <-chan minicli.Responses, out chan<- minicli.Responses) {
	for v := range in {
		out <- v
	}
}

// runCommands is RunCommands without locking cmdLock.
func runCommands(cmd ...*minicli.Command) <-chan minicli.Responses {
	out := make(chan minicli.Responses)

	// Preprocess all the commands so that if there's an error, we haven't
	// already started to run some of the commands.
	for i := range cmd {
		if err := cliPreprocessor(cmd[i]); err != nil {
			log.Errorln(err)

			// Send the error from a separate goroutine because nothing will
			// receive from this channel until we return. Otherwise, we will
			// cause a deadlock.
			go func() {
				out <- errResp(err)
				close(out)
			}()
			return out
		}
	}

	// Completed preprocessing run commands serially and forward all the
	// responses to out
	go func() {
		defer close(out)

		for _, c := range cmd {
			forward(minicli.ProcessCommand(c), out)
		}
	}()

	return out
}

// RunCommands wraps minicli.ProcessCommand for multiple commands, combining
// their outputs into a single channel.
func RunCommands(cmd ...*minicli.Command) <-chan minicli.Responses {
	cmdLock.Lock()

	out := make(chan minicli.Responses)
	go func() {
		// Unlock and close the channel after forwarding all the responses
		defer cmdLock.Unlock()
		defer close(out)

		forward(runCommands(cmd...), out)
	}()

	return out
}

// runCommandGlobally runs the given command across all nodes on meshage,
// including the local node and combines the results into a single channel.
func runCommandGlobally(cmd *minicli.Command) <-chan minicli.Responses {
	// Keep the original CLI input
	original := cmd.Original
	record := cmd.Record

	cmd, err := minicli.Compilef("mesh send %s %s", Wildcard, original)
	if err != nil {
		log.Fatal("cannot run `%v` globally -- %v", original, err)
	}
	cmd.SetRecord(record)

	return runCommands(cmd, cmd.Subcommand)
}

// makeCommandHosts creates commands to run the given command on a set of hosts
// handling the special case where the local node is included in the list.
// makeCommandHosts is namespace-aware -- it generates commands based on the
// currently active namespace.
func makeCommandHosts(hosts []string, cmd *minicli.Command) []*minicli.Command {
	// filter out the local host, if included
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

	var cmds = []*minicli.Command{}

	if includeLocal {
		// Create a deep copy of the command by recompiling it
		cmd2 := minicli.MustCompile(cmd.Original)
		cmd2.SetRecord(cmd.Record)
		cmd2.SetSource(cmd.Source)

		cmds = append(cmds, cmd2)
	}

	if len(hosts2) > 0 {
		ns := GetNamespace()

		targets := strings.Join(hosts2, ",")

		// Keep the original CLI input
		original := cmd.Original

		// Prefix with namespace, if one is set
		if ns != nil {
			original = fmt.Sprintf("namespace %q %v", ns.Name, original)
		}

		cmd2 := minicli.MustCompilef("mesh send %s %s", targets, original)
		cmd2.SetRecord(cmd.Record)
		cmd2.SetSource(cmd.Source)

		cmds = append(cmds, cmd2)
	}

	return cmds
}

// local command line interface, wrapping readline
func cliLocal() {
	goreadline.FilenameCompleter = iomCompleter

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		for range sig {
			goreadline.Signal()
		}
	}()
	defer signal.Stop(sig)

	for {
		namespace := GetNamespaceName()

		prompt := "minimega$ "
		if namespace != "" {
			prompt = fmt.Sprintf("minimega[%v]$ ", namespace)
		}

		line, err := goreadline.Readline(prompt, true)
		if err != nil {
			return
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
			cmd.SetRecord(false)
		}

		// The namespace changed between when we prompted the user (and could
		// still change before we actually run the command).
		if namespace != GetNamespaceName() {
			// TODO: should we abort the command?
			log.Warn("namespace changed between prompt and execution")
		}

		for resp := range RunCommands(cmd) {
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
// Current preprocessors "file:", "http://", and "http://".
//
// Note: we don't run preprocessors when we're not running the `local` behavior
// (see wrapBroadcastCLI) to avoid expanding files before we're running the
// command on the correct machine.
func cliPreprocessor(c *minicli.Command) error {
	if c.Source != GetNamespaceName() {
		return nil
	}

	helper := func(v string) (string, error) {
		if u, err := url.Parse(v); err == nil {
			switch u.Scheme {
			case "file":
				log.Debug("file preprocessor")
				return iomHelper(u.Opaque)
			case "http", "https":
				log.Debug("http/s preprocessor")

				// Check if we've already downloaded the file
				v2, err := iomHelper(u.Path)
				if err == nil {
					return v2, err
				}

				if err.Error() == "file not found" {
					log.Info("attempting to download %v", u)

					// Try to download the file, save to files
					dst := filepath.Join(*f_iomBase, u.Path)
					if err := wget(v, dst); err != nil {
						return "", err
					}

					return dst, nil
				}

				return "", err
			}
		}

		return v, nil
	}

	for k, v := range c.StringArgs {
		v2, err := helper(v)
		if err != nil {
			return err
		}

		log.Debug("cliPreprocessor: %v -> %v", v, v2)
		c.StringArgs[k] = v2
	}

	for k := range c.ListArgs {
		for k2, v := range c.ListArgs[k] {
			v2, err := helper(v)
			if err != nil {
				return err
			}

			log.Debug("cliPreprocessor: %v -> %v", v, v2)
			c.ListArgs[k][k2] = v2
		}
	}

	return nil
}
