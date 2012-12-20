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

// TODO: refactor this so that the vm_info struct has the Cmd struct in it, 
// and the methods for it. This way, we can just keep a global list of 
// vm_infos. This will make it easy to do things like attach to an instance's 
// stdio, or qmp socket, etc. Meanwhile, the vm_info's themselves all have 
// things running in goroutines.

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
	info        vm_info       // current vm info, interfaced be the cli
	launch_rate time.Duration // launch/kill rate for vms

	// each vm struct acknowledges that it launched. this way, we won't
	// return from a vm_launch command until all have actually launched.
	launch_ack chan int
	kill_ack   chan int
)

const (
	VM_BUILDING = iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

// total list of vms running on this host
type vm_list struct {
	vms []*vm_info
}

type vm_info struct {
	Id            int
	Memory        string // memory for the vm, in megabytes
	Vcpus         string // number of virtual cpus
	Disk_path     string
	Cdrom_path    string
	Kernel_path   string
	Initrd_path   string
	Append        string
	Qemu_Append   []string  // extra arguments for QEMU
	State         int       // one of the VM_ states listed above
	Kill          chan bool // kill channel to signal to shut a vm down
	instance_path string
	q             qmp.Conn // qmp connection for this vm
	taps          []string // list of taps associated with this vm
	Networks      []string // ordered list of networks (matches 1-1 with Taps)
}

func init() {
	launch_rate = time.Millisecond * 100
	launch_ack = make(chan int)
	kill_ack = make(chan int)

	// default parameters at startup
	info.Memory = "512"
	info.Vcpus = "1"
	info.Disk_path = ""
	info.Kernel_path = ""
	info.Initrd_path = ""
	info.State = VM_BUILDING
}

// return internal and qmp status of one or more vms
func (l *vm_list) status(c cli_command) cli_response {
	if len(c.Args) == 0 {
		var s string
		for _, i := range l.vms {
			s += i.status()
		}
		return cli_response{
			Response: s,
		}
	} else if len(c.Args) != 1 {
		return cli_response{
			Error: "status takes one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err.Error(),
			}
		}
		// find that vm, should be in order...
		if id < len(l.vms) {
			s := l.vms[id].status()
			return cli_response{
				Response: s,
			}
		} else {
			return cli_response{
				Error: "invalid VM id",
			}
		}
	}
	return cli_response{}
}

// start vms that are paused or building
func (l *vm_list) start(c cli_command) cli_response {
	if len(c.Args) == 0 { // start all paused vms
		for _, i := range l.vms {
			i.start()
		}
	} else if len(c.Args) != 1 {
		return cli_response{
			Error: "start takes one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err.Error(),
			}
		}
		if id < len(l.vms) {
			l.vms[id].start()
		} else {
			return cli_response{
				Error: "invalid VM id",
			}
		}
	}
	return cli_response{}
}

