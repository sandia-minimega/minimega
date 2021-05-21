// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"qemu"
	"ranges"
	"ron"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"vlans"
	"vnc"
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

	// Assignment created by dry-run that the user can tinker with. Used on the
	// next call to Schedule() unless invalidated.
	assignment map[string][]*QueuedVMs

	// Status of launching things
	scheduleStats []*scheduleStat

	// Names of host taps associated with this namespace
	Taps map[string]bool

	// Names of bridges associated with this namespace and there associated
	// tunnel keys
	Bridges map[string]uint32

	// Names of mirrors associated with this namespace
	Mirrors map[string]bool

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

	*vnc.Recorder // embed vnc recorder for this namespace
	*vnc.Player   // embed vnc player for this namespace

	// Command and control for this namespace
	ccServer *ron.Server
	ccFilter *ron.Filter
	ccPrefix string

	ccMounts map[string]ccMount

	// optimizations
	hugepagesMountPath string

	affinityEnabled bool
	affinityFilter  []string
	affinityMu      sync.Mutex // protects affinityCPUSets
	affinityCPUSets map[string][]int
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
		Bridges:    map[string]uint32{},
		Mirrors:    map[string]bool{},
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
		Recorder:      vnc.NewRecorder(),
		Player:        vnc.NewPlayer(),
		vmConfig:      NewVMConfig(),
		savedVMConfig: make(map[string]VMConfig),
		ccMounts:      make(map[string]ccMount),
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

	// unmount
	n.clearCCMount("")

	// Stop all captures
	n.captures.StopAll()
	n.counter.Stop()

	// Stop VNC record/replay
	n.Recorder.Clear()
	n.Player.Clear()

	// Kill and flush all the VMs
	n.Kill(Wildcard)
	n.FlushAll(n.ccServer)

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

	// We don't need to delete mirrors -- deleting the taps should clean those
	// up automatically.

	// Free up any VLANs associated with the namespace
	vlans.Delete(n.Name, "")
	mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

	n.ccServer.Destroy()

	return nil
}

