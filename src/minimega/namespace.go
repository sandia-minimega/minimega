// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"path/filepath"
	"ron"
	"runtime"
	"sort"
	"sync"
	"time"
)

const (
	SchedulerRunning   = "running"
	SchedulerCompleted = "completed"

	DefaultNamespace = "minimega"
)

type scheduleStat struct {
	start, end time.Time

	state string

	launched, failures, total, hosts int
}

type Namespace struct {
	Name string

	Hosts map[string]bool

	// Queued VMs to launch
	queue []*QueuedVMs

	// Status of launching things
	scheduleStats []*scheduleStat

	// Names of host taps associated with this namespace
	Taps map[string]bool

	// How to determine which host is least loaded
	HostSortBy string

	VMs  // embed VMs for this namespace
	vmID *Counter

	// QueuedVMs toggles whether we should queue VMs or not when launching
	QueueVMs bool

	vmConfig      VMConfig
	savedVMConfig map[string]VMConfig

	captures // embed captures for this namespace

	routers map[int]*Router

	vncRecorder // embed vnc recorder for this namespace
	vncPlayer   // embed vnc player for this namespace

	// Command and control for this namespace
	ccServer *ron.Server
	ccFilter *ron.Filter
	ccPrefix string
}

type NamespaceInfo struct {
	Name    string
	VMs     int
	MinVLAN int
	MaxVLAN int
	Active  bool
}

var (
	namespace     string
	namespaces    = map[string]*Namespace{}
	namespaceLock sync.Mutex
)

func NewNamespace(name string) *Namespace {
	log.Info("creating new namespace -- `%v`", name)

	// so many maps
	ns := &Namespace{
		Name:       name,
		Hosts:      map[string]bool{},
		Taps:       map[string]bool{},
		HostSortBy: "cpucommit",
		VMs: VMs{
			m: make(map[int]VM),
		},
		vmID:    NewCounter(),
		routers: make(map[int]*Router),
		captures: captures{
			m:       make(map[int]capture),
			counter: NewCounter(),
		},
		vncRecorder: vncRecorder{
			kb: make(map[string]*vncKBRecord),
			fb: make(map[string]*vncFBRecord),
		},
		vncPlayer: vncPlayer{
			m: make(map[string]*vncKBPlayback),
		},
		vmConfig:      NewVMConfig(),
		savedVMConfig: make(map[string]VMConfig),
	}

	if name == DefaultNamespace {
		// default only contains this node by default
		ns.Hosts[hostname] = true

		// default does not use a subpath
		ccServer, err := ron.NewServer(*f_iomBase, "", plumber)
		if err != nil {
			log.Fatal("creating cc node %v", err)
		}
		ns.ccServer = ccServer

		return ns
	}

	ccServer, err := ron.NewServer(*f_iomBase, name, plumber)
	if err != nil {
		log.Fatal("creating cc node %v", err)
	}
	ns.ccServer = ccServer

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

	return ns
}

func (n *Namespace) String() string {
	return n.Name
}

func (n *Namespace) Destroy() error {
	log.Info("destroying namespace: %v", n.Name)

	for _, stats := range n.scheduleStats {
		// TODO: We could kill the scheduler -- that wouldn't be too hard to do
		// (add a kill channel and close it here). Easier to make the user
		// wait, for now.
		if stats.state != SchedulerCompleted {
			return errors.New("scheduler still running for namespace")
		}
	}

	n.vmID.Stop()

	// Stop all captures
	n.captures.StopAll()
	n.counter.Stop()

	// Stop VNC record/replay
	n.vncRecorder.Clear()
	n.vncPlayer.Clear()

	// Kill and flush all the VMs
	n.Kill(Wildcard)
	n.Flush()

	// Stop ron server
	n.ccServer.Destroy()

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
	mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

	n.ccServer.Destroy()

	return nil
}

// Queue handles storing the current VM config to the namespace's queued VMs so
// that we can launch it in the future.
func (n *Namespace) Queue(arg string, vmType VMType, vmConfig VMConfig) error {
	names, err := expandLaunchNames(arg)
	if err != nil {
		return err
	}

	if len(names) > 1 && vmConfig.UUID != "" {
		return errors.New("cannot launch multiple VMs with a pre-configured UUID")
	}

	// look for name and UUID conflicts across the namespace
	takenName := map[string]bool{}
	takenUUID := map[string]bool{}

	// LOCK: This is only invoked via the CLI so we already hold cmdLock (can
	// call globalVMs instead of GlobalVMs).
	for _, vm := range globalVMs(n) {
		takenName[vm.GetName()] = true
		takenUUID[vm.GetUUID()] = true
	}

	for _, name := range names {
		if takenName[name] {
			return fmt.Errorf("vm already exists with name `%s`", name)
		}
	}

	if takenUUID[vmConfig.UUID] && vmConfig.UUID != "" {
		return fmt.Errorf("vm already exists with UUID `%s`", vmConfig.UUID)
	}

	// add in all the queued VM names and then recheck
	for _, q := range n.queue {
		for _, name := range q.Names {
			takenName[name] = true
		}
		takenUUID[q.VMConfig.UUID] = true
	}

	for _, name := range names {
		if takenName[name] && name != "" {
			return fmt.Errorf("vm already queued with name `%s`", name)
		}
	}

	if takenUUID[vmConfig.UUID] && vmConfig.UUID != "" {
		return fmt.Errorf("vm already queued with UUID `%s`", vmConfig.UUID)
	}

	n.queue = append(n.queue, &QueuedVMs{
		VMConfig: vmConfig,
		VMType:   vmType,
		Names:    names,
	})

	return nil
}

