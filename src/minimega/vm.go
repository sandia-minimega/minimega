// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// virtual machine control routines. The vm state is centered around the 'info'
// struct, which is updated via the cli. When vms are launched, the info struct
// is copied once for each VM and launched. The user can then update the vm 
// struct for launching vms of other types.
package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"qmp"
	"strconv"
	"strings"
	"time"
)

var (
	info       vmInfo        // current vm info, interfaced be the cli
	launchRate time.Duration // launch/kill rate for vms

	// each vm struct acknowledges that it launched. this way, we won't
	// return from a vm_launch command until all have actually launched.
	launchAck chan int
	killAck   chan int
)

const (
	VM_BUILDING = iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

// total list of vms running on this host
type vmList struct {
	vms []*vmInfo
}

type vmInfo struct {
	Id           int
	Memory       string // memory for the vm, in megabytes
	Vcpus        string // number of virtual cpus
	DiskPath     string
	CdromPath    string
	KernelPath   string
	InitrdPath   string
	Append       string
	QemuAppend   []string  // extra arguments for QEMU
	State        int       // one of the VM_ states listed above
	Kill         chan bool // kill channel to signal to shut a vm down
	instancePath string
	q            qmp.Conn // qmp connection for this vm
	taps         []string // list of taps associated with this vm
	Networks     []int    // ordered list of networks (matches 1-1 with Taps)
}

func init() {
	launchRate = time.Millisecond * 100
	launchAck = make(chan int)
	killAck = make(chan int)

	// default parameters at startup
	info.Memory = "512"
	info.Vcpus = "1"
	info.DiskPath = ""
	info.KernelPath = ""
	info.InitrdPath = ""
	info.State = VM_BUILDING
}

// return internal and qmp status of one or more vms
func (l *vmList) status(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		var s string
		for _, i := range l.vms {
			s += i.status()
		}
		return cliResponse{
			Response: s,
		}
	} else if len(c.Args) != 1 {
		return cliResponse{
			Error: "status takes one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		// find that vm, should be in order...
		if id < len(l.vms) {
			s := l.vms[id].status()
			return cliResponse{
				Response: s,
			}
		} else {
			return cliResponse{
				Error: "invalid VM id",
			}
		}
	}
	return cliResponse{}
}

// start vms that are paused or building
func (l *vmList) start(c cliCommand) cliResponse {
	if len(c.Args) == 0 { // start all paused vms
		for _, i := range l.vms {
			i.start()
		}
	} else if len(c.Args) != 1 {
		return cliResponse{
			Error: "start takes one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		if id < len(l.vms) {
			l.vms[id].start()
		} else {
			return cliResponse{
				Error: "invalid VM id",
			}
		}
	}
	return cliResponse{}
}

// kill one or all vms (-1 for all)
func (l *vmList) kill(id int) {
	if id == -1 {
		for _, i := range l.vms {
			if i.State != VM_QUIT && i.State != VM_ERROR {
				i.Kill <- true
				log.Info("VM %v killed", <-killAck)
			}
		}
	} else {
		if len(l.vms) < id {
			log.Error("invalid VM id")
		} else if l.vms[id] == nil {
			log.Error("invalid VM id")
		} else {
			if l.vms[id].State != VM_QUIT && l.vms[id].State != VM_ERROR {
				l.vms[id].Kill <- true
				log.Info("VM %v killed", <-killAck)
			}
		}
	}
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (l *vmList) launch(numVms int) {
	// we have some configuration from the cli (right?), all we need 
	// to do here is fire off the vms in goroutines, passing the 
	// configuration in by value, as it may change for the next run.
	log.Info("launching %v vms", numVms)
	start := len(l.vms)
	for i := start; i < numVms+start; i++ {
		vm := info
		vm.Id = i
		vm.Kill = make(chan bool)
		l.vms = append(l.vms, &vm)
		go vm.launchOne()
	}
	// get acknowledgements from each vm
	for i := 0; i < numVms; i++ {
		fmt.Printf("VM: %v launched\n", <-launchAck)
	}
}

func (vm *vmInfo) status() string {
	var s string
	switch vm.State {
	case VM_BUILDING:
		s = "BUILDING"
	case VM_RUNNING:
		s = "RUNNING"
	case VM_PAUSED:
		s = "PAUSED"
	case VM_QUIT: // don't call qmp if we're VM_QUIT
		return fmt.Sprintf("VM %v: QUIT\n", vm.Id)
	case VM_ERROR:
		return fmt.Sprintf("VM %v: ERROR\n", vm.Id)
	}
	status, err := vm.q.Status()
	if err != nil {
		log.Error("could not get qmp status")
		vm.state(VM_ERROR)
	}
	return fmt.Sprintf("VM %v : %v, QMP : %v\n", vm.Id, s, status["status"])
}

func (vm *vmInfo) start() {
	if vm.State != VM_PAUSED && vm.State != VM_BUILDING {
		log.Info("VM %v not runnable", vm.Id)
		return
	}
	log.Info("starting VM: %v", vm.Id)
	err := vm.q.Start()
	if err != nil {
		log.Error("%v", err)
		vm.state(VM_ERROR)
	} else {
		vm.state(VM_RUNNING)
	}
}

func (vm *vmInfo) launchOne() {
	log.Info("launching vm: %v", vm.Id)

	vm.instancePath = *f_base + strconv.Itoa(vm.Id) + "/"
	err := os.MkdirAll(vm.instancePath, os.FileMode(0700))
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	// assert our state as building
	vm.state(VM_BUILDING)

	// create and add taps if we are associated with any networks
	for _, lan := range vm.Networks {
		tap, err := currentBridge.TapCreate(lan)
		if err != nil {
			log.Errorln(err)
			vm.state(VM_ERROR)
			return
		}
		vm.taps = append(vm.taps, tap)
	}

	if len(vm.Networks) > 0 {
		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(vm.taps, "\n")), 0666)
		if err != nil {
			log.Errorln(err)
			vm.state(VM_ERROR)
			return
		}
	}

	args := vm.vmGetArgs()
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	cmd := &exec.Cmd{
		Path:   process("qemu"),
		Args:   args,
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	err = cmd.Start()

	if err != nil {
		log.Error("%v %v", err, sErr.String())
		vm.state(VM_ERROR)
		return
	}
	waitChan := make(chan bool)
	go func() {
		err = cmd.Wait()
		vm.state(VM_QUIT)
		if err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("%v %v", err, sErr.String())
				vm.state(VM_ERROR)
			}
		}
		waitChan <- true
	}()

	time.Sleep(launchRate)

	// connect to qmp
	vm.q, err = qmp.Dial(vm.qmpPath())
	if err != nil {
		log.Error("vm %v failed to connect to qmp: %v", vm.Id, err)
		vm.state(VM_ERROR)
		return
	}

	go vm.asyncLogger()

	launchAck <- vm.Id

	select {
	case <-waitChan:
		log.Info("VM %v exited", vm.Id)
	case <-vm.Kill:
		fmt.Printf("Killing VM %v\n", vm.Id)
		cmd.Process.Kill()
	}
	time.Sleep(launchRate)

	killAck <- vm.Id
}

