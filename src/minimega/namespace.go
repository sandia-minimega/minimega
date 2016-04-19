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
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

const (
	SchedulerRunning   = "running"
	SchedulerCompleted = "completed"
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

type queuedVM struct {
	VMConfig // embed

	names  []string
	vmType VMType
}

type scheduleStat struct {
	start, end time.Time

	state string

	launched, failures, total, hosts int
}

type Namespace struct {
	Name string

	Hosts map[string]bool

	vmID *Counter

	// Queued VMs to launch,
	queuedVMs []queuedVM

	// Status of launching things
	scheduleStats []*scheduleStat
}

var namespace string
var namespaces map[string]*Namespace

func init() {
	namespaces = map[string]*Namespace{}
}

func (n Namespace) hostSlice() []string {
	hosts := []string{}
	for host := range n.Hosts {
		hosts = append(hosts, host)
	}

	return hosts
}

func (n Namespace) Destroy() error {
	// TODO: should we ensure that there are no VMs running in the namespace
	// before we delete it?

	for _, stats := range n.scheduleStats {
		// TODO: We could kill the scheduler -- that wouldn't be too hard to do
		// (add a kill channel and close it here). Easier to make the user
		// wait, for now.
		if stats.state != SchedulerCompleted {
			return errors.New("scheduler still running for namespace")
		}
	}

	// Free up any VLANs associated with the namespace
	allocatedVLANs.Delete(n.Name, "")

	n.vmID.Stop()

	return nil
}

func cliNamespace(c *minicli.Command, respChan chan minicli.Responses) {
	resp := &minicli.Response{Host: hostname}

	if name, ok := c.StringArgs["name"]; ok {
		if _, ok := namespaces[name]; !ok && name != "" {
			log.Info("creating new namespace -- %v", name)

			if strings.Contains(name, ".") {
				log.Warn("namespace names probably shouldn't contain `.`")
			}

			ns := Namespace{
				Name:  name,
				Hosts: map[string]bool{},
				vmID:  NewCounter(),
			}

			// By default, every mesh-reachable node is part of the namespace
			// except for the local node which is typically the "head" node.
			for _, host := range meshageNode.BroadcastRecipients() {
				ns.Hosts[host] = true
			}

			namespaces[name] = &ns
		}

		if c.Subcommand != nil {
			// Setting namespace for a single command, revert back afterwards
			defer func(old string) {
				namespace = old
			}(namespace)
			namespace = name

			// Run the subcommand and forward the responses
			forward(minicli.ProcessCommand(c.Subcommand), respChan)
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
		// TODO: Make this pretty or divide it up somehow
		ns := namespaces[namespace]
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
	}

	respChan <- minicli.Responses{resp}
}

func cliNamespaceMod(c *minicli.Command, resp *minicli.Response) error {
	if namespace == "" {
		return errors.New("cannot run nsmod without active namespace")
	}

	ns := namespaces[namespace]

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
		namespace = ""
		return nil
	}

	// Trying to delete a namespace
	ns, exists := namespaces[name]
	if !exists {
		return fmt.Errorf("unknown namespace `%v`", name)
	}

	// Attempt to destroy the namespace
	if err := ns.Destroy(); err != nil {
		return err
	}

	// If we're deleting the currently active namespace, we should get out of
	// that namespace
	if namespace == name {
		namespace = ""
	}

	delete(namespaces, name)
	return nil
}

// namespaceQueue handles storing the current VM config to the namespace's
// queuedVMs so that we can launch it in the future.
func namespaceQueue(c *minicli.Command, resp *minicli.Response) error {
	ns := namespaces[namespace]

	names, err := expandVMLaunchNames(c.StringArgs["name"], GlobalVMs())
	if err != nil {
		return err
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
				return fmt.Errorf("vm already queued with name `%s`", name)
			}
		}
	}

	vmType, err := findVMType(c.BoolArgs)
	if err != nil {
		return err
	}

	ns.queuedVMs = append(ns.queuedVMs, queuedVM{
		VMConfig: vmConfig.Copy(),
		names:    names,
		vmType:   vmType,
	})

	return nil
}

