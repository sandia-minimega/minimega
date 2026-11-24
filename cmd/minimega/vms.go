// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/ranges"
)

// VMs contains all the VMs running on this host, the key is the VM's ID
type VMs struct {
	m  map[int]VM
	mu sync.Mutex

	// stopVNC, if set, stops the VNC recordings, playback, and pastes of the
	// named VM. A KVM VM calls it when QEMU exits.
	stopVNC func(name string)

	// pending holds the VMs that Launch has reserved and is launching, keyed
	// by ID. Each one moves to m when its Launch returns.
	pending     map[int]VM
	pendingNICs int
	launched    *sync.Cond // broadcast when a VM leaves pending
}

// newVM creates the VMs that Launch launches. Tests replace it.
var newVM = NewVM

// vmApplyFunc is passed into VMs.apply
type vmApplyFunc func(VM, bool) (bool, error)

type Tag struct {
	ID         int
	Key, Value string
}

// QueuedVMs stores all the info needed to launch a batch of VMs
type QueuedVMs struct {
	Names    []string
	VMType   // embed
	VMConfig // embed

	// book keeping for scheduler

	// counts for colocated VMs, indexed by name
	colocatedCounts map[string]int
	// sum of colocatedCounts, used for sorting
	colocatedCount int
}

// GetFiles looks through the VMConfig for files in the IOMESHAGE directory and
// fetches them if they do not already exist. Currently, we enumerate all the
// fields that take a file. If updatee is provided, it will periodically be sent
// status update messages about file transfer status.
func (q QueuedVMs) GetFiles(updatee string) error {
	for _, f := range q.files() {
		if strings.HasPrefix(f, *f_iomBase) {
			if _, err := iomHelper(f, updatee); err != nil {
				return err
			}
		}
	}

	return nil
}

// files returns the paths of the files that the VMs in q use.
func (q QueuedVMs) files() []string {
	files := []string{
		q.ContainerConfig.Preinit,
		q.KVMConfig.CdromPath,
		q.KVMConfig.InitrdPath,
		q.KVMConfig.KernelPath,
		q.KVMConfig.MigratePath,
	}
	for _, f := range q.KVMConfig.Disks {
		files = append(files, f.Path)
	}
	if p := q.FirecrackerConfig.FirecrackerSnapshotPath; p != "" && q.VMType == FIRECRACKER {
		p = fcSnapshotPrefix(p)
		files = append(files, p+".json", p+".state", p+".mem")
		if size, _ := fcParseSize(q.FirecrackerConfig.FirecrackerScratch); size > 0 {
			files = append(files, p+fcSaveSuffix(0, true))
		}
	}

	return files
}

// Count of launched VMs, including the VMs that are launching.
func (vms *VMs) Count() int {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	return len(vms.m) + len(vms.pending)
}

func (vms *VMs) CountTypeState(t VMType, mask VMState) int {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	var count int

	for _, vm := range vms.m {
		if vm.GetType() == t && vm.GetState()&mask != 0 {
			count++
		}
	}

	// A launching VM holds its lock, so do not call GetState on it.
	if mask&VM_BUILDING != 0 {
		for _, vm := range vms.pending {
			if vm.GetType() == t {
				count++
			}
		}
	}

	return count
}

// Limit is the lowest coschedule value for VMs (-1 is no limit)
func (vms *VMs) Limit() int {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	// assume unlimited
	limit := -1

	update := func(vm VM) {
		// update if limit is unlimited or we're not unlimited and we're less
		// than the previous limit
		v := vm.GetCoschedule()
		if limit == -1 || (v != -1 && v < limit) {
			limit = v
		}
	}

	for _, vm := range vms.m {
		update(vm)
	}
	for _, vm := range vms.pending {
		update(vm)
	}

	return limit
}

