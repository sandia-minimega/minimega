// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"minicli"
	"ranges"
	"text/tabwriter"
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
	{ // nsmod
		HelpShort: "modify namespace environments",
		HelpLong: `
Modify settings of the currently active namespace.

add-host - add comma-separated list of hosts to the namespace.
del-host - delete comma-separated list of hosts from the namespace.
`,
		Patterns: []string{
			"nsmod <add-host,> <hosts>",
			"nsmod <del-host,> <hosts>",
		},
		Call: wrapSimpleCLI(cliNamespaceMod),
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

func cliNamespace(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	// Get the active namespace
	ns := GetNamespace()

	if name, ok := c.StringArgs["name"]; ok {
		ns2 := GetOrCreateNamespace(name)

		if c.Subcommand != nil {
			// Setting namespace for a single command, revert back afterwards
			defer RevertNamespace(ns, ns2)
			SetNamespace(name)

			// Run the subcommand and forward the responses
			forward(processCommands(c.Subcommand), respChan)
			return
		}

		// Setting namespace for future commands
		SetNamespace(name)
		respChan <- minicli.Responses{resp}
		return
	}

	if ns == nil {
		resp.Response = fmt.Sprintf("Namespaces: %v", ListNamespaces())
		respChan <- minicli.Responses{resp}
		return
	}

	// TODO: Make this pretty or divide it up somehow
	resp.Response = fmt.Sprintf(`Namespace: "%v"
Hosts: %v
Number of queuedVMs: %v

Schedules:
`, namespace, ns.Hosts, len(ns.queuedVMs))

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(w, "start\tend\tstate\tlaunched\tfailures\ttotal\thosts")
	for _, stats := range ns.scheduleStats {
		var end string
		if !stats.end.IsZero() {
			end = fmt.Sprintf("%v", stats.end)
		}

		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\n",
			stats.start,
			end,
			stats.state,
			stats.launched,
			stats.failures,
			stats.total,
			stats.hosts)
	}
	w.Flush()

	resp.Response += o.String()

	respChan <- minicli.Responses{resp}
}

func cliNamespaceMod(c *minicli.Command, resp *minicli.Response) error {
	ns := GetNamespace()
	if ns == nil {
		return errors.New("cannot run nsmod without active namespace")
	}

	// Empty string should parse fine...
	hosts, err := ranges.SplitList(c.StringArgs["hosts"])
	if err != nil {
		return fmt.Errorf("invalid hosts -- %v", err)
	}

	if c.BoolArgs["add-host"] {
		peers := map[string]bool{}
		for _, peer := range meshageNode.BroadcastRecipients() {
			peers[peer] = true
		}

		// Test that the host is actually in the mesh. If it's not, we could
		// try to mesh dial it... Returning an error is simpler, for now.
		for i := range hosts {
			// Resolve localhost
			if hosts[i] == Localhost {
				hosts[i] = hostname
			}

			if hosts[i] != hostname && !peers[hosts[i]] {
				return fmt.Errorf("unknown host: `%v`", hosts[i])
			}
		}

		// After all have been checked, updated the namespace
		for _, host := range hosts {
			ns.Hosts[host] = true
		}

		return nil
	} else if c.BoolArgs["del-host"] {
		for _, host := range hosts {
			delete(ns.Hosts, host)
		}

		return nil
	}

	// boo, should be unreachable
	return errors.New("unreachable")
}

func cliClearNamespace(c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["name"]
	if name == "" {
		// Clearing the namespace global
		SetNamespace("")
		return nil
	}

	return DestroyNamespace(name)
}
