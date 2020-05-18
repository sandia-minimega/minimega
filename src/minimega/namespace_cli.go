// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bridge"
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"minicli"
	log "minilog"
	"net"
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
- schedule  : run scheduler (same as "vm launch")
  - dry-run : determine VM placement and print out VM -> host assignments
  - dump    : print out VM -> host assignments (after dry-run)
  - mv      : manually edit VM placement in schedule (after dry-run)
  - status  : display scheduling status
- bridge    : create a bridge, defaults to GRE mesh between hosts
- del-bridge: destroy a bridge
- snapshot  : take a snapshot of namespace or print snapshot progress
- run       : run a command on all nodes in the namespace
`,
		Patterns: []string{
			"ns <hosts,>",
			"ns <add-hosts,> <hostname or range or all>",
			"ns <del-hosts,> <hostname or range or all>",
			"ns <load,>",
			"ns <load,> <cpucommit,>",
			"ns <load,> <netcommit,>",
			"ns <load,> <memcommit,>",
			"ns <queue,>",
			"ns <flush,>",
			"ns <queueing,> [true,false]",
			"ns <schedule,>",
			"ns <schedule,> <dry-run,>",
			"ns <schedule,> <dump,>",
			"ns <schedule,> <mv,> <vm target> <dst>",
			"ns <schedule,> <status,>",
			"ns <bridge,> <bridge> [vxlan,gre]",
			"ns <del-bridge,> <bridge>",
			"ns <snapshot,> [name]",
			"ns <run,> (command)",
		},
		Call: cliNS,
		Suggest: wrapSuggest(func(_ *Namespace, val, prefix string) []string {
			if val == "hostname" {
				return cliHostnameSuggest(prefix, true, false, true)
			}
			return nil
		}),
	},
	{ // clear namespace
		HelpShort: "unset or delete namespace",
		HelpLong: `
Without an argument, "clear namespace" will reset the namespace to the default
namespace, minimega.

