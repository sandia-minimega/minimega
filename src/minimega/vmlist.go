// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"ranges"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// VMs contains all the VMs running on this host, the key is the VM's ID
type VMs map[int]VM

// vmApplyFunc is passed into VMs.apply
type vmApplyFunc func(VM, bool) (bool, error)

type Tag struct {
	ID         int
	Key, Value string
}

var vmLock sync.Mutex // lock for synchronizing access to vms

// Clone creates a snapshot of the currently running VMs. It should be safe to
// range over the returned value without holding the vmLock to perform
// read-only operations. Does *not* filter the returned VMs to just those in
// the active namespace.
func (vms VMs) Clone() VMs {
	vmLock.Lock()
	defer vmLock.Unlock()

	res := VMs{}
	for k, v := range res {
		res[k] = v
	}

	return res
}

// Count of VMs in current namespace.
func (vms VMs) Count() int {
	vmLock.Lock()
	defer vmLock.Unlock()

	i := 0

	for _, vm := range vms {
		if inNamespace(vm) {
			i += 1
		}
	}

	return i
}

// CountAll is Count, regardless of namespace.
func (vms VMs) CountAll() int {
	vmLock.Lock()
	defer vmLock.Unlock()

	return len(vms)
}

// Save the commands to configure the targeted VMs to file.
func (vms VMs) Save(file *os.File, target string) error {
	vmLock.Lock()
	defer vmLock.Unlock()

	// Stop on the first error
	var err error

	// For each VM, dump a list of commands to launch a VM of the same
	// configuration. Should not be run in parallel since we want to stop on
	// the first err.
	applyFunc := func(vm VM, _ bool) (bool, error) {
		if err != nil {
			return true, err
		}

		// build up the command list to re-launch this vm, first clear all
		// previous configuration.
		cmds := []string{"clear vm config"}

		cmds = append(cmds, saveConfig(baseConfigFns, vm.Config())...)

		switch vm := vm.(type) {
		case *KvmVM:
			cmds = append(cmds, saveConfig(kvmConfigFns, &vm.KVMConfig)...)
		case *ContainerVM:
			cmds = append(cmds, saveConfig(containerConfigFns, &vm.ContainerConfig)...)
		default:
		}

		// build the string to actually launch the VM
		arg := vm.GetName()
		if arg == "" {
			arg = "1"
		}
		cmds = append(cmds, fmt.Sprintf("vm launch %s %s", vm.GetType(), arg))

		// and a blank line
		cmds = append(cmds, "")

		// write commands to file
		for _, cmd := range cmds {
			if _, err = file.WriteString(cmd + "\n"); err != nil {
				return true, err
			}
		}

		return true, nil
	}

	vms.apply(target, false, applyFunc)

	return err
}

// Info populates resp with info about the VMs running in the active namespace.
func (vms VMs) Info(masks []string, resp *minicli.Response) {
	vmLock.Lock()
	defer vmLock.Unlock()

	resp.Header = masks
	res := VMs{} // for res.Data

	for _, vm := range vms {
		// Update dynamic fields before querying info
		vm.UpdateBW()

		res[vm.GetID()] = vm

		row := []string{}

		for _, mask := range masks {
			if v, err := vm.Info(mask); err != nil {
				// Field most likely not set for VM type
				row = append(row, "N/A")
			} else {
				row = append(row, v)
			}
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	resp.Data = res
}

func (vms VMs) SetTag(target, key, value string) {
	vmLock.Lock()
	defer vmLock.Unlock()

	// For each VM, set tag using key/value. Can be run in parallel.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		vm.SetTag(key, value)

		return true, nil
	}

	vms.apply(target, true, applyFunc)
}

func (vms VMs) GetTags(target, key string) []Tag {
	vmLock.Lock()
	defer vmLock.Unlock()

	res := []Tag{}

	// For each VM, start it if it's in a startable state. Cannot be run in parallel since
	// it aggregates results in res.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		if key == Wildcard {
			for k, v := range vm.GetTags() {
				res = append(res, Tag{
					Key:   k,
					Value: v,
					ID:    vm.GetID(),
				})
			}

			return true, nil
		}

		// TODO: return false if tag not set?
		res = append(res, Tag{
			Key:   key,
			Value: vm.Tag(key),
			ID:    vm.GetID(),
		})

		return true, nil
	}

	vms.apply(target, false, applyFunc)

	return res
}

