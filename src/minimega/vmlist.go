// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"ranges"
	"strconv"
	"sync"
	"time"
)

// VMs contains all the VMs running on this host, the key is the VM's ID
type VMs map[int]VM

// vmApplyFunc is passed into VMs.apply
type vmApplyFunc func(VM, bool) (bool, error)

// namespace returns the VMs that are part of the currently active namespace,
// if there is one. Otherwise, returns itself.
func (vms VMs) namespace() VMs {
	if namespace == "" {
		return vms
	}

	vmLock.Lock()
	defer vmLock.Unlock()

	res := map[int]VM{}
	for id, vm := range vms {
		if vm.GetNamespace() == namespace {
			res[id] = vm
		}
	}

	return res
}

func (vms VMs) save(file *os.File, target string) error {
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

		arg := vm.GetName()
		if arg == "" {
			arg = "1"
		}
		cmds = append(cmds, fmt.Sprintf("vm launch %v %v", vm.GetType(), arg))

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

func (vms VMs) qmp(idOrName, qmp string) (string, error) {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return "", vmNotFound(idOrName)
	}

	if vm, ok := vm.(*KvmVM); ok {
		return vm.QMPRaw(qmp)
	}

	return "", vmNotKVM(idOrName)
}

func (vms VMs) screenshot(idOrName string, max int) ([]byte, error) {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return nil, vmNotFound(idOrName)
	}

	if vm, ok := vm.(*KvmVM); ok {
		return vm.Screenshot(max)
	}

	return nil, vmNotPhotogenic(idOrName)
}

func (vms VMs) migrate(idOrName, filename string) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}

	if vm, ok := vm.(*KvmVM); ok {
		return vm.Migrate(filename)
	}

	return vmNotKVM(idOrName)
}

// findVm finds a VM based on it's ID, name, or UUID. Returns nil if no such VM
// exists.
func (vms VMs) findVm(s string) VM {
	if id, err := strconv.Atoi(s); err == nil {
		return vms[id]
	}

	// Search for VM by name or UUID
	for _, v := range vms {
		if v.GetName() == s {
			return v
		}
		if v.GetUUID() == s {
			return v
		}
	}

	return nil
}

// launch one VM of a given type. This needs to be called without VMs.namespace
// as we need to add the VM to the global VMs.
func (vms VMs) launch(vm VM) (err error) {
	// Actually launch the VM from a defered func when there's no error. This
	// happens *after* we've released the vmLock so that launching can happen
	// in parallel.
	defer func() {
		if err == nil {
			err = vm.Launch()
		}
	}()

	vmLock.Lock()
	defer vmLock.Unlock()

	// Make sure that there isn't an existing VM with the same name
	for _, vm2 := range vms {
		// We only care about name collisions if the VMs are running in the
		// same namespace or if the collision is a non-namespaced VM with an
		// already running namespaced VM.
		namesEq := vm.GetName() == vm2.GetName()
		namespaceEq := (vm.GetNamespace() == vm2.GetNamespace())

		if namesEq && (namespaceEq || vm.GetNamespace() == "") {
			return fmt.Errorf("vm launch duplicate VM name: %s", vm.GetName())
		}
	}

	vms[vm.GetID()] = vm
	return
}

func (vms VMs) start(target string) []error {
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

func (vms VMs) stop(target string) []error {
	// For each VM, stop it if it's running. Can be run in parallel.
	applyFunc := func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&VM_RUNNING != 0 {
			return true, vm.Stop()
		}

		return false, nil
	}

	return vms.apply(target, true, applyFunc)
}

func (vms VMs) kill(target string) []error {
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

// flush deletes VMs that are in the QUIT or ERROR state. This needs to be
// called without VMs.namespace as we need to delete VMs from the global VMs.
func (vms VMs) flush() {
	vmLock.Lock()
	defer vmLock.Unlock()

	for i, vm := range vms {
		// Skip VMs outside of current namespace
		if namespace != "" && vm.GetNamespace() != namespace {
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

func (vms VMs) info() ([]string, [][]string, error) {
	table := [][]string{}

	for _, vm := range vms {
		row := []string{}

		for _, mask := range vmMasks {
			if v, err := vm.Info(mask); err != nil {
				// Field not set for VM type, replace with placeholder
				row = append(row, "N/A")
			} else {
				row = append(row, v)
			}
		}

		table = append(table, row)
	}

	return vmMasks, table, nil
}

// cleanDirs removes all isntance directories in the minimega base directory
func (vms VMs) cleanDirs() {
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

// LocalVMs gets all the VMs running on the local host, filtered to the current
// namespace, if applicable.
func LocalVMs() VMs {
	return vms.namespace()
}

// HostVMs gets all the VMs running on the specified remote host, filtered to
// the current namespace, if applicable.
func HostVMs(host string) VMs {
	// Compile info command and set it not to record
	cmd := minicli.MustCompile("vm info")
	cmd.SetRecord(false)

	cmds := makeCommandHosts([]string{host}, cmd)

	var vms VMs

	for resps := range processCommands(cmds...) {
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
// namespace, if applicable. Unlike LocalVMs, the keys of the returned map do
// not match the VM's ID. Caller should hold cmdLock.
func GlobalVMs() VMs {
	// Figure out which hosts to query:
	//  * Hosts in the active namespace
	//  * Hosts connected via meshage plus ourselves
	var hosts []string
	if namespace != "" {
		hosts = namespaces[namespace].hostSlice()
	} else {
		hosts = meshageNode.BroadcastRecipients()
		hosts = append(hosts, hostname)
	}

	// Compile info command and set it not to record
	cmd := minicli.MustCompile("vm info")
	cmd.SetRecord(false)

	cmds := makeCommandHosts(hosts, cmd)

	// Collected VMs
	vms := VMs{}

	for resps := range processCommands(cmds...) {
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
