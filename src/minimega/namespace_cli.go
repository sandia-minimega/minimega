// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"ranges"
	"sort"
	"strconv"
	"strings"
)

var namespaceCLIHandlers = []minicli.Handler{
	{ // namespace
		HelpShort: "display or change namespace",
		HelpLong: `
With no arguments, "namespace" prints all the namespaces, the active namespace
will be displayed in brackets (e.g. "[minimega]"). When a namespace is
specified, it changes the active namespace or runs a single command in the
different namespace.`,
		Patterns: []string{
			"namespace [name]",
			"namespace <name> (command)",
		},
		Call: cliNamespace,
		Suggest: wrapSuggest(func(_ *Namespace, val, prefix string) []string {
			if val == "name" {
				return cliNamespaceSuggest(prefix, false)
			}
			return nil
		}),
	},
	{ // ns
		HelpShort: "tinker with active namespace",
		HelpLong: `
Display or modify the active namespace.

- hosts     : list hosts
- add-host  : add comma-separated list of hosts to the namespace
- del-host  : delete comma-separated list of hosts from the namespace
- load      : display or change host load is computed for scheduler, based on:
  - cpucommit : total CPU commit divided by number of CPUs (default)
  - netcommit : total NIC
  - memcommit : total memory commit divided by total memory
- queue     : print the VM queue
- queuing   : toggle queueing VMs when launching (default false)
- flush     : clear the VM queue
- schedules : display scheduling stats
`,
		Patterns: []string{
			"ns",
			"ns <hosts,>",
			"ns <add-host,> <hosts>",
			"ns <del-host,> <hosts>",
			"ns <load,>",
			"ns <load,> <cpucommit,>",
			"ns <load,> <netcommit,>",
			"ns <load,> <memcommit,>",
			"ns <queue,>",
			"ns <queueing,> [true,false]",
			"ns <flush,>",
			"ns <schedules,>",
		},
		Call: wrapSimpleCLI(cliNS),
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
		Suggest: wrapSuggest(func(_ *Namespace, val, prefix string) []string {
			if val == "name" {
				return cliNamespaceSuggest(prefix, true)
			}
			return nil
		}),
	},
}

// Functions pointers to the various handlers for the subcommands
var nsCliHandlers = map[string]wrappedCLIFunc{
	"hosts":     cliNamespaceHosts,
	"add-host":  cliNamespaceAddHost,
	"del-host":  cliNamespaceDelHost,
	"load":      cliNamespaceLoad,
	"queue":     cliNamespaceQueue,
	"queueing":  cliNamespaceQueueing,
	"flush":     cliNamespaceFlush,
	"schedules": cliNamespaceSchedules,
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

	other := []string{}
	for k := range namespaces {
		if k == namespace {
			k = "[" + k + "]"
		}
		other = append(other, k)
	}

	sort.Strings(other)

	resp.Response = strings.Join(other, ", ")

	respChan <- minicli.Responses{resp}
}

func cliNS(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Dispatcher for a sub handler
	if len(c.BoolArgs) > 0 {
		for k, fn := range nsCliHandlers {
			if c.BoolArgs[k] {
				log.Debug("ns handler %v", k)
				return fn(ns, c, resp)
			}
		}

		return errors.New("unreachable")
	}

	// If no sub handler, do something
	return nil
}

func cliNamespaceHosts(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Response = ranges.UnsplitList(ns.hostSlice())
	return nil
}

func cliNamespaceAddHost(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	hosts, err := ranges.SplitList(c.StringArgs["hosts"])
	if err != nil {
		return fmt.Errorf("invalid hosts -- %v", err)
	}

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
}

func cliNamespaceDelHost(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	hosts, err := ranges.SplitList(c.StringArgs["hosts"])
	if err != nil {
		return fmt.Errorf("invalid hosts -- %v", err)
	}

	for _, host := range hosts {
		if host == Wildcard {
			ns.Hosts = map[string]bool{}
			break
		}

		delete(ns.Hosts, host)
	}

	return nil
}

func cliNamespaceLoad(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// check if we're updating the sort by func
	for k := range hostSortByFns {
		if c.BoolArgs[k] {
			ns.HostSortBy = k
			return nil
		}
	}

	resp.Response = ns.HostSortBy
	return nil
}

func cliNamespaceQueue(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	var buf bytes.Buffer

	for _, q := range ns.queue {
		var names []string
		for _, n := range q.Names {
			if n != "" {
				names = append(names, n)
			}
		}

		fmt.Fprintf(&buf, "VMs: %v\n", len(q.Names))
		buf.WriteString("Names: ")
		buf.WriteString(ranges.UnsplitList(names))
		buf.WriteString("\n")
		buf.WriteString("VM Type: ")
		buf.WriteString(q.VMType.String())
		buf.WriteString("\n\n")
		buf.WriteString(q.VMConfig.String(ns.Name))
		buf.WriteString("\n\n")
	}

	resp.Response = buf.String()
	return nil
}

func cliNamespaceQueueing(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["true"] || c.BoolArgs["false"] {
		ns.QueueVMs = c.BoolArgs["true"]

		if len(ns.queue) > 0 {
			log.Warn("queueing behavior changed when VMs already queued")
		}
	} else {
		resp.Response = strconv.FormatBool(ns.QueueVMs)
	}

	return nil
}

func cliNamespaceFlush(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	ns.queue = nil
	return nil
}

func cliNamespaceSchedules(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"start", "end", "state", "launched", "failures", "total", "hosts",
	}

	for _, stats := range ns.scheduleStats {
		var end string
		if !stats.end.IsZero() {
			end = fmt.Sprintf("%v", stats.end)
		}

		row := []string{
			stats.start.String(),
			end,
			stats.state,
			strconv.Itoa(stats.launched),
			strconv.Itoa(stats.failures),
			strconv.Itoa(stats.total),
			strconv.Itoa(stats.hosts),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliClearNamespace(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	name := c.StringArgs["name"]
	if name == "" {
		// Clearing the namespace global
		return SetNamespace(DefaultNamespace)
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
