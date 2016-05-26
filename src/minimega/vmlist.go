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

		err = vm.SaveConfig(file)
		return true, err
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
		vm, err := NewVM(name, vmType)
		if err == nil {
			for _, vm2 := range vms {
				if err = vm2.Conflicts(vm); err != nil {
					break
				}
			}
		}

		if err != nil {
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
