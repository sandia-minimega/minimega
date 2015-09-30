// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"strings"
)

var namespaceCLIHandlers = []minicli.Handler{
	{ // namespace
		HelpShort: "control namespace environments",
		HelpLong: `
Control and run commands in namespace environments.`,
		Patterns: []string{
			"namespace [name]",
			"namespace <name> (command)",
		},
		Call: cliNamespace,
	},
	{ // clear namespace
		HelpShort: "unset namespace",
		HelpLong: `
Without a namespace, clear namespace unsets the current namespace.

With a namespace, clear namespace deletes the specified namespace.`,
		Patterns: []string{
			"clear namespace [name]",
		},
		Call: wrapSimpleCLI(cliClearNamespace),
	},
}

func init() {
	registerHandlers("namespace", namespaceCLIHandlers)
}

type queuedVM struct {
	VMConfig // embed

	names  []string
	vmType VMType
}

type Namespace struct {
	Hosts []string

	vmIDChan chan int

	// Queued VMs to launch,
	queuedVMs []queuedVM

	// Status of launching things
}

var namespace string
var namespaces map[string]*Namespace

func init() {
	namespaces = map[string]*Namespace{}
}

// VMs retrieves all the VMs across a namespace. Note that the keys for the
// returned map are arbitrary -- multiple VMs may share the same ID if they are
// on separate hosts so we cannot key off of ID. Note: this assumes that the
// caller has the cmdLock.
func (n Namespace) VMs() VMs {
	res := VMs{}

	cmd := minicli.MustCompile(`vm info`)
	cmd.Record = false

	for resps := range processCommands(makeCommandHosts(n.Hosts, cmd)...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if vms, ok := resp.Data.(VMs); ok {
				for _, vm := range vms {
					res[len(res)] = vm
				}
			} else {
				log.Error("unknown data field in `vm info`")
			}
		}
	}

	return res
}

func cliNamespace(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	if name, ok := c.StringArgs["name"]; ok {
		if _, ok := namespaces[name]; !ok && name != "" {
			log.Info("creating new namespace -- %v", name)

			// By default, every reachable node is part of the namespace
			namespaces[name] = &Namespace{
				Hosts:    append(meshageNode.BroadcastRecipients(), hostname),
				vmIDChan: makeIDChan(),
			}
		}

		if c.Subcommand != nil {
			// Setting namespace for a single command, revert back afterwards
			defer func(old string) {
				namespace = old
			}(namespace)
			namespace = name

			// Run the subcommand and forward the responses
			for resp := range minicli.ProcessCommand(c.Subcommand) {
				respChan <- resp
			}
			return
		}

		// Setting namespace for future commands
		namespace = name
		respChan <- minicli.Responses{resp}
		return
	}

	if namespace == "" {
		names := []string{}
		for name := range namespaces {
			names = append(names, name)
		}

		resp.Response = fmt.Sprintf("Namespaces: %v", names)
	} else {
		// TODO: Dump the queued VMs
		ns := namespaces[namespace]
		resp.Response = fmt.Sprintf("Namespace: `%v`\nHosts: %v", namespace, ns.Hosts)
	}

	respChan <- minicli.Responses{resp}
}

func cliClearNamespace(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if name, ok := c.StringArgs["name"]; ok {
		// Trying to delete a namespace
		if _, ok := namespaces[name]; !ok {
			resp.Error = fmt.Sprintf("unknown namespace `%v`", name)
		} else {
			// If we're deleting the currently active namespace, we should get
			// out of that namespace
			if namespace == name {
				namespace = ""
			}

			delete(namespaces, name)
		}

		return resp
	}

	// Clearing the namespace global
	namespace = ""
	return resp
}

func namespaceQueue(c *minicli.Command, resp *minicli.Response) {
	ns := namespaces[namespace]

	names, err := expandVMLaunchNames(c.StringArgs["name"], ns.VMs())
	if err != nil {
		resp.Error = err.Error()
		return
	}

	// Create a map so that we can look up existence in constant time
	namesMap := map[string]bool{}
	for _, name := range names {
		namesMap[name] = true
	}
	delete(namesMap, "") // delete unconfigured name

	// Extra check for name collisions -- look in the already queued VMs
	for _, queued := range ns.queuedVMs {
		for _, name := range queued.names {
			if namesMap[name] {
				resp.Error = fmt.Sprintf("vm already queued with name `%s`", name)
				return
			}
		}
	}

	vmType, err := findVMType(c.BoolArgs)
	if err != nil {
		resp.Error = err.Error()
		return
	}

	ns.queuedVMs = append(ns.queuedVMs, queuedVM{
		VMConfig: *vmConfig.Copy(),
		names:    names,
		vmType:   vmType,
	})
}