func (vms VMs) ClearTags(target, key string) {
	vmLock.Lock()
	defer vmLock.Unlock()

	// For each VM, set tag using key/value. Can be run in parallel.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		vm.ClearTag(key)

		return true, nil
	}

	vms.apply(target, true, applyFunc)
}

// FindVM finds a VM in the active namespace based on its ID, name, or UUID.
func (vms VMs) FindVM(s string) VM {
	vmLock.Lock()
	defer vmLock.Unlock()

	return vms.findVM(s)
}

// findVM assumes vmLock is held.
func (vms VMs) findVM(s string) VM {
	if id, err := strconv.Atoi(s); err == nil {
		if vm := vms[id]; inNamespace(vm) {
			return vm
		}

		return nil
	}

	// Search for VM by name or UUID
	for _, vm := range vms {
		if !inNamespace(vm) {
			continue
		}

		if vm.GetName() == s || vm.GetUUID() == s {
			return vm
		}
	}

	return nil
}

// FindKvmVM finds a VM in the active namespace based on its ID, name, or UUID.
func (vms VMs) FindKvmVM(s string) (*KvmVM, error) {
	vmLock.Lock()
	defer vmLock.Unlock()

	return vms.findKvmVM(s)
}

// findKvmVm is FindKvmVM without locking vmLock.
func (vms VMs) findKvmVM(s string) (*KvmVM, error) {
	vm := vms.findVM(s)
	if vm == nil {
		return nil, vmNotFound(s)
	}

	if vm, ok := vm.(*KvmVM); ok {
		return vm, nil
	}

	return nil, vmNotKVM(s)
}

// FindKvmVMs finds all KvmVMs in the active namespace.
func (vms VMs) FindKvmVMs() []*KvmVM {
	vmLock.Lock()
	defer vmLock.Unlock()

	res := []*KvmVM{}

	for _, vm := range vms {
		if !inNamespace(vm) {
			continue
		}

		if vm, ok := vm.(*KvmVM); ok {
			res = append(res, vm)
		}
	}

	return res
}

func (vms VMs) Launch(names []string, vmType VMType) <-chan error {
	vmLock.Lock()

	out := make(chan error)

	log.Info("launching %v %v vms", len(names), vmType)
	start := time.Now()

	var wg sync.WaitGroup

	for _, name := range names {
		// This uses the global vmConfigs so we have to create the VMs in the
		// CLI thread (before the next command gets processed which could
		// change the vmConfigs).
		vm := NewVM(name, vmType)

		if err := vms.check(vm); err != nil {
			// Send from new goroutine to prevent deadlock since we haven't
			// even returned the output channel yet... hopefully we won't spawn
			// too many goroutines.
			wg.Add(1)
			go func() {
				defer wg.Done()

				out <- err
			}()
			continue
		}

		// Record newly created VM
		vms[vm.GetID()] = vm

		// The actual launching can happen in parallel, we just want to
		// make sure that we complete all the one-vs-all VM checks and add
		// to vms while holding the vmLock.
		wg.Add(1)
		go func(name string) {
			defer wg.Done()

			out <- vm.Launch()
		}(name)
	}

	go func() {
		// Don't unlock until we've finished launching all the VMs
		defer vmLock.Unlock()
		defer close(out)

		wg.Wait()

		stop := time.Now()
		log.Info("launched %v %v vms in %v", len(names), vmType, stop.Sub(start))
	}()

	return out
}

