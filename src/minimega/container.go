// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"ipmac"
	log "minilog"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
)

type ContainerConfig struct {
	FSPath   string
	UUID     string
	Snapshot bool
}

type ContainerVM struct {
	BaseVM          // embed
	ContainerConfig // embed

	kill chan bool // kill channel to signal to shut a vm down

	pid int

	ActiveCC bool // Whether CC is active, updated by calling UpdateCCActive
}

func (vm *ContainerVM) UpdateCCActive() {
	vm.ActiveCC = ccHasClient(vm.UUID)
}

// Ensure that ContainerVM implements the VM interface
var _ VM = (*KvmVM)(nil)

// Valid names for output masks for vm kvm info, in preferred output order
var containerMasks = []string{
	"id", "name", "state", "memory", "type", "vlan", "bridge", "tap",
	"mac", "ip", "ip6", "bandwidth", "filesystem", "snapshot", "uuid",
	"cc_active", "tags",
}

func init() {
	// Reset everything to default
	for _, fns := range containerConfigFns {
		fns.Clear(&vmConfig.ContainerConfig)
	}
}

// Copy makes a deep copy and returns reference to the new struct.
func (old *ContainerConfig) Copy() *ContainerConfig {
	res := new(ContainerConfig)

	// Copy all fields
	*res = *old

	// Make deep copy of slices
	// none yet - placeholder

	return res
}

func (vm *ContainerVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func NewContainer() *ContainerVM {
	vm := new(ContainerVM)

	vm.BaseVM = *NewVM()

	vm.kill = make(chan bool)

	return vm
}

// launch one or more vms. this will copy the info struct, one per vm and
// launch each one in a goroutine. it will not return until all vms have
// reported that they've launched.
func (vm *ContainerVM) Launch(name string, ack chan int) error {
	if err := vm.BaseVM.launch(name, CONTAINER); err != nil {
		return err
	}
	vm.ContainerConfig = *vmConfig.ContainerConfig.Copy() // deep-copy configured fields

	vmLock.Lock()
	vms[vm.ID] = vm
	vmLock.Unlock()

	go vm.launch(ack)

	return nil
}

func (vm *ContainerVM) Start() error {
	s := vm.GetState()

	stateMask := VM_PAUSED | VM_BUILDING | VM_QUIT | VM_ERROR
	if s&stateMask == 0 {
		return nil
	}

	if s == VM_QUIT || s == VM_ERROR {
		log.Info("restarting VM: %v", vm.ID)
		ack := make(chan int)
		go vm.launch(ack)
		log.Debug("ack restarted VM %v", <-ack)
	}

	log.Info("starting VM: %v", vm.ID)

	// TODO: container unpause

	// 	err := vm.q.Start()
	// 	if err != nil {
	// 		log.Errorln(err)
	// 		if err != qmp.ERR_READY {
	// 			vm.setState(VM_ERROR)
	// 		}
	// 	} else {
	// 		vm.setState(VM_RUNNING)
	// 	}
	//
	return nil
}

func (vm *ContainerVM) Stop() error {
	if vm.GetState() != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.ID)
	}

	log.Info("stopping VM: %v", vm.ID)

	// TODO: container pause

	// 	err := vm.q.Stop()
	// 	if err == nil {
	// 		vm.setState(VM_PAUSED)
	// 	}

	return nil
}

func (vm *ContainerVM) Kill() error {
	// Close the channel to signal to all dependent goroutines that they should
	// stop. Anyone blocking on the channel will unblock immediately.
	// http://golang.org/ref/spec#Receive_operator
	close(vm.kill)
	// TODO: ACK if killed?
	return nil
}

func (vm *ContainerVM) String() string {
	return fmt.Sprintf("%s:%d:container", hostname, vm.ID)
}

func (vm *ContainerVM) Info(mask string) (string, error) {
	// If it's a field handled by the baseVM, use it.
	if v, err := vm.BaseVM.info(mask); err == nil {
		return v, nil
	}

	// If it's a configurable field, use the Print fn.
	if fns, ok := containerConfigFns[mask]; ok {
		return fns.Print(&vm.ContainerConfig), nil
	}

	switch mask {
	case "cc_active":
		return fmt.Sprintf("%v", vm.ActiveCC), nil
	}

	return "", fmt.Errorf("invalid mask: %s", mask)
}