// namespaceLaunch runs the scheduler and launches VMs across the namespace.
// Blocks until all the `vm launch ... noblock` commands are in-flight.
func namespaceLaunch(c *minicli.Command, resp *minicli.Response) error {
	ns := namespaces[namespace]

	if len(ns.Hosts) == 0 {
		return errors.New("namespace must contain at least one host to launch VMs")
	}

	if len(ns.queuedVMs) == 0 {
		return errors.New("namespace must contain at least one queued VM to launch VMs")
	}

	// Create the host -> VMs assignment
	// TODO: This is a static assignment... should it be updated periodically
	// during the launching process?
	stats, assignment := schedule(namespace)

	// Clear the queuedVMs -- we're just about to launch them (hopefully!)
	ns.queuedVMs = nil

	stats.start = time.Now()
	stats.state = SchedulerRunning

	ns.scheduleStats = append(ns.scheduleStats, stats)

	// Result of vm launch commands
	respChan := make(chan minicli.Responses)

	var wg sync.WaitGroup

	for host, queuedVMs := range assignment {
		wg.Add(1)

		go func(host string, queuedVMs []queuedVM) {
			defer wg.Done()

			for _, q := range queuedVMs {
				namespaceHostLaunch(host, namespace, q, respChan)
			}
		}(host, queuedVMs)
	}

	go func() {
		wg.Wait()
		close(respChan)
	}()

	// Collect all the responses and log them
	for resps := range respChan {
		for _, resp := range resps {
			stats.launched += 1
			if resp.Error != "" {
				stats.failures += 1
				log.Error("launch error, host %v -- %v", resp.Host, resp.Error)
			} else if resp.Response != "" {
				log.Debug("launch response, host %v -- %v", resp.Host, resp.Response)
			}
		}
	}

	stats.end = time.Now()
	stats.state = SchedulerCompleted

	return nil
}

// namespaceHostLaunch launches a queuedVM on the specified host and namespace.
// We blast a bunch of `vm config` commands at the host and then call `vm
// launch ... noblock` if there are no errors. We assume that this is
// serialized on a per-host basis -- it's fine to run multiple of these in
// parallel, as long as they target different hosts.
func namespaceHostLaunch(host, namespace string, queued queuedVM, respChan chan minicli.Responses) {
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

	// Run all the config commands on the local or remote node, sending all the
	// responses to configChan.
	configChan := make(chan minicli.Responses)

	// run all the config commands
	go func() {
		defer close(configChan)

		for _, cmd := range cmds {
			cmd := minicli.MustCompile(cmd)
			cmd.SetRecord(false)

			if host == hostname {
				forward(processCommands(cmd), configChan)
			} else {
				in, err := meshageSend(cmd, host)
				if err != nil {
					configChan <- errResp(err)
					break
				}

				forward(in, configChan)
			}
		}
	}()

	var abort bool

	// Read all the configChan responses, set abort if we find a single error.
	for resps := range configChan {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Error("config error, host %v -- %v", resp.Host, resp.Error)
				abort = true
			}
		}
	}

	if abort {
		return
	}

	// Send the launch command
	names := strings.Join(queued.names, ",")
	log.Debug("launch vms on host %v -- %v", host, names)

	cmd := minicli.MustCompilef("namespace %q vm launch %v %v noblock", namespace, queued.vmType, names)
	cmd.SetRecord(false)
	cmd.SetSource(namespace)

	if host == hostname {
		forward(processCommands(cmd), respChan)
	} else {
		in, err := meshageSend(cmd, host)
		if err != nil {
			respChan <- errResp(err)
			return
		}

		forward(in, respChan)
	}
}
