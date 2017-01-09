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
	"strings"
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
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "name" {
				return cliNamespaceSuggest(prefix, false)
			}
			return nil
		}),
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
		HelpShort: "unset or delete namespace",
		HelpLong: `
If a namespace is active, "clear namespace" will deactivate it. If no namespace
is active, "clear namespace" returns an error and does nothing.

If you specify a namespace by name, then the specified namespace will be
deleted. You may use "all" to delete all namespaces.`,
		Patterns: []string{
			"clear namespace [name]",
		},
		Call: wrapSimpleCLI(cliClearNamespace),
		Suggest: wrapSuggest(func(val, prefix string) []string {
			if val == "name" {
				return cliNamespaceSuggest(prefix, true)
			}
			return nil
		}),
	},
}

func cliNamespace(c *minicli.Command, respChan chan<- minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	// Get the active namespace
	ns := GetNamespace()

	if name, ok := c.StringArgs["name"]; ok {
		ns2 := GetOrCreateNamespace(name)

		if c.Subcommand != nil {
			// Setting namespace for a single command, revert back afterwards
			defer RevertNamespace(ns, ns2)
			if err := SetNamespace(name); err != nil {
				resp.Error = err.Error()
				respChan <- minicli.Responses{resp}
				return
			}

			// Run the subcommand and forward the responses.
			//
			// LOCK: This is a CLI so we already hold cmdLock (can call
			// runCommands instead of RunCommands).
			forward(runCommands(c.Subcommand), respChan)
			return
		}

		// Setting namespace for future commands
		if err := SetNamespace(name); err != nil {
			resp.Error = err.Error()
		}
		respChan <- minicli.Responses{resp}
		return
	}

	if ns == nil {
		resp.Response = fmt.Sprintf("Namespaces: %v", ListNamespaces())
		respChan <- minicli.Responses{resp}
		return
	}

	hosts := []string{}
	for h := range ns.Hosts {
		hosts = append(hosts, h)
	}

	// TODO: Make this pretty or divide it up somehow
	resp.Response = fmt.Sprintf(`Namespace: "%v"
Hosts: %v
Taps: %v
Number of queuedVMs: %v

Schedules:
`, namespace, ranges.UnsplitList(hosts), ns.Taps, len(ns.queue))

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
			// Add all the peers if we see a wildcard
			if hosts[i] == Wildcard {
				for peer := range peers {
					ns.Hosts[peer] = true
				}

				return nil
			}

			// Resolve `localhost` to actual hostname
			if hosts[i] == Localhost {
				hosts[i] = hostname
			}

			// Otherwise, ensure that the peer is in the mesh
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
			if host == Wildcard {
				ns.Hosts = map[string]bool{}
				break
			}

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
		return SetNamespace("")
	}

	return DestroyNamespace(name)
}

// cliNamespaceSuggest suggests namespaces that have the given prefix. If wild
// is true, Wildcard is included in the list of suggestions.
func cliNamespaceSuggest(prefix string, wild bool) []string {
	res := []string{}

	if wild && strings.HasPrefix(Wildcard, prefix) {
		res = append(res, Wildcard)
	}

	for _, name := range ListNamespaces() {
		if strings.HasPrefix(name, prefix) {
			res = append(res, name)
		}
	}

	return res
}