// Returns the total cpu, memory, and network commit across all VMs.
func (vms *VMs) Commit() (uint64, uint64, int) {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	cpu := uint64(0)
	mem := uint64(0)
	net := vms.pendingNICs

	for _, vm := range vms.m {
		cpu += vm.GetCPUs()
		mem += vm.GetMem()
		net += len(vm.GetNetworks())
	}

	for _, vm := range vms.pending {
		cpu += vm.GetCPUs()
		mem += vm.GetMem()
	}

	return cpu, mem, net
}

// Info populates resp with info about launched VMs.
func (vms *VMs) Info(masks []string, resp *minicli.Response) {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	resp.Header = masks
	// for resp.Data
	res := []VM{}

	for _, vm := range vms.m {
		// Update dynamic fields before querying info
		vm.UpdateNetworks()

		// Copy the VM and use the copy from here on. This ensures that the
		// Tabular info matches the Data field.
		vm := vm.Copy()

		res = append(res, vm)

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

// FindVM finds a VM based on its ID, name, or UUID.
func (vms *VMs) FindVM(s string) VM {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	return vms.findVM(s)
}

// findVM assumes vms.mu is held.
func (vms *VMs) findVM(s string) VM {
	if id, err := strconv.Atoi(s); err == nil {
		if vm, ok := vms.m[id]; ok {
			return vm
		}

		return nil
	}

	// Search for VM by name or UUID
	for _, vm := range vms.m {
		if vm.GetName() == s || vm.GetUUID() == s {
			return vm
		}
	}

	return nil
}

// FindContainerVM finds a VM based on its ID, name, or UUID.
func (vms *VMs) FindContainerVM(s string) (*ContainerVM, error) {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	return vms.findContainerVM(s)
}

// findContainerVM assumes vms.mu is held.
func (vms *VMs) findContainerVM(s string) (*ContainerVM, error) {
	vm := vms.findVM(s)
	if vm == nil {
		return nil, vmNotFound(s)
	}

	if vm, ok := vm.(*ContainerVM); ok {
		return vm, nil
	}

	return nil, vmNotContainer(s)
}

// FindKvmVM finds a VM based on its ID, name, or UUID.
func (vms *VMs) FindKvmVM(s string) (*KvmVM, error) {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	return vms.findKvmVM(s)
}

// findKvmVm assumesvms.mu is held.
func (vms *VMs) findKvmVM(s string) (*KvmVM, error) {
	vm := vms.findVM(s)
	if vm == nil {
		return nil, vmNotFound(s)
	}

	if vm, ok := vm.(*KvmVM); ok {
		return vm, nil
	}

	return nil, vmNotKVM(s)
}

// FindKvmVMs finds all KvmVMs.
func (vms *VMs) FindKvmVMs() []*KvmVM {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	res := []*KvmVM{}

	for _, vm := range vms.m {
		if vm, ok := vm.(*KvmVM); ok {
			res = append(res, vm)
		}
	}

	return res
}

// Launch takes QueuedVMs and launches them after performing a few sanity
// checks. Launch returns any errors that occur via a channel since it launches
// VMs asynchronously.
func (vms *VMs) Launch(requestor, namespace string, q *QueuedVMs) <-chan error {
	errs := make(chan error)

	go func() {
		defer close(errs)

		// prefetch any files associated with VMs
		if err := q.GetFiles(requestor); err != nil {
			errs <- launchBatchError{err}
			return
		}

		log.Info("launching %v %v vms", len(q.Names), q.VMType)
		start := time.Now()

		nics := len(q.Networks)

		// Hold vms.mu only to create the VMs and to reserve their names and
		// IDs. The launches run without it.
		var reserved []VM
		var failed []error

		vms.mu.Lock()
		for _, name := range q.Names {
			// Create new VM and test it for conflicts against other VMs.
			vm, err := newVM(name, namespace, q.VMType, q.VMConfig)
			if err == nil {
				err = vms.conflicts(vm)
			}

			if err != nil {
				failed = append(failed, err)
				continue
			}

			// Reserve the VM so that it gets included in future conflict tests.
			if vms.pending == nil {
				vms.pending = map[int]VM{}
			}
			vms.pending[vm.GetID()] = vm
			vms.pendingNICs += nics
			vms.hookExit(vm)

			reserved = append(reserved, vm)
		}
		vms.mu.Unlock()

		for _, err := range failed {
			errs <- err
		}

		var wg sync.WaitGroup
		var fcBatch fcLaunchBatch

		for _, vm := range reserved {
			wg.Add(1)
			go func(vm VM) {
				defer wg.Done()
				defer vms.finishLaunch(vm, nics)

				if err := vm.Launch(); err != nil {
					errs <- err
					return
				}

				if err := writeVMConfig(vm); err != nil {
					errs <- err
					return
				}

				if vm.GetType() == FIRECRACKER {
					fcBatch.add(vm)
				}
			}(vm)
		}

		wg.Wait()

		if fcBatch.len() > 0 {
			log.Info("%v", fcBatch.message(time.Since(start)))
		}

		stop := time.Now()
		log.Info("launched %v %v vms in %v", len(q.Names), q.VMType, stop.Sub(start))
	}()

	return errs
}

// conflicts tests vm for conflicts against the launched and the launching
// VMs. Assumes vms.mu is held.
func (vms *VMs) conflicts(vm VM) error {
	for _, vm2 := range vms.m {
		if err := vm2.Conflicts(vm); err != nil {
			return err
		}
	}

	// The launch of a pending VM holds its lock, and the launch does not
	// change the fields that the conflict test reads.
	for _, vm2 := range vms.pending {
		if err := vm2.conflictsNoLock(vm); err != nil {
			return err
		}
	}

	return nil
}

// finishLaunch moves vm from pending to the launched VMs, whether or not its
// Launch succeeded.
func (vms *VMs) finishLaunch(vm VM, nics int) {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	delete(vms.pending, vm.GetID())
	vms.pendingNICs -= nics
	vms.m[vm.GetID()] = vm

	vms.launchedCond().Broadcast()
}

// launchedCond returns the condition that finishLaunch broadcasts. Assumes
// vms.mu is held.
func (vms *VMs) launchedCond() *sync.Cond {
	if vms.launched == nil {
		vms.launched = sync.NewCond(&vms.mu)
	}

	return vms.launched
}

// waitLaunches waits until no VM is launching. Assumes vms.mu is held.
func (vms *VMs) waitLaunches() {
	for len(vms.pending) > 0 {
		vms.launchedCond().Wait()
	}
}

// WaitLaunches waits until the VMs that are launching are in the VMs map.
func (vms *VMs) WaitLaunches() {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	vms.waitLaunches()
}

// Stop VMs matching target.
func (vms *VMs) Stop(target string) error {
	return vms.Apply(target, func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&VM_RUNNING != 0 {
			return true, vm.Stop()
		}

		return false, nil
	})
}

// Kill VMs matching target
func (vms *VMs) Kill(target string) error {
	return vms.Apply(target, func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&VM_KILLABLE == 0 {
			return false, nil
		}

		if err := vm.Kill(); err != nil {
			log.Error("unleash the zombie VM: %v", err)
			return true, err
		}

		return true, nil
	})
}

