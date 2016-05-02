// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"strings"
	"sync"
	"time"
)

const (
	SchedulerRunning   = "running"
	SchedulerCompleted = "completed"
)

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

var (
	namespace     string
	namespaces    = map[string]*Namespace{}
	namespaceLock sync.Mutex
)

func (n Namespace) String() string {
	return n.Name
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

func (n Namespace) hostSlice() []string {
	hosts := []string{}
	for host := range n.Hosts {
		hosts = append(hosts, host)
	}

	return hosts
}

// Queue handles storing the current VM config to the namespace's queuedVMs so
// that we can launch it in the future.
func (n *Namespace) Queue(arg string, vmType VMType) error {
	// LOCK: This is only invoked via the CLI so we already hold cmdLock (can
	// call globalVMs instead of GlobalVMs).
	names, err := expandLaunchNames(arg, globalVMs())
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
	for _, queued := range n.queuedVMs {
		for _, name := range queued.names {
			if namesMap[name] {
				return fmt.Errorf("vm already queued with name `%s`", name)
			}
		}
	}

	n.queuedVMs = append(n.queuedVMs, queuedVM{
		VMConfig: vmConfig.Copy(),
		names:    names,
		vmType:   vmType,
	})

	return nil
}

// Launch runs the scheduler and launches VMs across the namespace. Blocks
// until all the `vm launch ... noblock` commands are in-flight.
func (n *Namespace) Launch() error {
	if len(n.Hosts) == 0 {
		return errors.New("namespace must contain at least one host to launch VMs")
	}

	if len(n.queuedVMs) == 0 {
		return errors.New("namespace must contain at least one queued VM to launch VMs")
	}

	// Create the host -> VMs assignment
	// TODO: This is a static assignment... should it be updated periodically
	// during the launching process?
	stats, assignment := schedule(n.queuedVMs, n.hostSlice())

	// Clear the queuedVMs -- we're just about to launch them (hopefully!)
	n.queuedVMs = nil

	stats.start = time.Now()
	stats.state = SchedulerRunning

	n.scheduleStats = append(n.scheduleStats, stats)

	// Result of vm launch commands
	respChan := make(chan minicli.Responses)

	var wg sync.WaitGroup

	for host, queuedVMs := range assignment {
		wg.Add(1)

		go func(host string, queuedVMs []queuedVM) {
			defer wg.Done()

			for _, q := range queuedVMs {
				n.HostLaunch(host, q, respChan)
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
func (n *Namespace) HostLaunch(host string, queued queuedVM, respChan chan<- minicli.Responses) {
	log.Info("scheduling %v %v VMs on %v", len(queued.names), queued.vmType, host)

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

	// Replace empty VM names with generic name
	for i, name := range queued.names {
		if name == "" {
			queued.names[i] = fmt.Sprintf("vm-%v-%v", n.Name, n.vmID.Next())
		}
	}

	// Send the launch command
	names := strings.Join(queued.names, ",")
	log.Debug("launch vms on host %v -- %v", host, names)

	cmd := minicli.MustCompilef("namespace %q vm launch %v %v noblock", n.Name, queued.vmType, names)
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

// GetNamespace returns the active namespace. Returns nil if there isn't a
// namespace active.
func GetNamespace() *Namespace {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	return namespaces[namespace]
}

func GetNamespaceName() string {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	return namespace
}

// GetOrCreateNamespace returns the specified namespace, creating one if it
// doesn't already exist.
func GetOrCreateNamespace(name string) *Namespace {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	if _, ok := namespaces[name]; !ok {
		log.Info("creating new namespace -- `%v`", name)

		ns := &Namespace{
			Name:  name,
			Hosts: map[string]bool{},
			vmID:  NewCounter(),
		}

		// By default, every mesh-reachable node is part of the namespace
		// except for the local node which is typically the "head" node.
		for _, host := range meshageNode.BroadcastRecipients() {
			ns.Hosts[host] = true
		}

		namespaces[name] = ns
	}

	return namespaces[name]
}

// SetNamespace sets the active namespace
func SetNamespace(name string) {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	log.Info("setting active namespace: %v", name)

	namespace = name
}

// RevertNamespace reverts the active namespace (which should match curr) back
// to the old namespace.
func RevertNamespace(old, curr *Namespace) {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	// This is very odd and should *never* happen unless something has gone
	// horribly wrong.
	if namespace != curr.Name {
		log.Warn("unexpected namespace, `%v` != `%v`, when reverting to `%v`", namespace, curr, old)
	}

	if old == nil {
		namespace = ""
	} else {
		namespace = old.Name
	}
}

func DestroyNamespace(name string) error {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	log.Info("destroying namespace: %v", name)

	ns, ok := namespaces[namespace]
	if !ok {
		return fmt.Errorf("unknown namespace: %v", name)
	}

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

func ListNamespaces() []string {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	res := []string{}
	for n := range namespaces {
		res = append(res, n)
	}

	return res
}