// check VM doesn't have any conflicts with the existing VMs
func (vms VMs) check(vm VM) error {
	// Make sure that there isn't an existing VM with the same name or UUID
	for _, vm2 := range vms {
		// We only care about name collisions if the VMs are running in the
		// same namespace or if the collision is a non-namespaced VM with an
		// already running namespaced VM.
		namesEq := vm.GetName() == vm2.GetName()
		uuidEq := vm.GetUUID() == vm2.GetUUID()
		namespaceEq := (vm.GetNamespace() == vm2.GetNamespace())

		if uuidEq && (namespaceEq || vm.GetNamespace() == "") {
			return fmt.Errorf("vm launch duplicate UUID: %s", vm.GetUUID())
		}

		if namesEq && (namespaceEq || vm.GetNamespace() == "") {
			return fmt.Errorf("vm launch duplicate VM name: %s", vm.GetName())
		}
	}

	// Check the interfaces/disks/filesystem is sane
	if err := vms.checkInterfaces(vm); err != nil {
		return err
	}

	switch vm := vm.(type) {
	case *KvmVM:
		return vms.checkDisks(vm)
	case *ContainerVM:
		return vms.checkFilesystem(vm)
	}

	return errors.New("unknown VM type")
}

// Start VMs matching target.
func (vms VMs) Start(target string) []error {
	vmLock.Lock()
	defer vmLock.Unlock()

	// For each VM, start it if it's in a startable state. Can be run in
	// parallel.
	applyFunc := func(vm VM, wild bool) (bool, error) {
		if wild && vm.GetState()&(VM_PAUSED|VM_BUILDING) != 0 {
			// If wild, we only start VMs in the building or running state
			return true, vm.Start()
		} else if !wild && vm.GetState()&VM_RUNNING == 0 {
			// If not wild, start VMs that aren't already running
			return true, vm.Start()
		}

		return false, nil
	}

	return vms.apply(target, true, applyFunc)
}

// Stop VMs matching target.
func (vms VMs) Stop(target string) []error {
	vmLock.Lock()
	defer vmLock.Unlock()

	// For each VM, stop it if it's running. Can be run in parallel.
	applyFunc := func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&VM_RUNNING != 0 {
			return true, vm.Stop()
		}

		return false, nil
	}

	return vms.apply(target, true, applyFunc)
}

// Kill VMs matching target.
func (vms VMs) Kill(target string) []error {
	vmLock.Lock()
	defer vmLock.Unlock()

	killedVms := map[int]bool{}

	// For each VM, kill it if it's in a killable state. Should not be run in
	// parallel because we record the IDs of the VMs we kill in killedVms.
	applyFunc := func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&VM_KILLABLE == 0 {
			return false, nil
		}

		if err := vm.Kill(); err != nil {
			log.Error("unleash the zombie VM: %v", err)
		} else {
			killedVms[vm.GetID()] = true
		}
		return true, nil
	}

	errs := vms.apply(target, false, applyFunc)

outer:
	for len(killedVms) > 0 {
		select {
		case id := <-killAck:
			log.Info("VM %v killed", id)
			delete(killedVms, id)
		case <-time.After(COMMAND_TIMEOUT * time.Second):
			log.Error("vm kill timeout")
			break outer
		}
	}

	for id := range killedVms {
		log.Info("VM %d failed to acknowledge kill", id)
	}

	return errs
}

// Flush deletes VMs that are in the QUIT or ERROR state.
func (vms VMs) Flush() {
	vmLock.Lock()
	defer vmLock.Unlock()

	for i, vm := range vms {
		// Skip VMs outside of current namespace
		if !inNamespace(vm) {
			continue
		}

		if vm.GetState()&(VM_QUIT|VM_ERROR) != 0 {
			log.Info("deleting VM: %v", i)

			if err := vm.Flush(); err != nil {
				log.Error("clogged VM: %v", err)
			}

			delete(vms, i)
		}
	}
}