With an argument, "clear namespace <name>" will destroy the specified
namespace, cleaning up all state associated with it. You may use "all" to
destroy all namespaces. This command is broadcast to the cluster to clean up
any remote state as well.`,
		Patterns: []string{
			"clear namespace [name]",
		},
		Call: cliClearNamespace,
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
	"hosts":      wrapSimpleCLI(cliNamespaceHosts),
	"add-hosts":  wrapSimpleCLI(cliNamespaceAddHost),
	"del-hosts":  wrapSimpleCLI(cliNamespaceDelHost),
	"load":       wrapSimpleCLI(cliNamespaceLoad),
	"queue":      wrapSimpleCLI(cliNamespaceQueue),
	"queueing":   wrapSimpleCLI(cliNamespaceQueueing),
	"flush":      wrapSimpleCLI(cliNamespaceFlush),
	"schedule":   wrapSimpleCLI(cliNamespaceSchedule),
	"bridge":     wrapSimpleCLI(cliNamespaceBridge),
	"del-bridge": wrapSimpleCLI(cliNamespaceDelBridge),
	"snapshot":   cliNamespaceSnapshot,
	"run":        cliNamespaceRun,
}

func cliNamespace(c *minicli.Command, respChan chan<- minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	// Get the active namespace
	ns := GetNamespace()

	if name, ok := c.StringArgs["name"]; ok {
		// check the name is sane
		if !validName.MatchString(name) {
			resp.Error = validNameErr.Error()
			respChan <- minicli.Responses{resp}
			return
		}

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

			// we don't want to see both:
			// 		namespace foo vm info
			// 		vm info
			// in the logs (well, we would actually see them in reverse order
			// because the inner command runs first).
			c.Subcommand.Record = false

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
			row[1] = fmt.Sprintf("%v-%v", info.MinVLAN, info.MaxVLAN)
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
	hosts, err := ranges.SplitList(c.StringArgs["hostname"])
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
	hosts, err := ranges.SplitList(c.StringArgs["hostname"])
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

func cliNamespaceSchedule(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["dry-run"]:
		if err := ns.Schedule(true); err != nil {
			return err
		}

		fallthrough
	case c.BoolArgs["dump"]:
		if ns.assignment == nil {
			return errors.New("must run dry-run first")
		}

		resp.Header = []string{"vm", "dst"}

		for k, vms := range ns.assignment {
			for _, vm := range vms {
				for _, v := range vm.Names {
					row := []string{v, k}

					resp.Tabular = append(resp.Tabular, row)
				}
			}
		}

		return nil
	case c.BoolArgs["mv"]:
		return ns.Reschedule(c.StringArgs["vm"], c.StringArgs["dst"])
	case c.BoolArgs["status"]:
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
	default:
		return ns.Schedule(false)
	}
}

func cliNamespaceBridge(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]
	if b == "" {
		return errors.New("bridge name must not be empty string")
	}

	if len(ns.Hosts) == 1 {
		return nil
	}

	tunnel := bridge.TunnelGRE
	if c.BoolArgs["vxlan"] {
		tunnel = bridge.TunnelVXLAN
	}

	// map from host to IP
	ips := map[string]string{}

	// create a copy that we can shuffle
	hosts := []string{}
	for host := range ns.Hosts {
		hosts = append(hosts, host)

		res, err := net.LookupIP(host)
		if err != nil {
			return fmt.Errorf("failure looking up %v: %v", host, err)
		}
		// track whether we found a non-loopback IP address
		foundIP := false
		for _, element := range res {
			if !element.IsLoopback() {
				foundIP = true
				ips[host] = element.String()
				break
			}
		}
		if !foundIP {
			return errors.New("host has no non-loopback IP")
		}
	}

	cmds := []*minicli.Command{}

	// create a random key for these tunnels
	key := rand.Uint32()

	// compute targets for each host, pairwise for now
	for host, hosts := range mesh(hosts, true) {
		// enable rstp before creating any tunnels because loops are bad
		cmd := minicli.MustCompilef("mesh send %q bridge config %q rstp_enable=true", host, b)
		if host == hostname {
			cmd = minicli.MustCompilef("bridge config %q rstp_enable=true", b)
		}

		cmds = append(cmds, cmd)

		for _, host2 := range hosts {
			cmd := minicli.MustCompilef("mesh send %q bridge tunnel %v %q %v %v", host, tunnel, b, ips[host2], key)
			if host == hostname {
				cmd = minicli.MustCompilef("bridge tunnel %v %q %v %v", tunnel, b, ips[host2], key)
			}

			cmds = append(cmds, cmd)
		}
	}

	// LOCK: This is a CLI handler.
	if err := consume(runCommands(cmds...)); err != nil {
		return err
	}

	// track bridge for later
	ns.Bridges[b] = key

	return nil
}

func cliNamespaceDelBridge(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	b := c.StringArgs["bridge"]
	if b == "" {
		return errors.New("bridge name must not be empty string")
	}

	if _, ok := ns.Bridges[b]; !ok {
		return errors.New("bridge is not associated with namespace")
	}

	cmds := []*minicli.Command{}

	for host := range ns.Hosts {
		cmd := minicli.MustCompilef("mesh send %q bridge destroy %q", host, b)
		if host == hostname {
			cmd = minicli.MustCompilef("bridge destroy %q", b)
		}

		cmds = append(cmds, cmd)
	}

	// LOCK: This is a CLI handler.
	return consume(runCommands(cmds...))
}

func cliNamespaceSnapshot(c *minicli.Command, respChan chan<- minicli.Responses) {
	ns := GetNamespace()

	resp := &minicli.Response{Host: hostname}

	if _, ok := c.StringArgs["name"]; !ok {
		cmd := minicli.MustCompile(".columns status vm migrate")

		var err error

		var total, completed int

		for resps := range runCommands(namespaceCommands(ns, cmd)...) {
			for _, resp := range resps {
				if resp.Error != "" {
					if err == nil {
						err = errors.New(resp.Error)
					}

					continue
				}

				for _, row := range resp.Tabular {
					if len(row) == 1 {
						if row[0] == "completed" {
							completed += 1
						}

						total += 1
					}
				}
			}
		}

		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Header = []string{"completed", "total"}
			resp.Tabular = append(resp.Tabular, []string{
				strconv.Itoa(completed),
				strconv.Itoa(total),
			})
		}

		respChan <- minicli.Responses{resp}
		return
	}

	// start new snapshot
	if err := ns.Snapshot(c.StringArgs["name"]); err != nil {
		resp.Error = err.Error()
	}

	respChan <- minicli.Responses{resp}
}

func cliClearNamespace(c *minicli.Command, respChan chan<- minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	name := c.StringArgs["name"]
	if name == "" {
		// Going back to default namespace
		if err := SetNamespace(DefaultNamespace); err != nil {
			respChan <- errResp(err)
			return
		}

		respChan <- minicli.Responses{resp}
		return
	}

	// clean up any bridges that we created
	if c.Source == "" {
		ns := GetOrCreateNamespace(name)

		cmds := []*minicli.Command{}

		for b := range ns.Bridges {
			cmd := minicli.MustCompilef("namespace %q ns del-bridge %q", name, b)
			cmds = append(cmds, cmd)
		}

		// LOCK: This is a CLI handler.
		if err := consume(runCommands(cmds...)); err != nil {
			respChan <- errResp(err)
			return
		}
	}

	// destroy the namespace locally first
	if err := DestroyNamespace(name); err != nil {
		respChan <- errResp(err)
		return
	}

	// destroy the namespace on all remote hosts as well
	if c.Source == "" {
		// recompile and set source so that we don't try to broadcast again
		cmd := minicli.MustCompilef(c.Original)
		cmd.Source = name

		respChan2, err := meshageSend(cmd, Wildcard)
		if err != nil {
			respChan <- errResp(err)
			return
		}

		res := minicli.Responses{resp}

		for resps := range respChan2 {
			for _, resp := range resps {
				// suppress warnings if we created and deleted the namespace
				// locally without actually running any commands to create the
				// namespace remotely.
				if strings.HasPrefix(resp.Error, "unknown namespace:") {
					resp.Error = ""
				}

				res = append(res, resp)
			}
		}

		respChan <- res
		return
	}

	respChan <- minicli.Responses{resp}
}

func cliNamespaceRun(c *minicli.Command, respChan chan<- minicli.Responses) {
	// HAX: prevent running as a subcommand
	if c.Source == SourceMeshage {
		err := fmt.Errorf("cannot run `%s` via meshage", c.Original)
		respChan <- errResp(err)
		return
	}

	// HAX: Make sure we don't run strange nested commands. We have to test for
	// these explicity rather than allow the handlers to check because the
	// Source for the locally executed commands will be the current namespace
	// and not "Meshage".
	for _, forbidden := range []string{"read", "mesh send", "ns run", "vm launch"} {
		if hasCommand(c.Subcommand, forbidden) {
			err := fmt.Errorf("cannot run `%s` using `ns run`", c.Subcommand.Original)
			respChan <- errResp(err)
			return
		}
	}

	ns := GetNamespace()

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