func (vm *ContainerConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current container configuration:")
	fmt.Fprintf(w, "Filesystem Path:\t%v\n", vm.FSPath)
	fmt.Fprintf(w, "Snapshot:\t%v\n", vm.Snapshot)
	fmt.Fprintf(w, "UUID:\t%v\n", vm.UUID)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (vm *ContainerVM) launchPreamble(ack chan int) bool {
	// check if the vm has a conflict with the disk or mac address of another vm
	// build state of currently running system
	macMap := map[string]bool{}
	selfMacMap := map[string]bool{}
	diskSnapshotted := map[string]bool{}
	diskPersistent := map[string]bool{}

	vmLock.Lock()
	defer vmLock.Unlock()

	err := os.MkdirAll(vm.instancePath, os.FileMode(0700))
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	// generate a UUID if we don't have one
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}

	// populate selfMacMap
	for _, net := range vm.Networks {
		if net.MAC == "" { // don't worry about empty mac addresses
			continue
		}

		if _, ok := selfMacMap[net.MAC]; ok {
			// if this vm specified the same mac address for two interfaces
			log.Errorln("Cannot specify the same mac address for two interfaces")
			vm.setState(VM_ERROR)
			ack <- vm.ID // signal that this vm is "done" launching
			return false
		}
		selfMacMap[net.MAC] = true
	}

	stateMask := VM_BUILDING | VM_RUNNING | VM_PAUSED

	// populate macMap, diskSnapshotted, and diskPersistent
	for _, vm2 := range vms {
		if vm == vm2 { // ignore this vm
			continue
		}

		s := vm2.GetState()

		if s&stateMask != 0 {
			// populate mac addresses set
			for _, net := range vm2.Config().Networks {
				macMap[net.MAC] = true
			}

			if vm2, ok := vm2.(*ContainerVM); ok {
				// populate disk sets
				if vm2.Snapshot {
					diskSnapshotted[vm2.FSPath] = true
				} else {
					diskPersistent[vm2.FSPath] = true
				}

			}
		}
	}

	// check for mac address conflicts and fill in unspecified mac addresses without conflict
	for i := range vm.Networks {
		net := &vm.Networks[i]

		if net.MAC == "" { // create mac addresses where unspecified
			existsOther, existsSelf, newMac := true, true, "" // entry condition/initialization
			for existsOther || existsSelf {                   // loop until we generate a random mac that doesn't conflict (already exist)
				newMac = randomMac()               // generate a new mac address
				_, existsOther = macMap[newMac]    // check it against the set of mac addresses from other vms
				_, existsSelf = selfMacMap[newMac] // check it against the set of mac addresses specified from this vm
			}

			net.MAC = newMac          // set the unspecified mac address
			selfMacMap[newMac] = true // add this mac to the set of mac addresses for this vm
		}
	}

	// check for disk conflict
	_, existsSnapshotted := diskSnapshotted[vm.FSPath]                   // check if another vm is using this disk in snapshot mode
	_, existsPersistent := diskPersistent[vm.FSPath]                     // check if another vm is using this disk in persistent mode (snapshot=false)
	if existsPersistent || (vm.Snapshot == false && existsSnapshotted) { // if we have a disk conflict
		log.Error("disk path %v is already in use by another vm.", vm.FSPath)
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return false
	}

	return true
}

func (vm *ContainerVM) launch(ack chan int) {
	log.Info("launching vm: %v", vm.ID)

	s := vm.GetState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT && !vm.launchPreamble(ack) {
		return
	}

	vm.setState(VM_BUILDING)

	// write the config for this vm
	config := vm.String()
	err := ioutil.WriteFile(vm.instancePath+"config", []byte(config), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	err = ioutil.WriteFile(vm.instancePath+"name", []byte(vm.Name), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	var cmd *exec.Cmd
	var waitChan = make(chan int)

	// clear taps, we may have come from the quit state
	for i := range vm.Networks {
		vm.Networks[i].Tap = ""
	}

	// create and add taps if we are associated with any networks
	for i := range vm.Networks {
		net := &vm.Networks[i]

		b, err := getBridge(net.Bridge)
		if err != nil {
			log.Error("get bridge: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}

		net.Tap, err = b.TapCreate(net.VLAN)
		if err != nil {
			log.Error("create tap: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}

		updates := make(chan ipmac.IP)
		go func(vm *ContainerVM, net *NetConfig) {
			defer close(updates)
			for {
				// TODO: need to acquire VM lock?
				select {
				case update := <-updates:
					if update.IP4 != "" {
						net.IP4 = update.IP4
					} else if net.IP6 != "" && strings.HasPrefix(update.IP6, "fe80") {
						log.Debugln("ignoring link-local over existing IPv6 address")
					} else if update.IP6 != "" {
						net.IP6 = update.IP6
					}
				case <-vm.kill:
					b.iml.DelMac(net.MAC)
					return
				}
			}
		}(vm, net)

		b.iml.AddMac(net.MAC, updates)
	}

	if len(vm.Networks) > 0 {
		taps := []string{}
		for _, net := range vm.Networks {
			taps = append(taps, net.Tap)
		}

		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(taps, "\n")), 0666)
		if err != nil {
			log.Error("write instance taps file: %v", err)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return
		}
	}

	// 	cmd = &exec.Cmd{
	// 		Path:   process("qemu"),
	// 		Args:   args,
	// 		Env:    nil,
	// 		Dir:    "",
	// 		Stdout: &sOut,
	// 		Stderr: &sErr,
	// 	}
	// 	err = cmd.Start()
	// 	if err != nil {
	// 		log.Error("start qemu: %v %v", err, sErr.String())
	// 		vm.setState(VM_ERROR)
	// 		ack <- vm.ID
	// 		return
	// 	}
	//
	// 	vm.pid = cmd.Process.Pid
	// 	log.Debug("vm %v has pid %v", vm.ID, vm.pid)

	// TODO: add affinity funcs for containers
	// vm.CheckAffinity()

	go func() {
		err := cmd.Wait()
		vm.setState(VM_QUIT)
		if err != nil {
			if err.Error() != "signal: killed" { // because we killed it
				log.Error("kill container: %v %v", err, sErr.String())
				vm.setState(VM_ERROR)
			}
		}
		waitChan <- vm.ID
	}()

	// we can't just return on error at this point because we'll leave dangling goroutines, we have to clean up on failure
	sendKillAck := false

	ack <- vm.ID

	select {
	case <-waitChan:
		log.Info("VM %v exited", vm.ID)
	case <-vm.kill:
		log.Info("Killing VM %v", vm.ID)
		// TODO: kill vm
		// cmd.Process.Kill()
		<-waitChan
		sendKillAck = true // wait to ack until we've cleaned up
	}

	for _, net := range vm.Networks {
		b, err := getBridge(net.Bridge)
		if err != nil {
			log.Error("get bridge: %v", err)
		} else {
			b.TapDestroy(net.VLAN, net.Tap)
		}
	}

	if sendKillAck {
		killAck <- vm.ID
	}
}

// update the vm state, and write the state to file
func (vm *ContainerVM) setState(s VMState) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.State = s
	err := ioutil.WriteFile(vm.instancePath+"state", []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}
