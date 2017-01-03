// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"sync"
	"time"
)

const (
	SchedulerRunning   = "running"
	SchedulerCompleted = "completed"
)

type scheduleStat struct {
	start, end time.Time

	state string

	launched, failures, total, hosts int
}

type Namespace struct {
	Name string

	Hosts map[string]bool

	vmID *Counter

	// Queued VMs to launch
	queue []QueuedVMs

	// Status of launching things
	scheduleStats []*scheduleStat

	// Names of host taps associated with this namespace
	Taps map[string]bool
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

	// Delete any Taps associated with the namespace
	for t := range n.Taps {
		tap, err := bridges.FindTap(t)
		if err != nil {
			return err
		}

		br, err := getBridge(tap.Bridge)
		if err != nil {
			return err
		}

		if err := br.DestroyTap(tap.Name); err != nil {
			return err
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

func (n Namespace) AddTap(tap string) {
	n.Taps[tap] = true
}

func (n Namespace) HasTap(tap string) bool {
	return n.Taps[tap]
}

func (n Namespace) RemoveTap(tap string) {
	delete(n.Taps, tap)
}

// Queue handles storing the current VM config to the namespace's queued VMs so
// that we can launch it in the future.
func (n *Namespace) Queue(arg string, vmType VMType, vmConfig VMConfig) error {
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
	for _, q := range n.queue {
		for _, name := range q.Names {
			if namesMap[name] {
				return fmt.Errorf("vm already queued with name `%s`", name)
			}
		}
	}

	n.queue = append(n.queue, QueuedVMs{
		VMConfig: vmConfig,
		VMType:   vmType,
		Names:    names,
	})

	return nil
}

// Launch runs the scheduler and launches VMs across the namespace. Blocks
// until all the `vm launch ... noblock` commands are in-flight.
func (n *Namespace) Launch() error {
	if len(n.Hosts) == 0 {
		return errors.New("namespace must contain at least one host to launch VMs")
	}

	if len(n.queue) == 0 {
		return errors.New("namespace must contain at least one queued VM to launch VMs")
	}

	// Create the host -> VMs assignment
	// TODO: This is a static assignment... should it be updated periodically
	// during the launching process?
	stats, assignment := schedule(n.queue, n.hostSlice())

	// Clear the queuedVMs -- we're just about to launch them (hopefully!)
	n.queue = nil

	stats.start = time.Now()
	stats.state = SchedulerRunning

	n.scheduleStats = append(n.scheduleStats, stats)

	// Result of vm launch commands
	respChan := make(chan minicli.Responses)

	var wg sync.WaitGroup

	for host, queue := range assignment {
		wg.Add(1)

		go func(host string, queue []QueuedVMs) {
			defer wg.Done()

			for _, q := range queue {
				n.hostLaunch(host, q, respChan)
			}
		}(host, queue)
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

	log.Info("scheduling complete")

	stats.end = time.Now()
	stats.state = SchedulerCompleted

	return nil
}

// hostLaunch launches a queuedVM on the specified host and namespace.
func (n *Namespace) hostLaunch(host string, queued QueuedVMs, respChan chan<- minicli.Responses) {
	log.Info("scheduling %v %v VMs on %v", len(queued.Names), queued.VMType, host)

	// Launching the VMs locally
	if host == hostname {
		errs := []error{}
		for err := range vms.Launch(n.Name, queued) {
			errs = append(errs, err)
		}

		resp := &minicli.Response{Host: hostname}

		if err := makeErrSlice(errs); err != nil {
			resp.Error = err.Error()
		}

		respChan <- minicli.Responses{resp}

		return
	}

	forward(meshageLaunch(host, n.Name, queued), respChan)
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
			Taps:  map[string]bool{},
			vmID:  NewCounter(),
		}

		// By default, every mesh-reachable node is part of the namespace
		// except for the local node which is typically the "head" node.
		for _, host := range meshageNode.BroadcastRecipients() {
			ns.Hosts[host] = true
		}

		// If there aren't any other nodes in the mesh, assume that minimega is
		// running in a single host environment and that we want to launch VMs
		// on localhost.
		if len(ns.Hosts) == 0 {
			log.Info("no meshage peers, adding localhost to the namespace")
			ns.Hosts[hostname] = true
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

	ns, ok := namespaces[name]
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