// hostStats returns stats from hosts in the namespace.
//
// LOCK: Assumes cmdLock is held.
func (n *Namespace) hostStats() []*HostStats {
	// run `host` across the namespace
	cmds := namespaceCommands(n, minicli.MustCompile("host"))

	res := []*HostStats{}

	for resps := range runCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if v, ok := resp.Data.(*HostStats); ok {
				res = append(res, v)
			} else {
				log.Error("unknown data field in `host` from %v", resp.Host)
			}
		}
	}

	return res
}

// Schedule runs the scheduler, launching VMs across the cluster. Blocks until
// all the `vm launch ...` commands are in-flight.
//
// LOCK: Assumes cmdLock is held.
func (n *Namespace) Schedule() error {
	if len(n.Hosts) == 0 {
		return errors.New("namespace must contain at least one host to launch VMs")
	}

	if len(n.queue) == 0 {
		return errors.New("namespace must contain at least one queued VM to launch VMs")
	}

	hostStats := n.hostStats()

	var hostSorter hostSortBy
	for k, fn := range hostSortByFns {
		if n.HostSortBy == k {
			hostSorter = fn
		}
	}

	// Create the host -> VMs assignment
	assignment, err := schedule(n.queue, hostStats, hostSorter)
	if err != nil {
		return err
	}

	total := 0
	for _, q := range n.queue {
		total += len(q.Names)
	}

	// Clear the queuedVMs -- we're just about to launch them (hopefully!)
	n.queue = nil

	stats := &scheduleStat{
		total: total,
		hosts: len(hostStats),
		start: time.Now(),
		state: SchedulerRunning,
	}

	n.scheduleStats = append(n.scheduleStats, stats)

	// Result of vm launch commands
	respChan := make(chan minicli.Responses)

	var wg sync.WaitGroup

	for host, queue := range assignment {
		wg.Add(1)

		go func(host string, queue []*QueuedVMs) {
			defer wg.Done()

			for _, q := range queue {
				// set name here instead of on the remote host to ensure that we
				// get names that are unique across the namespace
				for i, name := range q.Names {
					if name == "" {
						q.Names[i] = fmt.Sprintf("vm-%v", n.vmID.Next())
					}
				}

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

// Launch wraps VMs.Launch, registering the launched VMs with ron. It blocks
// until all the VMs are launched.
func (n *Namespace) Launch(q *QueuedVMs) []error {
	vms, errChan := n.VMs.Launch(n.Name, q)

	// fire off goroutine to do the registration
	go func() {
		for vm := range vms {
			if err := vm.Connect(n.ccServer); err != nil {
				log.Warn("unable to connect to cc for vm %v: %v", vm.GetID(), err)
			}
		}
	}()

	// collect all the errors
	errs := []error{}
	for err := range errChan {
		errs = append(errs, err)
	}

	return errs
}

// Flush wraps VMs.Flush, unregistering the flushed VMs with ron.
func (n *Namespace) Flush() {
	for _, vm := range n.VMs.Flush() {
		n.ccServer.UnregisterVM(vm)
	}
}

// NewCommand takes a command, adds the current filter and prefix, and then
// sends the command to ron.
func (ns *Namespace) NewCommand(c *ron.Command) int {
	c.Filter = ns.ccFilter
	c.Prefix = ns.ccPrefix

	id := ns.ccServer.NewCommand(c)
	log.Debug("generated command %v: %v", id, c)

	return id
}

// hostLaunch launches a queuedVM on the specified host and namespace.
func (n *Namespace) hostLaunch(host string, queued *QueuedVMs, respChan chan<- minicli.Responses) {
	log.Info("scheduling %v %v VMs on %v", len(queued.Names), queued.VMType, host)

	// Launching the VMs locally
	if host == hostname {
		resp := &minicli.Response{Host: hostname}

		if err := makeErrSlice(n.Launch(queued)); err != nil {
			resp.Error = err.Error()
		}

		respChan <- minicli.Responses{resp}

		return
	}

	forward(meshageLaunch(host, n.Name, queued), respChan)
}

// hostSlice converts the hosts map into a slice of hostnames
func (n *Namespace) hostSlice() []string {
	hosts := []string{}
	for host := range n.Hosts {
		hosts = append(hosts, host)
	}

	return hosts
}

// processVMNets parses a list of netspecs using processVMNet and updates the
// active vmConfig.
func (n *Namespace) processVMNets(vals []string) error {
	n.vmConfig.Networks = nil

	for _, spec := range vals {
		nic, err := processVMNet(n.Name, spec)
		if err != nil {
			n.vmConfig.Networks = nil
			return err
		}
		nic.Raw = spec

		n.vmConfig.Networks = append(n.vmConfig.Networks, nic)
	}

	return nil
}

// GetNamespace returns the active namespace.
func GetNamespace() *Namespace {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	_, ok := namespaces[namespace]
	if namespace == DefaultNamespace && !ok {
		// recreate automatically
		namespaces[namespace] = NewNamespace(namespace)
	}

	return namespaces[namespace]
}

// GetOrCreateNamespace returns the specified namespace, creating one if it
// doesn't already exist.
func GetOrCreateNamespace(name string) *Namespace {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	if _, ok := namespaces[name]; !ok {
		namespaces[name] = NewNamespace(name)
	}

	return namespaces[name]
}

// SetNamespace sets the active namespace
func SetNamespace(name string) error {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	if name == "" {
		return errors.New("namespace name cannot be the empty string")
	}

	log.Info("setting active namespace: %v", name)

	if name == namespace {
		return fmt.Errorf("already in namespace: %v", name)
	}

	namespace = name
	return nil
}

// RevertNamespace reverts the active namespace (which should match curr) back
// to the old namespace.
func RevertNamespace(old, curr *Namespace) {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	log.Info("reverting to namespace: %v", old)

	// This is very odd and should *never* happen unless something has gone
	// horribly wrong.
	if namespace != curr.Name {
		log.Warn("unexpected namespace, `%v` != `%v`, when reverting to `%v`", namespace, curr, old)
	}

	namespace = old.Name
}

func DestroyNamespace(name string) error {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	var found bool

	for n, ns := range namespaces {
		if n != name && name != Wildcard {
			continue
		}

		found = true

		if err := ns.Destroy(); err != nil {
			return err
		}

		// If we're deleting the currently active namespace, we should get out of
		// that namespace
		if namespace == n {
			log.Info("active namespace destroyed, switching to default namespace")
			namespace = DefaultNamespace
		}

		delete(namespaces, n)
	}

	if !found && name != Wildcard {
		return fmt.Errorf("unknown namespace: %v", name)
	}

	return nil
}

// ListNamespaces lists all the namespaces.
func ListNamespaces() []string {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	res := []string{}
	for n := range namespaces {
		res = append(res, n)
	}

	// make sure the order is always the same
	sort.Strings(res)

	return res
}

// InfoNamespaces returns information about all namespaces
func InfoNamespaces() []NamespaceInfo {
	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	res := []NamespaceInfo{}
	for n := range namespaces {
		info := NamespaceInfo{
			Name:   n,
			Active: namespace == n,
		}

		for prefix, r := range allocatedVLANs.GetRanges() {
			if prefix == n || (prefix == "" && n == DefaultNamespace) {
				info.MinVLAN = r.Min
				info.MaxVLAN = r.Max
			}
		}

		res = append(res, info)
	}

	return res
}

// NewHostStats populates HostStats with fields spanning all namespaces.
func NewHostStats() *HostStats {
	h := HostStats{
		Name: hostname,
	}

	var err error

	// compute fields that don't require namespaceLock
	h.CPUs = runtime.NumCPU()
	h.Load, err = hostLoad()
	if err != nil {
		log.Error("unable to compute load: %v", err)
	}
	h.MemTotal, h.MemUsed, err = hostStatsMemory()
	if err != nil {
		log.Error("unable to compute memory stats: %v", err)
	}
	h.RxBps, h.TxBps = bridges.BandwidthStats()
	h.Uptime, err = hostUptime()
	if err != nil {
		log.Error("unable to compute uptime: %v", err)
	}

	namespaceLock.Lock()
	defer namespaceLock.Unlock()

	// default is unlimited unless we find out otherwise
	h.Limit = -1

	for _, ns := range namespaces {
		cpu, mem, net := ns.VMs.Commit()
		h.CPUCommit += cpu
		h.MemCommit += mem
		h.NetworkCommit += net
		h.VMs += ns.VMs.Count()

		// update if limit is unlimited or we're not unlimited and we're less
		// than the previous limit
		v := ns.VMs.Limit()
		if h.Limit == -1 || (v != -1 && v < h.Limit) {
			h.Limit = v
		}
	}

	if h.Limit != -1 {
		// we add one here because if we say `vm config coschedule 0` then what
		// we really want is there to only be one VM on the host
		h.Limit += 1
	}

	return &h
}
