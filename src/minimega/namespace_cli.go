// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"minicli"
	log "minilog"
	"ranges"
	"strconv"
	"strings"
	"time"
)

var namespaceCLIHandlers = []minicli.Handler{
	{ // namespace
		HelpShort: "display or change namespace",
		HelpLong: `
With no arguments, "namespace" prints summary info about namespaces:

- name   : name of the namespace
- vlans  : range of VLANs, empty if not set
- active : active or not

When a namespace is specified, it changes the active namespace or runs a single
command in the different namespace.`,
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
- add-hosts : add comma-separated list of hosts to the namespace
- del-hosts : delete comma-separated list of hosts from the namespace
- load      : display or change host load is computed for scheduler, based on:
  - cpucommit : total CPU commit divided by number of CPUs (default)
  - netcommit : total NIC
  - memcommit : total memory commit divided by total memory
- queue     : display VM queue
- flush     : clear the VM queue
- queueing  : toggle VMs queueing when launching (default false)
- schedules : display scheduling stats
- run       : run a command on all nodes in the namespace
`,
		Patterns: []string{
			"ns <hosts,>",
			"ns <add-hosts,> <hosts>",
			"ns <del-hosts,> <hosts>",
			"ns <load,>",
			"ns <load,> <cpucommit,>",
			"ns <load,> <netcommit,>",
			"ns <load,> <memcommit,>",
			"ns <queue,>",
			"ns <flush,>",
			"ns <queueing,> [true,false]",
			"ns <schedules,>",
			"ns <run,> (command)",
		},
		Call: cliNS,
	},
	{ // clear namespace
		HelpShort: "unset or delete namespace",
		HelpLong: `
Without an argument, "clear namespace" will reset the namespace to the default
namespace, minimega.

With an arugment, "clear namespace <name>" will delete the specified namespace.
You may use "all" to delete all namespaces.`,
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
var nsCliHandlers = map[string]minicli.CLIFunc{
	"hosts":     wrapSimpleCLI(cliNamespaceHosts),
	"add-hosts": wrapSimpleCLI(cliNamespaceAddHost),
	"del-hosts": wrapSimpleCLI(cliNamespaceDelHost),
	"load":      wrapSimpleCLI(cliNamespaceLoad),
	"queue":     wrapSimpleCLI(cliNamespaceQueue),
	"queueing":  wrapSimpleCLI(cliNamespaceQueueing),
	"flush":     wrapSimpleCLI(cliNamespaceFlush),
	"schedules": wrapSimpleCLI(cliNamespaceSchedules),
	"run":       cliNamespaceRun,
}

func cliNamespace(c *minicli.Command, respChan chan<- minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	// Get the active namespace
	ns := GetNamespace()

	if name, ok := c.StringArgs["name"]; ok {
		ns2 := GetOrCreateNamespace(name)

		if c.Subcommand != nil {
			// If we're not already in the desired namespace, change to it
			// before running the command and then revert back afterwards. If
			// we're already in the namespace, just run the command.
			if ns.Name != name {
				if err := SetNamespace(name); err != nil {
					resp.Error = err.Error()
					respChan <- minicli.Responses{resp}
					return
				}
				defer RevertNamespace(ns, ns2)
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

	resp.Header = []string{"namespace", "vlans", "active"}
	for _, info := range InfoNamespaces() {
		row := []string{
			info.Name,
			"",
			strconv.FormatBool(info.Active),
		}

		if info.MinVLAN != 0 || info.MaxVLAN != 0 {
			row[2] = fmt.Sprintf("%v-%v", info.MinVLAN, info.MaxVLAN)
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	respChan <- minicli.Responses{resp}
}

func cliNS(c *minicli.Command, respChan chan<- minicli.Responses) {
	// Dispatcher for a sub handler
	for k, fn := range nsCliHandlers {
		if c.BoolArgs[k] {
			fn(c, respChan)
			return
		}
	}
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

		// Resolve `localhost` to actual hostname
		if host == Localhost {
			host = hostname
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
			end = stats.end.Format(time.RFC822)
		}

		row := []string{
			stats.start.Format(time.RFC822),
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
		// Going back to default namespace
		return SetNamespace(DefaultNamespace)
	}

	return DestroyNamespace(name)
}

func cliNamespaceRun(c *minicli.Command, respChan chan<- minicli.Responses) {
	ns := GetNamespace()

	// HAX: Ensure we aren't sending read or mesh send commands over meshage
	if hasCommand(c, "read") || hasCommand(c, "mesh send") {
		err := fmt.Errorf("cannot run `%s` using ns run", c.Original)
		respChan <- errResp(err)
		return
	}

	res := minicli.Responses{}

	// see wrapBroadcastCLI
	for resps := range runCommands(namespaceCommands(ns, c.Subcommand)...) {
		for _, resp := range resps {
			res = append(res, resp)
		}
	}

	respChan <- res
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