// CleanDirs removes all instance directories in the minimega base directory
func (vms VMs) CleanDirs() {
	vmLock.Lock()
	defer vmLock.Unlock()

	log.Debugln("cleanDirs")
	for _, vm := range vms {
		path := vm.GetInstancePath()
		log.Debug("cleaning instance path: %v", path)
		err := os.RemoveAll(path)
		if err != nil {
			log.Error("clearDirs: %v", err)
		}
	}
}

// apply is the fan out/in method to apply a function to a set of VMs specified
// by target. Specifically, it:
//
// 	1. Expands target to a list of VM names and IDs (or wild)
// 	2. Invokes fn on all the matching VMs
// 	3. Collects all the errors from the invoked fns
// 	4. Records in the log a list of VMs that were not found
//
// The fn that is passed in takes two arguments: the VM struct and a boolean
// specifying whether the invocation was wild or not. The fn returns a boolean
// that indicates whether the target was applicable (e.g. calling start on an
// already running VM would not be applicable) and an error.
//
// The concurrent boolean controls whether fn is run concurrently on multiple
// VMs or not. If the fns alter state they can set this flag to false rather
// than dealing with locking.
func (vms VMs) apply(target string, concurrent bool, fn vmApplyFunc) []error {
	// Some callstack voodoo magic
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			log.Debug("applying %v to %v (concurrent = %t)", fn.Name(), target, concurrent)
		}
	}

	names := map[string]bool{} // Names of VMs for which to apply fn
	ids := map[int]bool{}      // IDs of VMs for which to apply fn

	vals, err := ranges.SplitList(target)
	if err != nil {
		return []error{err}
	}
	for _, v := range vals {
		id, err := strconv.Atoi(v)
		if err == nil {
			ids[id] = true
		} else {
			names[v] = true
		}
	}
	wild := hasWildcard(names)
	delete(names, Wildcard)

	// wg determine when it's okay to close errChan
	var wg sync.WaitGroup
	errChan := make(chan error)

	// lock prevents concurrent writes to results
	var lock sync.Mutex
	results := map[string]bool{}

	// Wrap function with magic
	magicFn := func(vm VM) {
		defer wg.Done()
		ok, err := fn(vm, wild)
		if err != nil {
			errChan <- err
		}

		lock.Lock()
		defer lock.Unlock()
		results[vm.GetName()] = ok
		results[strconv.Itoa(vm.GetID())] = ok
	}

	for _, vm := range vms {
		if !inNamespace(vm) {
			continue
		}

		if wild || names[vm.GetName()] || ids[vm.GetID()] {
			delete(names, vm.GetName())
			delete(ids, vm.GetID())
			wg.Add(1)

			// Use concurrency only if requested
			if concurrent {
				go magicFn(vm)
			} else {
				magicFn(vm)
			}
		}
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	var errs []error

	for err := range errChan {
		errs = append(errs, err)
	}

	// Special cases: specified one VM and
	//   1. it wasn't found
	//   2. it wasn't a valid target (e.g. start already running VM)
	if len(vals) == 1 && !wild {
		if (len(names) + len(ids)) == 1 {
			errs = append(errs, vmNotFound(vals[0]))
		} else if !results[vals[0]] {
			errs = append(errs, fmt.Errorf("VM state error: %v", vals[0]))
		}
	}

	// Log the names/ids of the vms that weren't found
	if (len(names) + len(ids)) > 0 {
		vals := []string{}
		for v := range names {
			vals = append(vals, v)
		}
		for v := range ids {
			vals = append(vals, strconv.Itoa(v))
		}
		log.Info("VMs not found: %v", vals)
	}

	return errs
}