// hookExit makes a KVM VM stop its VNC recordings, playback, and pastes when
// QEMU exits. Call it for each VM that Launch creates or recovery finds.
func (vms *VMs) hookExit(vm VM) {
	if kvm, ok := vm.(*KvmVM); ok {
		kvm.onExit = &exitHook{fn: vms.stopVNC}
	}
}

// stopFlushedVNC stops the VNC recordings, playback, and pastes of a flushed
// KVM VM. It also removes the exit hook of the VM, so that a later QEMU exit
// cannot stop the VNC work of a new VM with the same name.
func (vms *VMs) stopFlushedVNC(vm VM) {
	kvm, ok := vm.(*KvmVM)
	if !ok {
		return
	}

	kvm.onExit.detach()
	if vms.stopVNC != nil {
		vms.stopVNC(kvm.GetName())
	}
}

// Flush VMs matching target.
func (vms *VMs) Flush(target string, cc *ron.Server) error {
	var mapLock sync.Mutex
	var flushed []int

	vms.mu.Lock()
	errs := vms.apply(target, func(vm VM, _ bool) (bool, error) {
		if vm.GetState()&(VM_QUIT|VM_ERROR) == 0 {
			return false, nil
		}

		log.Info("deleting VM: %v", vm.GetID())

		if err := vm.Disconnect(cc); err != nil {
			log.Error("unable to disconnect to cc for vm %v: %v", vm.GetID(), err)
			return true, err
		}

		if err := vm.Flush(); err != nil {
			log.Error("clogged vm %v: %v", vm.GetID(), err)
			return true, err
		}

		vms.stopFlushedVNC(vm)

		mapLock.Lock()
		defer mapLock.Unlock()

		flushed = append(flushed, vm.GetID())

		return true, nil
	})

	// apply iterates over vms.m, so delete after it returns
	for _, id := range flushed {
		delete(vms.m, id)
	}
	vms.mu.Unlock()

	return makeErrSlice(errs)
}