// Queue handles storing the current VM config to the namespace's queued VMs so
// that we can launch it in the future.
func (n *Namespace) Queue(arg string, vmType VMType, vmConfig VMConfig) error {
	// invalidate assignment
	n.assignment = nil

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

		if name != "" && !validName.MatchString(name) {
			return fmt.Errorf("%v: `%v`", validNameErr, name)
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
// If dryRun is true, the scheduler will determine VM placement but not
// actually launch any VMs so that the user can tinker with the schedule.
//
// LOCK: Assumes cmdLock is held.
func (n *Namespace) Schedule(dryRun bool) error {
	if len(n.Hosts) == 0 {
		return errors.New("namespace must contain at least one host to launch VMs")
	}

	if len(n.queue) == 0 {
		return errors.New("namespace must contain at least one queued VM to launch VMs")
	}

	// already have assignment so if we're not doing a dry run, run it
	if n.assignment != nil && !dryRun {
		if err := n.schedule(n.assignment); err != nil {
			return err
		}

		n.assignment = nil
		return nil
	}

	// otherwise, generate a fresh assignment

	hostStats := n.hostStats()

	var hostSorter hostSortBy
	for k, fn := range hostSortByFns {
		if n.HostSortBy == k {
			hostSorter = fn
		}
	}

	// resolve any "colocated" VMs for VMs that are already launched
	for _, vm := range globalVMs(n) {
		for _, q := range n.queue {
			if q.Colocate == vm.GetName() {
				q.Schedule = vm.GetHost()
			}
		}
	}

	// Create the host -> VMs assignment
	assignment, err := schedule(n.queue, hostStats, hostSorter)
	if err != nil {
		return err
	}

	if dryRun {
		n.assignment = assignment
		return nil
	}

	return n.schedule(assignment)
}

func (n *Namespace) schedule(assignment map[string][]*QueuedVMs) error {
	total := 0
	for _, q := range n.queue {
		total += len(q.Names)
	}

	// Clear the queuedVMs -- we're just about to launch them (hopefully!)
	n.queue = nil

	stats := &scheduleStat{
		total: total,
		hosts: len(n.Hosts),
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
			if resp.Error != "" {
				stats.failures += 1
				log.Error("launch error, host %v -- %v", resp.Host, resp.Error)
			} else {
				// Response should number of VMs launched
				i, _ := strconv.Atoi(resp.Response)
				stats.launched += i

				log.Debug("launch response, host %v -- %v", resp.Host, resp.Response)
			}
		}
	}

	log.Info("scheduling complete")

	stats.end = time.Now()
	stats.state = SchedulerCompleted

	return nil
}

// Reschedule
func (n *Namespace) Reschedule(target, dst string) error {
	if n.assignment == nil {
		return errors.New("must run dry-run first")
	}

	if !n.Hosts[dst] {
		return errors.New("new dst host is not in namespace")
	}

	vals, err := ranges.SplitList(target)
	if err != nil {
		return err
	}

Outer:
	for _, v := range vals {
		// find each VM
		for src, qs := range n.assignment {
			for i, q := range qs {
				for j, v2 := range q.Names {
					// no match
					if v != v2 {
						continue
					}

					if len(q.Names) == 1 {
						// only a single name, simply relocate whole QueuedVMs
						n.assignment[src] = append(n.assignment[src][:i], n.assignment[src][i+1:]...)
						n.assignment[dst] = append(n.assignment[dst], q)

						continue Outer
					}

					// more than one name, need to split QueuedVMs
					q2 := *q
					q2.Names = []string{v2}
					q.Names = append(q.Names[:j], q.Names[j+1:]...)

					n.assignment[dst] = append(n.assignment[dst], &q2)
					continue Outer
				}
			}
		}

		// didn't find vm -- strange
		return fmt.Errorf("reassign %v: vm not found", v)
	}

	return nil
}

// Launch wraps VMs.Launch, registering the launched VMs with ron. It blocks
// until all the VMs are launched.
func (n *Namespace) Launch(q *QueuedVMs) []error {
	// collect all the errors
	errs := []error{}
	for err := range n.VMs.Launch(n.Name, q) {
		errs = append(errs, err)
	}

	return errs
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
		resp := &minicli.Response{
			Host:     hostname,
			Response: strconv.Itoa(len(queued.Names)),
		}

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

// processVMDisks parses a list of diskspecs using processVMDisk and updates the
// active vmConfig.
func (n *Namespace) processVMDisks(vals []string) error {
	n.vmConfig.Disks = nil

	var ideCount int

	for _, spec := range vals {
		disk, err := ParseDiskConfig(spec, n.vmConfig.Snapshot)
		if err != nil {
			n.vmConfig.Disks = nil
			return err
		}

		if disk.Interface == "ide" || (disk.Interface == "" && DefaultKVMDiskInterface == "ide") {
			ideCount += 1
		}

		// check for disk conflicts in a single VM
		for _, d2 := range n.vmConfig.Disks {
			if disk.Path == d2.Path {
				n.vmConfig.Disks = nil
				return fmt.Errorf("disk conflict: %v", d2.Path)
			}
		}

		n.vmConfig.Disks = append(n.vmConfig.Disks, *disk)
	}

	if ideCount > 3 {
		// Warn or return an error? Maybe some systems support more than four?
		log.Warn("too many IDE devices, one for cdrom and %v for disks", ideCount)
	}

	return nil
}

// processVMNets parses a list of netspecs using processVMNet and updates the
// active vmConfig.
func (n *Namespace) parseVMNets(vals []string) ([]NetConfig, error) {
	// get valid NIC drivers for current qemu/machine
	nics, err := qemu.NICs(n.vmConfig.QemuPath, n.vmConfig.Machine)

	// warn on not finding kvm because we may just be using containers,
	// otherwise throw a regular error
	if err != nil && strings.Contains(err.Error(), "executable file not found in $PATH") {
		log.Warnln(err)
	} else if err != nil {
		return nil, err
	}

	res := []NetConfig{}

	for _, spec := range vals {
		nic, err := ParseNetConfig(spec, nics)
		if err != nil {
			n.vmConfig.Networks = nil
			return nil, err
		}

		vlan, err := lookupVLAN(n.Name, nic.Alias)
		if err != nil {
			n.vmConfig.Networks = nil
			return nil, err
		}

		nic.VLAN = vlan
		nic.Raw = spec
		res = append(res, *nic)
	}

	return res, nil
}

// processVMNets parses a list of netspecs using parseVMNet and updates the
// active vmConfig.
func (n *Namespace) processVMNets(vals []string) error {
	n.vmConfig.Networks = nil

	nics, err := n.parseVMNets(vals)
	if err != nil {
		return err
	}

	for _, nic := range nics {
		n.vmConfig.Networks = append(n.vmConfig.Networks, nic)
	}
	return nil
}

// Snapshot creates a snapshot of a namespace so that it can be restored later.
// Both a state file (migrate) and hard disk file (disk) are created for each
// VM in the namespace. If dir is not an absolute path, it will be a
// subdirectory of iomBase.
//
// LOCK: Assumes cmdLock is held.
func (n *Namespace) Snapshot(dir string) error {
	var useIOM bool
	if !filepath.IsAbs(dir) {
		useIOM = true
		dir = filepath.Join(*f_iomBase, "snapshots", dir)
	}

	if _, err := os.Stat(dir); err == nil {
		return errors.New("snapshot with this name already exists")
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(dir, "launch.mm"))
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "namespace %q\n\n", n.Name)

	// LOCK: This is only invoked via the CLI so we already hold cmdLock (can
	// call globalVMs instead of GlobalVMs).
	for _, vm := range globalVMs(n) {
		// only snapshot KVMs
		if vm.GetType() == KVM {
			cmds := []*minicli.Command{}
			// pause all vms
			cmd := minicli.MustCompilef("vm stop all")
			cmd.Record = false
			cmds = append(cmds, cmd)
			// snapshot all vms
			stateDst := filepath.Join(dir, vm.GetName()) + ".migrate"
			diskDst := filepath.Join(dir, vm.GetName()) + ".hdd"
			cmd = minicli.MustCompilef("vm snapshot %q %v %v", vm.GetName(), stateDst, diskDst)
			cmd.Record = false
			cmds = append(cmds, cmd)

			var respChan <-chan minicli.Responses
			for _, c := range cmds {
				if vm.GetHost() == hostname {
					// run locally
					respChan = runCommands(c)
				} else {
					// run remotely
					cmd = minicli.MustCompilef("namespace %q %v", n.Name, c.Original)
					cmd.Source = n.Name
					cmd.Record = false

					var err error
					respChan, err = meshageSend(cmd, vm.GetHost())
					if err != nil {
						return err
					}
				}
			}

			// read all the responses and look for any errors
			if err := consume(respChan); err != nil {
				return err
			}

			fmt.Fprintf(f, "clear vm config\n")

			if err := vm.WriteConfig(f); err != nil {
				return err
			}

			// override the migrate and disk paths;
			// skip disk if using kernel/initrd or cdrom as boot device
			disks, _ := vm.Info("disks")
			if useIOM {
				rel, _ := filepath.Rel(*f_iomBase, stateDst)
				fmt.Fprintf(f, "vm config migrate file:%v\n", rel)
				if disks != "" {
					rel, _ = filepath.Rel(*f_iomBase, diskDst)
					fmt.Fprintf(f, "vm config disk file:%v\n", rel)
				}
			} else {
				fmt.Fprintf(f, "vm config migrate %v\n", stateDst)
				if disks != "" {
					fmt.Fprintf(f, "vm config disk %v\n", diskDst)
				}
			}
		} else if vm.GetType() == CONTAINER {
			log.Warn("Skipping snapshot for container: %q\n", vm.GetName(), err)
			fmt.Fprintf(f, "clear vm config\n")

			if err := vm.WriteConfig(f); err != nil {
				return err
			}
		}

		fmt.Fprintf(f, "vm launch %v %q\n\n", vm.GetType(), vm.GetName())
	}

	fmt.Fprintf(f, "vm start all\n")
	// the snapshot process saves the VMs in a paused state, so do a stop/start
	fmt.Fprintf(f, "# the snapshot process saves the VMs in a paused state, so do a stop/start\n")
	fmt.Fprintf(f, "shell sleep 10\n")
	fmt.Fprintf(f, "vm stop all\n")
	fmt.Fprintf(f, "vm start all\n")

	return nil
}

// Start VMs matching target and setup interactions with namespace such as connecting
// them to the correct ron.Server.
func (ns *Namespace) Start(target string) error {
	// For each VM, start it if it's in a startable state.
	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		// whether this is a reconnect for CC or not
		reconnect := true

		switch vm.GetState() {
		case VM_BUILDING:
			// always start building, first connect so reconnect=false
			reconnect = false

			// first launch, set affinity
			if ns.affinityEnabled {
				if err := ns.addAffinity(vm); err != nil {
					return true, err
				}
			}
		case VM_PAUSED:
			// always start paused
		case VM_QUIT, VM_ERROR:
			// only start quit or error when not wild
			if wild {
				return false, nil
			}
		case VM_RUNNING:
			// shouldn't start an already running vm
			if !wild {
				return true, errors.New("vm is already running")
			}

			return false, nil
		}

		if err := vm.Start(); err != nil {
			return true, err
		}

		if err := vm.Connect(ns.ccServer, reconnect); err != nil {
			log.Warn("unable to connect to cc for vm %v: %v", vm.GetID(), err)
		}

		return true, nil
	})
}

func (ns *Namespace) clearCCMount(s string) error {
	for uuid, mnt := range ns.ccMounts {
		switch s {
		case "", uuid, mnt.Name, mnt.Path:
			// match
		default:
			continue
		}

		if mnt.Path != "" {
			if err := syscall.Unmount(mnt.Path, 0); err != nil {
				return err
			}
		}

		vm := ns.VMs.FindVM(uuid)
		if vm == nil {
			// VM was mounted from remote host
			delete(ns.ccMounts, uuid)
			continue
		}

		// VM is running locally
		if err := ns.ccServer.DisconnectUFS(uuid); err != nil {
			return err
		}

		delete(ns.ccMounts, uuid)
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

		for prefix, r := range vlans.GetRanges() {
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