// checkInterfaces checks to make sure that a VM's MAC addresses are unique.
// Only returns an error if the MAC addresses are not unique for the interfaces
// on the same VM. If multiple VMs share the same MAC address, logs a warning.
// If a VM's MAC address is empty for a given interface, it randomly assigns a
// valid, unique, MAC to that interface.
func (vms VMs) checkInterfaces(vm VM) error {
	macs := map[string]bool{}

	for _, net := range vm.Config().Networks {
		// Skip unassigned MACs
		if net.MAC == "" {
			continue
		}

		// Check if the VM already has this MAC for one of its interfaces
		if _, ok := macs[net.MAC]; ok {
			return fmt.Errorf("VM has same MAC for more than one interface -- %s", net.MAC)
		}

		macs[net.MAC] = true
	}

	for _, vm2 := range vms {
		// Skip ourself
		if vm.GetID() == vm2.GetID() {
			continue
		}

		for _, net := range vm2.Config().Networks {
			// VM must still be in the pre-building stage so it hasn't been
			// assigned a MAC yet. We skip this case in order to supress
			// duplicate MAC errors on an empty string.
			if net.MAC == "" {
				continue
			}

			// Warn if we see a conflict
			if _, ok := macs[net.MAC]; ok {
				log.Warn("VMs share MAC (%v) -- %v %v", net.MAC, vm.GetID(), vm2.GetID())
			}

			macs[net.MAC] = true
		}
	}

	// Get a handle to the VM's actual Networks so that we can update them
	var networks []NetConfig
	switch vm := vm.(type) {
	case *KvmVM:
		networks = vm.Networks
	case *ContainerVM:
		networks = vm.Networks
	default:
		return errors.New("unable to CheckInterfaces for unknown VM type")
	}

	// Find any unassigned MACs and randomly generate a MAC for them
	for i := range networks {
		n := &networks[i]

		if n.MAC != "" {
			continue
		}

		// Loop until we don't have a conflict
		for exists := true; exists; _, exists = macs[n.MAC] {
			n.MAC = randomMac()
		}

		macs[n.MAC] = true
	}

	return nil
}

// checkDisks looks for Kvm VMs that share the same disk image and don't have
// Snapshot set to true.
func (vms VMs) checkDisks(vm *KvmVM) error {
	// Disk path to whether it is a snapshot or not
	disks := map[string]bool{}

	// Record which disks are in use and whether they are being used as a
	// snapshot or not by other VMs. If the same disk happens to be in use by
	// different VMs and they have mismatched snapshot flags, assume that the
	// disk is not being used in snapshot mode.
	for _, vm2 := range vms {
		// Skip ourself
		if vm == vm2 {
			continue
		}

		if vm2, ok := vm2.(*KvmVM); ok {
			for _, disk := range vm2.DiskPaths {
				disks[disk] = vm2.Snapshot || disks[disk]
			}
		}
	}

	// Check our disks to see if we're trying to use a disk that is in use by
	// another VM (unless both are being used in snapshot mode).
	for _, disk := range vm.DiskPaths {
		if snapshot, ok := disks[disk]; ok && (snapshot != vm.Snapshot) {
			return fmt.Errorf("disk path %v is already in use by another vm", disk)
		}
	}

	return nil
}

// checkFilesystem looks for Container VMs that share the same filesystem
// directory and don't have Snapshot set to true.
func (vms VMs) checkFilesystem(vm *ContainerVM) error {
	if vm.FSPath == "" {
		return errors.New("unable to launch container without a configured filesystem")
	}

	// Disk path to whether it is a snapshot or not
	disks := map[string]bool{}

	// Record which disks are in use and whether they are being used as a
	// snapshot or not by other VMs. If the same disk happens to be in use by
	// different VMs and they have mismatched snapshot flags, assume that the
	// disk is not being used in snapshot mode.
	for _, vm2 := range vms {
		// Skip ourself
		if vm == vm2 { // ignore this vm
			continue
		}

		if vm2, ok := vm2.(*ContainerVM); ok {
			disks[vm2.FSPath] = vm2.Snapshot
		}
	}

	// Check our disk to see if we're trying to use a disk that is in use by
	// another VM (unless both are being used in snapshot mode).
	if snapshot, ok := disks[vm.FSPath]; ok && (snapshot != vm.Snapshot) {
		return fmt.Errorf("disk path %v is already in use by another vm", vm.FSPath)
	}

	return nil
}