// FlushAll deletes VMs that are in the QUIT or ERROR state, disconnecting them
// from the provided ron.Server first. It waits for running launches first, so
// that it also deletes the VMs whose launch failed.
func (vms *VMs) FlushAll(cc *ron.Server) error {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	vms.waitLaunches()

	var wg sync.WaitGroup
	var mapLock sync.Mutex
	var flushed []int

	for i, vm := range vms.m {
		if vm.GetState()&(VM_QUIT|VM_ERROR) != 0 {
			wg.Add(1)

			go func(i int, vm VM) {
				log.Info("deleting VM: %v", i)

				if err := vm.Disconnect(cc); err != nil {
					log.Error("unable to disconnect to cc for vm %v: %v", vm.GetID(), err)
				}

				if err := vm.Flush(); err != nil {
					log.Error("clogged vm %v: %v", vm.GetID(), err)
				}

				vms.stopFlushedVNC(vm)

				mapLock.Lock()
				defer mapLock.Unlock()
				flushed = append(flushed, i)
				wg.Done()
			}(i, vm)
		}
	}
	wg.Wait()

	for _, i := range flushed {
		delete(vms.m, i)
	}

	return nil
}

func (vms *VMs) ProcStats(d time.Duration) []*VMProcStats {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	var res []*VMProcStats

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, vm := range vms.m {
		wg.Add(1)

		go func(vm VM) {
			defer wg.Done()

			var err error

			p := &VMProcStats{
				Name: vm.GetName(),
			}

			p.A, err = vm.ProcStats()
			if err != nil {
				log.Error("failed to get process stats for %v: %v", vm.GetID(), err)
				return
			}

			time.Sleep(d)

			p.B, err = vm.ProcStats()
			if err != nil {
				log.Error("failed to get process stats for %v: %v", vm.GetID(), err)
				return
			}

			// Update dynamic fields before querying info
			vm.UpdateNetworks()
			for _, nic := range vm.GetNetworks() {
				p.RxRate += nic.RxRate
				p.TxRate += nic.TxRate
			}

			mu.Lock()
			defer mu.Unlock()
			res = append(res, p)
		}(vm)
	}

	wg.Wait()

	return res
}

// Apply fn to VMs, wrapping apply, with proper locking. Collapses error slice
// into single error.
func (vms *VMs) Apply(target string, fn vmApplyFunc) error {
	vms.mu.Lock()
	defer vms.mu.Unlock()

	return makeErrSlice(vms.apply(target, fn))
}

