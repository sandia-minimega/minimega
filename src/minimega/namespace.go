// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
)

var namespaceCLIHandlers = []minicli.Handler{
	{ // namespace
		HelpShort: "control namespace environments",
		HelpLong: `
Control namespace environments.`,
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

type Namespace struct {
	Hosts []string

	// Queued VMs to launch
	// Status of launching things
}

var namespace string
var namespaces map[string]Namespace

func init() {
	namespaces = map[string]Namespace{}

}

// VMs retrieves all the VMs across a namespace. Note that the keys for the
// returned map are arbitrary -- multiple VMs may share the same ID if they are
// on separate hosts so we cannot key off of ID.
func (n Namespace) VMs() VMs {
	var res VMs

	cmd := minicli.MustCompile(`vm info`)
	cmd.Record = false

	for resps := range runCommandHosts(n.Hosts, cmd) {
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
			namespaces[name] = Namespace{
				Hosts: append(meshageNode.BroadcastRecipients(), hostname),
			}
		}

		if c.Subcommand == nil {
			// Setting namespace for future commands
			namespace = name
			respChan <- minicli.Responses{resp}
			return
		} else {
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
	}

	if namespace == "" {
		names := []string{}
		for name := range namespaces {
			names = append(names, name)
		}

		resp.Response = fmt.Sprintf("Namespaces: %v", names)
	} else {
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
