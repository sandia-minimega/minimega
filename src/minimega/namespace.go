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

func cliNamespace(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	if name, ok := c.StringArgs["name"]; ok {
		if _, ok := namespaces[name]; !ok {
			log.Info("creating new namespace -- %v", name)

			namespaces[name] = Namespace{}
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

	names := []string{}
	for name := range namespaces {
		names = append(names, name)
	}

	resp.Response = fmt.Sprintf("Current: `%v` -- Known: %v", namespace, names)
	respChan <- minicli.Responses{resp}
}