// apply is the fan-out/fan-in method to apply a function to a set of VMs
// specified by target. Specifically, it:
//
//  1. Expands target to a list of VM names and IDs (or wild)
//  2. Invokes fn on all the matching VMs
//  3. Collects all the errors from the invoked fns
//  4. Records in the log a list of VMs that were not found
//
// The fn that is passed in takes two arguments: the VM struct and a boolean
// specifying whether the invocation was wild or not. The fn returns a boolean
// that indicates whether the target was applicable (e.g. calling start on an
// already running VM would not be applicable) and an error.
func (vms *VMs) apply(target string, fn vmApplyFunc) []error {
	// Some callstack voodoo magic
	if pc, _, _, ok := runtime.Caller(1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			log.Debug("applying %v to %v", fn.Name(), target)
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

	for _, vm := range vms.m {
		if wild || names[vm.GetName()] || ids[vm.GetID()] {
			delete(names, vm.GetName())
			delete(ids, vm.GetID())
			wg.Add(1)

			go magicFn(vm)
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

// meshageVMLauncher handles VM launches sent by the scheduler
func meshageVMLauncher() {
	for m := range meshageVMLaunchChan {
		go func(m *meshage.Message) {
			cmd := m.Body.(meshageVMLaunch)

			ns := GetOrCreateNamespace(cmd.Namespace)

			launchErrs := ns.Launch(cmd.From, cmd.QueuedVMs)

			errs := []string{}
			for _, err := range launchErrs {
				errs = append(errs, err.Error())
			}

			to := []string{m.Source}
			msg := meshageVMResponse{
				Errors:   errs,
				Launched: launchedCount(len(cmd.QueuedVMs.Names), launchErrs),
				TID:      cmd.TID,
			}

			if _, err := meshageNode.Set(to, msg); err != nil {
				log.Errorln(err)
			}
		}(m)
	}
}

// launchBatchError is an error that stops the launch of all the VMs of a
// queued entry.
type launchBatchError struct {
	err error
}

func (e launchBatchError) Error() string { return e.err.Error() }
func (e launchBatchError) Unwrap() error { return e.err }

// launchedCount returns the number of VMs of a queued entry with total VMs that
// launched, from the errors of the launch.
func launchedCount(total int, errs []error) int {
	for _, err := range errs {
		if errors.As(err, new(launchBatchError)) {
			return 0
		}
	}

	return max(total-len(errs), 0)
}

// globalVMs gets the VMs from all hosts in the mesh, filtered to the current
// namespace, if applicable. The keys of the returned map do not match the VM's
// ID.
func globalVMs(ns *Namespace) []VM {
	// run `vm info` across the namespace
	cmds := namespaceCommands(ns, minicli.MustCompile("vm info"))

	// Collected VMs
	vms := []VM{}

	for resps := range runCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if vms2, ok := resp.Data.([]VM); ok {
				for _, vm := range vms2 {
					vms = append(vms, vm)
				}
			} else {
				log.Error("unknown data field in `vm info` from %v", resp.Host)
			}
		}
	}

	return vms
}

// expandVMNames takes a VM name, range, or count and expands it into a list of
// names of VMs that should be launched. Does several sanity checks on the
// names to make sure that they aren't reserved words.
func expandLaunchNames(arg string) ([]string, error) {
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

	for i, name := range names {
		if isReserved(name) {
			return nil, fmt.Errorf("invalid vm name, `%s` is a reserved word", name)
		}

		if _, err := strconv.Atoi(name); err == nil {
			return nil, fmt.Errorf("invalid vm name, `%s` is an integer", name)
		}

		if name == "vince" {
			log.Warn("vince is unstoppable")
		}

		// Check for conflicts within the provided names. Don't conflict with
		// ourselves or if the name is unspecified.
		for j, name2 := range names {
			if i != j && name == name2 && name != "" {
				return nil, fmt.Errorf("`%s` is specified twice in VMs to launch", name)
			}
		}
	}

	return names, nil
}
