// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	log "minilog"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// total list of vms running on this host
type VMs map[int]*vmInfo

// apply applies the provided function to the vm in VMs whose name or ID
// matches the provided vm parameter.
func (vms VMs) apply(idOrName string, fn func(*vmInfo) error) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}
	return fn(vm)
}

// start vms that are paused or building, or restart vms in the quit state
func (vms VMs) start(vm string, quit bool) []error {
	if vm != Wildcard {
		err := vms.apply(vm, func(vm *vmInfo) error { return vm.start() })
		return []error{err}
	}

	stateMask := VM_PAUSED | VM_BUILDING
	if quit {
		stateMask |= VM_QUIT
	}

	// start all paused vms
	count := 0
	errAck := make(chan error)

	for _, i := range vms {
		// only bulk start VMs matching our state mask
		if i.State&stateMask != 0 {
			count++
			go func(v *vmInfo) {
				err := v.start()
				errAck <- err
			}(i)
		}
	}

	errors := []error{}

	// get all of the acks
	for j := 0; j < count; j++ {
		if err := <-errAck; err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

// stop vms that are paused or building
func (vms VMs) stop(vm string) []error {
	if vm != Wildcard {
		err := vms.apply(vm, func(vm *vmInfo) error { return vm.stop() })
		return []error{err}
	}

	errors := []error{}
	for _, i := range vms {
		err := i.stop()
		if err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func (vms VMs) save(file *os.File, args []string) error {
	var allVms bool
	for _, vm := range args {
		if vm == Wildcard {
			allVms = true
			break
		}
	}

	if allVms && len(args) != 1 {
		log.Debug("ignoring vm names, wildcard is present")
	}

	var toSave []string
	if allVms {
		for k, _ := range vms {
			toSave = append(toSave, fmt.Sprintf("%v", k))
		}
	} else {
		toSave = args
	}

	for _, vmStr := range toSave { // iterate over the vm id's specified
		vm := vms.findVm(vmStr)
		if vm == nil {
			return fmt.Errorf("vm %v not found", vm)
		}

		// Commands to run to re-launch this vm, starting with clearing all the
		// existing vm config fields.
		cmds := []string{fmt.Sprintf("clear vm config")}

		// Add the "simple" fields
		for _, field := range vmConfigFields {
			switch f := vm.getField(field).(type) {
			case *string:
				if *f != "" {
					cmds = append(cmds, fmt.Sprintf("vm config %s %q", field, *f))
				}
			case *bool:
				cmds = append(cmds, fmt.Sprintf("vm config %s %t", field, *f))
			case *[]string:
				for _, v := range *f {
					cmds = append(cmds, fmt.Sprintf("vm config %s %q", field, v))
				}
			}
		}

		// Add the "special" fields
		for _, fns := range vmConfigSpecial {
			v := fns.PrintCLI(vm)
			if v != "" {
				cmds = append(cmds, v)
			}
		}

		if vm.Name != "" {
			cmds = append(cmds, "vm launch "+vm.Name)
		} else {
			cmds = append(cmds, "vm launch 1")
		}

		// and a blank line
		cmds = append(cmds, "")

		// write commands to file
		for _, cmd := range cmds {
			_, err := file.WriteString(cmd + "\n")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (vms VMs) qmp(idOrName, qmp string) (string, error) {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return "", vmNotFound(idOrName)
	}

	return vm.QMPRaw(qmp)
}

func (vms VMs) screenshot(idOrName, path string, max int) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}

	suffix := rand.New(rand.NewSource(time.Now().UnixNano())).Int31()
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_screenshot_%v", suffix))

	err := vm.q.Screendump(tmp)
	if err != nil {
		return err
	}

	err = ppmToPng(tmp, path, max)
	if err != nil {
		return err
	}

	err = os.Remove(tmp)
	if err != nil {
		return err
	}

	return nil
}

func (vms VMs) migrate(idOrName, filename string) error {
	vm := vms.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}

	return vm.Migrate(filename)
}

func (vms VMs) findVm(idOrName string) *vmInfo {
	id, err := strconv.Atoi(idOrName)
	if err != nil {
		// Search for VM by name
		for _, v := range vms {
			if v.Name == idOrName {
				return v
			}
		}
	}

	return vms[id]
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (vms VMs) launch(name string, ack chan int) error {
	// Make sure that there isn't another VM with the same name
	if name != "" {
		for _, vm := range vms {
			if vm.Name == name {
				return fmt.Errorf("vm launch duplicate VM name: %s", name)
			}
		}
	}

	vm := info.Copy() // returns reference to deep-copy of info
	vm.ID = <-vmIdChan
	vm.Name = name
	if vm.Name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.ID)
	}
	vm.kill = make(chan bool)
	vm.Hotplug = make(map[int]string)
	vm.Tags = make(map[string]string)
	vm.State = VM_BUILDING
	vmLock.Lock()
	vms[vm.ID] = vm
	vmLock.Unlock()
	go vm.launchOne(ack)

	return nil
}

// kill one or all vms (* for all)
func (vms VMs) kill(idOrName string) []error {
	stateMask := VM_QUIT | VM_ERROR
	killedVms := map[int]bool{}

	if idOrName != Wildcard {
		vm := vms.findVm(idOrName)
		if vm == nil {
			return []error{vmNotFound(idOrName)}
		}

		if vm.getState()&stateMask != 0 {
			return []error{fmt.Errorf("vm %v is not running", vm.Name)}
		}

		vm.kill <- true
		killedVms[vm.ID] = true
	} else {
		for _, vm := range vms {
			if vm.getState()&stateMask == 0 {
				vm.kill <- true
				killedVms[vm.ID] = true
			}
		}
	}

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

	errs := []error{}
	for id := range killedVms {
		errs = append(errs, fmt.Errorf("VM %d failed to acknowledge kill", id))
	}

	return errs
}

func (vms VMs) flush() {
	stateMask := VM_QUIT | VM_ERROR
	for i, vm := range vms {
		if vm.State&stateMask != 0 {
			log.Infoln("deleting VM: ", i)
			delete(vms, i)
		}
	}
}

func (vms VMs) info() ([]string, [][]string, error) {
	table := make([][]string, 0, len(vms))
	for _, vm := range vms {
		row, err := vm.info(vmMasks)
		if err != nil {
			continue
		}
		table = append(table, row)
	}

	return vmMasks, table, nil
}

// cleanDirs removes all isntance directories in the minimega base directory
func (vms VMs) cleanDirs() {
	log.Debugln("cleanDirs")
	for _, i := range vms {
		log.Debug("cleaning instance path: %v", i.instancePath)
		err := os.RemoveAll(i.instancePath)
		if err != nil {
			log.Error("clearDirs: %v", err)
		}
	}
}