func namespaceLaunch(c *minicli.Command, resp *minicli.Response) {
	ns := namespaces[namespace]

	// Create the host -> VMs assignment
	assignment := schedule(namespace)

	// Clear namespace so subcommands don't use -- revert afterwards
	defer func(old string) {
		namespace = old
	}(namespace)
	namespace = ""

	// Result of vm launch commands
	respChan := make(chan minicli.Responses)
	defer close(respChan)

	// Collect all the responses and log them
	go func() {
		for resps := range respChan {
			for _, resp := range resps {
				if resp.Error != "" {
					log.Error("launch error, host %v -- %v", resp.Host, resp.Error)
				} else if resp.Response != "" {
					log.Debug("launch response, host %v -- %v", resp.Host, resp.Response)
				}
			}
		}
	}()

	for host, queuedVMs := range assignment {
		namespaceHostLaunch(host, queuedVMs, respChan)
	}

	// Clear the queuedVMs -- we just launched them (hopefully!)
	ns.queuedVMs = nil
}

func namespaceHostLaunch(host string, queuedVMs []queuedVM, respChan chan minicli.Responses) {
	for _, queued := range queuedVMs {
		// Mesh send all the config commands
		cmds := []string{"clear vm config"}
		cmds = append(cmds, saveConfig(baseConfigFns, &queued.BaseConfig)...)

		switch queued.vmType {
		case KVM:
			cmds = append(cmds, saveConfig(kvmConfigFns, &queued.KVMConfig)...)
		case CONTAINER:
			cmds = append(cmds, saveConfig(containerConfigFns, &queued.ContainerConfig)...)
		default:
		}

		// Channel for all the `vm config ...` responses
		configChan := make(chan minicli.Responses)

		go func() {
			defer close(configChan)

			for _, cmd := range cmds {
				cmd := minicli.MustCompile(cmd)

				if host == hostname {
					forward(processCommands(cmd), configChan)
				} else {
					meshageSend(cmd, host, configChan)
				}
			}
		}()

		var abort bool

		for resps := range configChan {
			for _, resp := range resps {
				if resp.Error != "" {
					log.Error("config error, host %v -- %v", resp.Host, resp.Error)
					abort = true
				}
			}
		}

		// Send the launch command
		if !abort {
			names := strings.Join(queued.names, ",")
			log.Debug("launch vms on host %v -- %v", host, names)
			cmd := minicli.MustCompilef("vm launch %v %v noblock", queued.vmType, names)
			if host == hostname {
				forward(processCommands(cmd), respChan)
			} else {
				meshageSend(cmd, host, respChan)
			}
		}
	}
}

// wrapVMTargetCLI is a namespace-aware wrapper for VM commands that take a
// single argument -- the VM target. This is used by commands like `vm start`
// and `vm kill`.
func wrapVMTargetCLI(fn func(string) []error) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan minicli.Responses) {
		// No namespace specified, just invoke the handler
		if namespace == "" {
			resp := &minicli.Response{Host: hostname}

			errs := fn(c.StringArgs["target"])
			if len(errs) > 0 {
				resp.Error = errSlice(errs).String()
			}

			respChan <- minicli.Responses{resp}
			return
		}

		hosts := namespaces[namespace].Hosts

		// Clear namespace so subcommands don't use -- revert afterwards
		defer func(old string) {
			namespace = old
		}(namespace)
		namespace = ""

		res := minicli.Responses{}

		var ok bool

		// Broadcast to all machines, collecting errors and forwarding
		// successful commands.
		for resps := range processCommands(makeCommandHosts(hosts, c)...) {
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

// wrapBroadcastCLI is a namespace-aware wrapper for VM commands that
// broadcasts the command to all hosts in the namespace and collects all the
// responses together.
func wrapBroadcastCLI(fn func(*minicli.Command) *minicli.Response) minicli.CLIFunc {
	return func(c *minicli.Command, respChan chan minicli.Responses) {
		// No namespace specified, just invoke the handler
		if namespace == "" {
			respChan <- minicli.Responses{fn(c)}
			return
		}

		hosts := namespaces[namespace].Hosts

		// Clear namespace so subcommands don't use -- revert afterwards
		defer func(old string) {
			namespace = old
		}(namespace)
		namespace = ""

		res := minicli.Responses{}

		// Broadcast to all machines, collecting errors and forwarding
		// successful commands.
		for resps := range processCommands(makeCommandHosts(hosts, c)...) {
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