// kill one or all vms (-1 for all)
func (l *vm_list) kill(id int) {
	if id == -1 {
		for _, i := range l.vms {
			if i.State != VM_QUIT {
				i.Kill <- true
				log.Info("VM %v killed", <-kill_ack)
			}
		}
	} else {
		if l.vms[id] == nil {
			log.Error("invalid VM id")
		} else {
			if l.vms[id].State != VM_QUIT {
				l.vms[id].Kill <- true
				log.Info("VM %v killed", <-kill_ack)
			}
		}
	}
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (l *vm_list) launch(num_vms int) {
	// we have some configuration from the cli (right?), all we need 
	// to do here is fire off the vms in goroutines, passing the 
	// configuration in by value, as it may change for the next run.
	log.Info("launching %v vms", num_vms)
	start := len(l.vms)
	for i := start; i < num_vms+start; i++ {
		vm := info
		vm.Id = i
		vm.Kill = make(chan bool)
		l.vms = append(l.vms, &vm)
		go vm.launch_one()
	}
	// get acknowledgements from each vm
	for i := 0; i < num_vms; i++ {
		fmt.Printf("VM: %v launched\n", <-launch_ack)
	}
}

func (vm *vm_info) status() string {
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
	return fmt.Sprintf("VM %v: %v, QMP: %v\n", vm.Id, s, status["status"])
}

func (vm *vm_info) start() {
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

func (vm *vm_info) launch_one() {
	log.Info("launching vm: %v", vm.Id)

	vm.instance_path = *f_base + strconv.Itoa(vm.Id) + "/"
	err := os.MkdirAll(vm.instance_path, os.FileMode(0700))
	if err != nil {
		log.Fatal("%v", err)
	}

	// assert our state as building
	vm.state(VM_BUILDING)

	// create and add taps if we are associated with any networks
	for _, lan := range vm.Networks {
		tap, err := current_bridge.Tap_create(lan, false)
		if err != nil {
			log.Error("%v", err)
			continue
		}
		vm.taps = append(vm.taps, tap)
	}

	if len(vm.Networks) > 0 {
		err := ioutil.WriteFile(vm.instance_path+"taps", []byte(strings.Join(vm.taps, "\n")), 0666)
		if err != nil {
			log.Error("%v", err)
		}
	}

	args := vm.vm_get_args()
	var s_out bytes.Buffer
	var s_err bytes.Buffer
	cmd := &exec.Cmd{
		Path:   process("qemu"),
		Args:   args,
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	err = cmd.Start()

	if err != nil {
		log.Error("%v %v", err, s_err.String())
	}
	wait_chan := make(chan bool)
	go func() {
		err = cmd.Wait()
		if err != nil {
			if err.Error() != "signal 9" { // because we killed it
				log.Error("%v %v", err, s_err.String())
			}
		}
		wait_chan <- true
	}()

	time.Sleep(launch_rate)

	// connect to qmp
	vm.q, err = qmp.Dial(vm.qmp_path())
	if err != nil {
		log.Error("vm %v failed to connect to qmp: %v", vm.Id, err)
	}

	go vm.async_logger()

	launch_ack <- vm.Id

	select {
	case <-wait_chan:
		log.Info("VM %v exited", vm.Id)
	case <-vm.Kill:
		fmt.Printf("Killing VM %v\n", vm.Id)
		cmd.Process.Kill()
	}
	time.Sleep(launch_rate)

	kill_ack <- vm.Id
	//err = os.RemoveAll(vm.instance_path)
	//if err != nil {
	//	log.Error("%v", err)
	//}
	vm.state(VM_QUIT)
}

// update the vm state, and write the state to file
func (vm *vm_info) state(s int) {
	var state_string string
	switch s {
	case VM_BUILDING:
		state_string = "VM_BUILDING"
	case VM_RUNNING:
		state_string = "VM_RUNNING"
	case VM_PAUSED:
		state_string = "VM_PAUSED"
	case VM_QUIT:
		state_string = "VM_QUIT"
	case VM_ERROR:
		state_string = "VM_ERROR"
	default:
		log.Errorln("unknown state")
	}
	vm.State = s
	err := ioutil.WriteFile(vm.instance_path+"state", []byte(state_string), 0666)
	if err != nil {
		log.Errorln(err)
	}
}

// return the path to the qmp socket
func (vm *vm_info) qmp_path() string {
	return vm.instance_path + "qmp"
}

// build the horribly long qemu argument string
func (vm *vm_info) vm_get_args() []string {
	var args []string

	s_id := strconv.Itoa(vm.Id)

	args = append(args, process("qemu"))

	args = append(args, "-enable-kvm")

	args = append(args, "-name")
	args = append(args, s_id)

	args = append(args, "-m")
	args = append(args, vm.Memory)

	args = append(args, "-nographic")

	args = append(args, "-balloon")
	args = append(args, "none")

	args = append(args, "-vnc")
	args = append(args, "0.0.0.0:"+s_id) // if we have more than 10000 vnc sessions, we're in trouble

	args = append(args, "-usbdevice") // this allows absolute pointers in vnc, and works great on android vms
	args = append(args, "tablet")

	args = append(args, "-smp")
	args = append(args, vm.Vcpus)

	args = append(args, "-qmp")
	args = append(args, "unix:"+vm.qmp_path()+",server")

	args = append(args, "-vga")
	args = append(args, "cirrus")

	args = append(args, "-rtc")
	args = append(args, "clock=vm,base=utc")

	args = append(args, "-chardev")
	args = append(args, "file,id=charserial0,path="+vm.instance_path+"serial")

	args = append(args, "-pidfile")
	args = append(args, vm.instance_path+"qemu.pid")

	args = append(args, "-device")
	args = append(args, "isa-serial,chardev=charserial0,id=serial0")

	args = append(args, "-k")
	args = append(args, "en-us")

	args = append(args, "-cpu")
	args = append(args, "qemu64")

	args = append(args, "-net")
	args = append(args, "none")

	args = append(args, "-S")

	if vm.Disk_path != "" {
		args = append(args, "-drive")
		args = append(args, "file="+vm.Disk_path+",cache=writeback,media=disk")
		args = append(args, "-snapshot")
	}

	if vm.Kernel_path != "" {
		args = append(args, "-kernel")
		args = append(args, vm.Kernel_path)
	}
	if vm.Initrd_path != "" {
		args = append(args, "-initrd")
		args = append(args, vm.Initrd_path)
	}
	if vm.Append != "" {
		args = append(args, "-append")
		args = append(args, vm.Append)
	}

	if vm.Cdrom_path != "" {
		args = append(args, "-drive")
		args = append(args, "file="+vm.Cdrom_path+",if=ide,index=1,media=cdrom")
		args = append(args, "-boot")
		args = append(args, "once=d")
	}

	for _, tap := range vm.taps {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", tap, tap))
		args = append(args, "-device")
		args = append(args, fmt.Sprintf("e1000,netdev=%v,mac=%v", tap, random_mac()))
	}

	if len(vm.Qemu_Append) > 0 {
		args = append(args, vm.Qemu_Append...)
	}

	log.Info("args for vm %v is: %v", vm.Id, strings.Join(args, " "))
	return args
}

// log any asynchronous messages, such as vnc connects, to log.Info
func (vm *vm_info) async_logger() {
	for {
		v := vm.q.Message()
		log.Info("VM %v received asynchronous message: %v", vm.Id, v)
	}
}