// HostVMs gets all the VMs running on the specified remote host, filtered to
// the current namespace, if applicable.
func HostVMs(host string) VMs {
	cmdLock.Lock()
	defer cmdLock.Unlock()

	return hostVMs(host)
}

// hostVMs is HostVMs without locking cmdLock.
func hostVMs(host string) VMs {
	// Compile info command and set it not to record
	cmd := minicli.MustCompile("vm info")
	cmd.SetRecord(false)
	cmd.SetSource(GetNamespaceName())

	cmds := makeCommandHosts([]string{host}, cmd)

	var vms VMs

	// LOCK: see func description.
	for resps := range runCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if vms2, ok := resp.Data.(VMs); ok {
				if vms != nil {
					// odd... should only be one vms per host and we're
					// querying a single host
					log.Warn("so many vms")
				}
				vms = vms2
			}
		}
	}

	return vms
}

// GlobalVMs gets the VMs from all hosts in the mesh, filtered to the current
// namespace, if applicable. The keys of the returned map do not match the VM's
// ID.
func GlobalVMs() VMs {
	cmdLock.Lock()
	defer cmdLock.Unlock()

	return globalVMs()
}

// globalVMs is GlobalVMs without locking cmdLock.
func globalVMs() VMs {
	// Compile info command and set it not to record
	cmd := minicli.MustCompile("vm info")
	cmd.SetRecord(false)
	cmd.SetSource(GetNamespaceName())

	// Figure out which hosts to query:
	//  * Hosts in the active namespace
	//  * Hosts connected via meshage plus ourselves
	var hosts []string
	if ns := GetNamespace(); ns != nil {
		hosts = ns.hostSlice()
	} else {
		hosts = meshageNode.BroadcastRecipients()
		hosts = append(hosts, hostname)
	}

	log.Info("globalVMs command: %#v", cmd)

	cmds := makeCommandHosts(hosts, cmd)

	// Collected VMs
	vms := VMs{}

	// LOCK: see func description.
	for resps := range runCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if vms2, ok := resp.Data.(VMs); ok {
				for _, vm := range vms2 {
					vms[len(vms)] = vm
				}
			} else {
				log.Error("unknown data field in vm info from %v", resp.Host)
			}
		}
	}

	return vms
}

// ExpandLaunchNames takes a VM name, range, or count and expands it into a
// list of names of VMs that should be launched. Does several sanity checks on
// the names to make sure that they aren't reserved words and don't collide
// with any existing VMs (as supplied via the vms argument).
func ExpandLaunchNames(arg string, vms VMs) ([]string, error) {
	vmLock.Lock()
	defer vmLock.Unlock()

	return expandLaunchNames(arg, vms)
}

// expandLaunchNames is ExpandLaunchNames without locking vmLock.
func expandLaunchNames(arg string, vms VMs) ([]string, error) {
	names := []string{}

	count, err := strconv.ParseInt(arg, 10, 32)
	if err != nil {
		names, err = ranges.SplitList(arg)
	} else if count <= 0 {
		err = errors.New("invalid number of vms (must be > 0)")
	} else {
		names = make([]string, count)
	}

	if err != nil {
		return nil, err
	}

	if len(names) == 0 {
		return nil, errors.New("no VMs to launch")
	}

	for _, name := range names {
		if isReserved(name) {
			return nil, fmt.Errorf("invalid vm name, `%s` is a reserved word", name)
		}

		if _, err := strconv.Atoi(name); err == nil {
			return nil, fmt.Errorf("invalid vm name, `%s` is an integer", name)
		}

		for _, vm := range vms {
			if !inNamespace(vm) {
				continue
			}

			if vm.GetName() == name {
				return nil, fmt.Errorf("vm already exists with name `%s`", name)
			}
		}
	}

	return names, nil
}