// update the vm state, and write the state to file
func (vm *vmInfo) state(s int) {
	var stateString string
	switch s {
	case VM_BUILDING:
		stateString = "VM_BUILDING"
	case VM_RUNNING:
		stateString = "VM_RUNNING"
	case VM_PAUSED:
		stateString = "VM_PAUSED"
	case VM_QUIT:
		stateString = "VM_QUIT"
	case VM_ERROR:
		stateString = "VM_ERROR"
	default:
		log.Errorln("unknown state")
	}
	vm.State = s
	err := ioutil.WriteFile(vm.instancePath+"state", []byte(stateString), 0666)
	if err != nil {
		log.Errorln(err)
	}
}

// return the path to the qmp socket
func (vm *vmInfo) qmpPath() string {
	return vm.instancePath + "qmp"
}

// build the horribly long qemu argument string
func (vm *vmInfo) vmGetArgs() []string {
	var args []string

	sId := strconv.Itoa(vm.Id)

	args = append(args, process("qemu"))

	args = append(args, "-enable-kvm")

	args = append(args, "-name")
	args = append(args, sId)

	args = append(args, "-m")
	args = append(args, vm.Memory)

	args = append(args, "-nographic")

	args = append(args, "-balloon")
	args = append(args, "none")

	args = append(args, "-vnc")
	args = append(args, "0.0.0.0:"+sId) // if we have more than 10000 vnc sessions, we're in trouble

	args = append(args, "-usbdevice") // this allows absolute pointers in vnc, and works great on android vms
	args = append(args, "tablet")

	args = append(args, "-smp")
	args = append(args, vm.Vcpus)

	args = append(args, "-qmp")
	args = append(args, "unix:"+vm.qmpPath()+",server")

	args = append(args, "-vga")
	args = append(args, "cirrus")

	args = append(args, "-rtc")
	args = append(args, "clock=vm,base=utc")

	args = append(args, "-chardev")
	args = append(args, "file,id=charserial0,path="+vm.instancePath+"serial")

	args = append(args, "-pidfile")
	args = append(args, vm.instancePath+"qemu.pid")

	args = append(args, "-device")
	args = append(args, "isa-serial,chardev=charserial0,id=serial0")

	args = append(args, "-k")
	args = append(args, "en-us")

	args = append(args, "-cpu")
	args = append(args, "qemu64")

	args = append(args, "-net")
	args = append(args, "none")

	args = append(args, "-S")

	if vm.DiskPath != "" {
		args = append(args, "-drive")
		args = append(args, "file="+vm.DiskPath+",cache=writeback,media=disk")
		args = append(args, "-snapshot")
	}

	if vm.KernelPath != "" {
		args = append(args, "-kernel")
		args = append(args, vm.KernelPath)
	}
	if vm.InitrdPath != "" {
		args = append(args, "-initrd")
		args = append(args, vm.InitrdPath)
	}
	if vm.Append != "" {
		args = append(args, "-append")
		args = append(args, vm.Append)
	}

	if vm.CdromPath != "" {
		args = append(args, "-drive")
		args = append(args, "file="+vm.CdromPath+",if=ide,index=1,media=cdrom")
		args = append(args, "-boot")
		args = append(args, "once=d")
	}

	for _, tap := range vm.taps {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", tap, tap))
		args = append(args, "-device")
		args = append(args, fmt.Sprintf("e1000,netdev=%v,mac=%v", tap, randomMac()))
	}

	if len(vm.QemuAppend) > 0 {
		args = append(args, vm.QemuAppend...)
	}

	log.Info("args for vm %v is: %v", vm.Id, strings.Join(args, " "))
	return args
}

// log any asynchronous messages, such as vnc connects, to log.Info
func (vm *vmInfo) asyncLogger() {
	for {
		v := vm.q.Message()
		log.Info("VM %v received asynchronous message: %v", vm.Id, v)
	}
}
