// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"strconv"
	"time"
)

// total list of vms running on this host
type vmList struct {
	vms map[int]*vmInfo
}

// apply applies the provided function to the vm in vmList whose name or ID
// matches the provided vm parameter.
func (l *vmList) apply(idOrName string, fn func(*vmInfo) error) error {
	vm := l.findVm(idOrName)
	if vm == nil {
		return vmNotFound(idOrName)
	}
	return fn(vm)
}

// start vms that are paused or building, or restart vms in the quit state
func (l *vmList) start(vm string, quit bool) []error {
	if vm != Wildcard {
		err := l.apply(vm, func(vm *vmInfo) error { return vm.start() })
		return []error{err}
	}

	stateMask := VM_PAUSED | VM_BUILDING
	if quit {
		stateMask |= VM_QUIT
	}

	// start all paused vms
	count := 0
	errAck := make(chan error)

	for _, i := range l.vms {
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
func (l *vmList) stop(vm string) []error {
	if vm != Wildcard {
		err := l.apply(vm, func(vm *vmInfo) error { return vm.stop() })
		return []error{err}
	}

	errors := []error{}
	for _, i := range l.vms {
		err := i.stop()
		if err != nil {
			errors = append(errors, err)
		}
	}

	return errors
}

func (l *vmList) save(file *os.File, vms []string) error {
	var allVms bool
	for _, vm := range vms {
		if vm == Wildcard {
			allVms = true
			break
		}
	}

	if allVms && len(vms) != 1 {
		log.Info("ignoring vm names, wildcard is present")
	}

	var toSave []string
	if allVms {
		for k, _ := range l.vms {
			toSave = append(toSave, fmt.Sprintf("%v", k))
		}
	} else {
		toSave = vms
	}

	for _, vmStr := range toSave { // iterate over the vm id's specified
		vm := l.findVm(vmStr)
		if vm == nil {
			return fmt.Errorf("vm %v not found", vm)
		}

		// build up the command list to re-launch this vm
		cmds := []string{}

		for k, fns := range vmConfigFns {
			var value string
			if fns.PrintCLI != nil {
				value = fns.PrintCLI(vm)
			} else {
				value = fns.Print(vm)
				if len(value) > 0 {
					value = fmt.Sprintf("vm config %s %s", k, value)
				}
			}

			if len(value) != 0 {
				cmds = append(cmds, value)
			} else {
				cmds = append(cmds, fmt.Sprintf("clear vm config %s", k))
			}
		}

		if vm.Name != "" {
			cmds = append(cmds, "vm launch "+vm.Name)
		} else {
			cmds = append(cmds, "vm launch 1")
		}

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

func (l *vmList) qmp(idOrName, qmp string) (string, error) {
	vm := l.findVm(idOrName)
	if vm == nil {
		return "", vmNotFound(idOrName)
	}

	return vm.QMPRaw(qmp)
}

func (l *vmList) findVm(idOrName string) *vmInfo {
	id, err := strconv.Atoi(idOrName)
	if err != nil {
		// Search for VM by name
		for _, v := range l.vms {
			if v.Name == idOrName {
				return v
			}
		}
	}

	return l.vms[id]
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (l *vmList) launch(name string, ack chan int) error {
	// Make sure that there isn't another VM with the same name
	if name != "" {
		for _, vm := range l.vms {
			if vm.Name == name {
				return fmt.Errorf("vm launch duplicate VM name: %s", name)
			}
		}
	}

	vm := info.Copy() // returns reference to deep-copy of info
	vm.Id = <-vmIdChan
	vm.Name = name
	if vm.Name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.Id)
	}
	vm.Kill = make(chan bool)
	vm.Hotplug = make(map[int]string)
	vm.Tags = make(map[string]string)
	vm.State = VM_BUILDING
	vmLock.Lock()
	l.vms[vm.Id] = vm
	vmLock.Unlock()
	go vm.launchOne(ack)

	return nil
}

// kill one or all vms (* for all)
func (l *vmList) kill(idOrName string) []error {
	stateMask := VM_QUIT | VM_ERROR
	killedVms := map[int]bool{}

	if idOrName != Wildcard {
		vm := l.findVm(idOrName)
		if vm == nil {
			return []error{vmNotFound(idOrName)}
		}

		if vm.getState()&stateMask != 0 {
			return []error{fmt.Errorf("vm %v is not running", vm.Name)}
		}

		vm.Kill <- true
		killedVms[vm.Id] = true
	} else {
		for _, vm := range l.vms {
			if vm.getState()&stateMask == 0 {
				vm.Kill <- true
				killedVms[vm.Id] = true
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

func (l *vmList) flush() {
	stateMask := VM_QUIT | VM_ERROR
	for i, vm := range vms.vms {
		if vm.State&stateMask != 0 {
			log.Infoln("deleting VM: ", i)
			delete(vms.vms, i)
		}
	}
}

func (l *vmList) info() ([]string, [][]string, error) {
	table := make([][]string, 0, len(l.vms))
	for _, j := range l.vms {
		row := make([]string, 0, len(vmMasks))

		for _, mask := range vmMasks {
			switch mask {
			case "id":
				row = append(row, fmt.Sprintf("%v", j.Id))
			case "name":
				row = append(row, fmt.Sprintf("%v", j.Name))
			case "memory":
				row = append(row, fmt.Sprintf("%v", j.Memory))
			case "vcpus":
				row = append(row, fmt.Sprintf("%v", j.Vcpus))
			case "state":
				row = append(row, j.State.String())
			case "disk":
				row = append(row, fmt.Sprintf("%v", j.DiskPaths))
			case "snapshot":
				row = append(row, fmt.Sprintf("%v", j.Snapshot))
			case "initrd":
				row = append(row, fmt.Sprintf("%v", j.InitrdPath))
			case "kernel":
				row = append(row, fmt.Sprintf("%v", j.KernelPath))
			case "cdrom":
				row = append(row, fmt.Sprintf("%v", j.CdromPath))
			case "append":
				row = append(row, fmt.Sprintf("%v", j.Append))
			case "bridge":
				row = append(row, fmt.Sprintf("%v", j.bridges))
			case "tap":
				row = append(row, fmt.Sprintf("%v", j.taps))
			case "mac":
				row = append(row, fmt.Sprintf("%v", j.macs))
			case "tags":
				row = append(row, fmt.Sprintf("%v", j.Tags))
			case "ip":
				var ips []string
				for _, m := range j.macs {
					ip := GetIPFromMac(m)
					if ip != nil {
						ips = append(ips, ip.IP4)
					}
				}
				row = append(row, fmt.Sprintf("%v", ips))
			case "ip6":
				var ips []string
				for _, m := range j.macs {
					ip := GetIPFromMac(m)
					if ip != nil {
						ips = append(ips, ip.IP6)
					}
				}
				row = append(row, fmt.Sprintf("%v", ips))
			case "vlan":
				var vlans []string
				for _, v := range j.Networks {
					if v == -1 {
						vlans = append(vlans, "disconnected")
					} else {
						vlans = append(vlans, fmt.Sprintf("%v", v))
					}
				}
				row = append(row, fmt.Sprintf("%v", vlans))
			case "uuid":
				row = append(row, fmt.Sprintf("%v", j.UUID))
			case "cc_active":
				activeClients := ccClients()
				row = append(row, fmt.Sprintf("%v", activeClients[j.UUID]))
			default:
				return nil, nil, fmt.Errorf("invalid mask: %s", mask)
			}
		}

		table = append(table, row)
	}

	return vmMasks, table, nil
}

// cleanDirs removes all isntance directories in the minimega base directory
func (l *vmList) cleanDirs() {
	log.Debugln("cleanDirs")
	for _, i := range l.vms {
		log.Debug("cleaning instance path: %v", i.instancePath)
		err := os.RemoveAll(i.instancePath)
		if err != nil {
			log.Error("clearDirs: %v", err)
		}
	}
}
