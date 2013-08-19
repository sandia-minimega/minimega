// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

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
	"text/tabwriter"
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
	Name         string
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
	macs         []string // ordered list of macs (matches 1-1 with Taps, Networks)
	Snapshot     bool
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
	info.Snapshot = true
}

// return a pretty printed list of the current configuration
func cliVMConfig(c cliCommand) cliResponse {
	config := configToString()

	return cliResponse{
		Response: config,
	}
}

func configToString() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current VM configuration:")
	fmt.Fprintf(w, "Memory:\t%v\n", info.Memory)
	fmt.Fprintf(w, "VCPUS:\t%v\n", info.Vcpus)
	fmt.Fprintf(w, "Disk Path:\t%v\n", info.DiskPath)
	fmt.Fprintf(w, "CDROM Path:\t%v\n", info.CdromPath)
	fmt.Fprintf(w, "Kernel Path:\t%v\n", info.KernelPath)
	fmt.Fprintf(w, "Initrd Path:\t%v\n", info.InitrdPath)
	fmt.Fprintf(w, "Kernel Append:\t%v\n", info.Append)
	fmt.Fprintf(w, "QEMU Path:\t%v\n", process("qemu"))
	fmt.Fprintf(w, "QEMU Append:\t%v\n", info.QemuAppend)
	fmt.Fprintf(w, "Snapshot:\t%v\n", info.Snapshot)
	fmt.Fprintf(w, "Networks:\t%v\n", info.Networks)
	w.Flush()
	return o.String()
}

func cliVMSnapshot(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: fmt.Sprintf("%v", info.Snapshot),
		}
	}
	switch strings.ToLower(c.Args[0]) {
	case "true":
		info.Snapshot = true
	case "false":
		info.Snapshot = false
	default:
		return cliResponse{
			Error: "usage: vm_snapshot [true,false]",
		}
	}
	return cliResponse{}
}

// start vms that are paused or building
func (l *vmList) start(c cliCommand) cliResponse {
	errors := ""
	if len(c.Args) == 0 { // start all paused vms
		for _, i := range l.vms {
			err := i.start()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		}
	} else if len(c.Args) != 1 {
		return cliResponse{
			Error: "vm_start takes zero or one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			id = l.findByName(c.Args[0])
		}

		if id == -2 {
			return cliResponse{
				Error: fmt.Sprintf("VM %v not found", c.Args[0]),
			}
		} else if id < len(l.vms) && id >= 0 {
			err := l.vms[id].start()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		} else {
			return cliResponse{
				Error: fmt.Sprintf("invalid VM id: %v", id),
			}
		}
	}
	return cliResponse{
		Error: errors,
	}
}

func (vm *vmInfo) start() error {
	if vm.State != VM_PAUSED && vm.State != VM_BUILDING {
		return nil
	}
	log.Info("starting VM: %v", vm.Id)
	err := vm.q.Start()
	if err != nil {
		vm.state(VM_ERROR)
		return err
	} else {
		vm.state(VM_RUNNING)
	}
	return nil
}

// stop vms that are paused or building
func (l *vmList) stop(c cliCommand) cliResponse {
	errors := ""
	if len(c.Args) == 0 { // start all paused vms
		for _, i := range l.vms {
			err := i.stop()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		}
	} else if len(c.Args) != 1 {
		return cliResponse{
			Error: "vm_stop takes zero or one argument",
		}
	} else {
		id, err := strconv.Atoi(c.Args[0])
		if err != nil {
			id = l.findByName(c.Args[0])
		}

		if id == -2 {
			return cliResponse{
				Error: fmt.Sprintf("VM %v not found", c.Args[0]),
			}
		} else if id < len(l.vms) && id >= 0 {
			err := l.vms[id].stop()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		} else {
			return cliResponse{
				Error: "invalid VM id",
			}
		}
	}
	return cliResponse{
		Error: errors,
	}
}

func (vm *vmInfo) stop() error {
	if vm.State != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.Id)
	}
	log.Info("stopping VM: %v", vm.Id)
	err := vm.q.Stop()
	if err != nil {
		vm.state(VM_ERROR)
		return err
	} else {
		vm.state(VM_PAUSED)
	}
	return nil
}

// findByName returns the id of a VM based on its name. If the VM doesn't exist
// return -2, as -1 is reserved as the wildcard.
func (l *vmList) findByName(name string) int {
	for i, v := range l.vms {
		if v.Name == name {
			return i
		}
	}
	return -2
}

// kill one or all vms (-1 for all)
func (l *vmList) kill(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "vm_kill takes one argument",
		}
	}
	// if the argument is a number, then kill that vm (or all vms on -1)
	// if it's a string, kill the one with that name
	id, err := strconv.Atoi(c.Args[0])
	if err != nil {
		id = l.findByName(c.Args[0])
	}

	if id == -2 {
		return cliResponse{
			Error: fmt.Sprintf("VM %v not found", c.Args[0]),
		}
	} else if id == -1 {
		for _, i := range l.vms {
			if i.State != VM_QUIT && i.State != VM_ERROR {
				i.Kill <- true
				log.Info("VM %v killed", <-killAck)
			}
		}
	} else {
		if len(l.vms) < id {
			return cliResponse{
				Error: fmt.Sprintf("invalid VM id: %v", id),
			}
		} else if l.vms[id] == nil {
			return cliResponse{
				Error: fmt.Sprintf("invalid VM id: %v", id),
			}
		} else {
			if l.vms[id].State != VM_QUIT && l.vms[id].State != VM_ERROR {
				l.vms[id].Kill <- true
				log.Info("VM %v killed", <-killAck)
			}
		}
	}
	return cliResponse{}
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (l *vmList) launch(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "vm_launch takes one argument",
		}
	}
	// if the argument is a number, then launch that many VMs
	// if it's a string, launch one with that name
	var name string
	numVms, err := strconv.Atoi(c.Args[0])
	if err != nil {
		numVms = 1
		name = c.Args[0]
	}

	// we have some configuration from the cli (right?), all we need
	// to do here is fire off the vms in goroutines, passing the
	// configuration in by value, as it may change for the next run.
	log.Info("launching %v vms, name %v", numVms, name)
	start := len(l.vms)
	for i := start; i < numVms+start; i++ {
		vm := info
		vm.Id = i
		vm.Name = name
		vm.Kill = make(chan bool)
		l.vms = append(l.vms, &vm)
		go vm.launchOne()
	}
	// get acknowledgements from each vm
	ret := ""
	for i := 0; i < numVms; i++ {
		ret += fmt.Sprintf("VM: %v launched\n", <-launchAck)
	}
	return cliResponse{
		Response: ret,
	}
}

func (l *vmList) info(c cliCommand) cliResponse {
	var v []*vmInfo

	var search string
	var mask string
	switch len(c.Args) {
	case 0:
	case 1: // search or mask
		if strings.Contains(c.Args[0], "=") {
			search = c.Args[0]
		} else if strings.HasPrefix(c.Args[0], "[") {
			mask = strings.Trim(c.Args[0], "[]")
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
	case 2: // first term MUST be search
		if strings.Contains(c.Args[0], "=") {
			search = c.Args[0]
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
		if strings.HasPrefix(c.Args[1], "[") {
			mask = strings.Trim(c.Args[1], "[]")
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
	default:
		return cliResponse{
			Error: "too many arguments",
		}
	}

	// vm_info takes a search term and an output mask, we'll start with the optional seach term
	if search != "" {
		d := strings.Split(c.Args[0], "=")
		if len(d) != 2 {
			return cliResponse{
				Error: "malformed search term",
			}
		}

		log.Debug("vm_info: search term: %v", d)

		switch strings.ToLower(d[0]) {
		case "host":
			host, err := os.Hostname()
			if err != nil {
				log.Errorln(err)
				teardown()
			}
			if strings.ToLower(d[1]) == strings.ToLower(host) {
				v = append(v, l.vms...)
			}
		case "id":
			id, err := strconv.Atoi(d[1])
			if err != nil {
				return cliResponse{
					Error: fmt.Sprintf("invalid ID: %v", d[1]),
				}
			}
			for i, j := range l.vms {
				if j.Id == id {
					v = append(v, l.vms[i])
					break // there can only be one vm with this id
				}
			}
		case "name":
			id := l.findByName(d[1])
			if id == -2 {
				return cliResponse{
					Error: fmt.Sprintf("cannot find VM %v", d[1]),
				}
			}
			for i, j := range l.vms {
				if j.Id == id {
					v = append(v, l.vms[i])
					break // there can only be one vm with this id
				}
			}
		case "memory":
			for i, j := range l.vms {
				if j.Memory == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "disk":
			for i, j := range l.vms {
				if j.DiskPath == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "initrd":
			for i, j := range l.vms {
				if j.InitrdPath == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "kernel":
			for i, j := range l.vms {
				if j.KernelPath == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "cdrom":
			for i, j := range l.vms {
				if j.CdromPath == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "state":
			var s int
			switch strings.ToLower(d[1]) {
			case "building":
				s = VM_BUILDING
			case "running":
				s = VM_RUNNING
			case "paused":
				s = VM_PAUSED
			case "quit":
				s = VM_QUIT
			case "error":
				s = VM_ERROR
			default:
				return cliResponse{
					Error: fmt.Sprintf("invalid state: %v", d[1]),
				}
			}
			for i, j := range l.vms {
				if j.State == s {
					v = append(v, l.vms[i])
				}
			}
		case "tap":
		VM_INFO_TAP_LOOP:
			for i, j := range l.vms {
				for _, k := range j.taps {
					if k == d[1] {
						v = append(v, l.vms[i])
						break VM_INFO_TAP_LOOP
					}
				}
			}
		case "mac":
			for i, j := range l.vms {
				for _, k := range j.macs {
					if k == d[1] {
						v = append(v, l.vms[i])
						break
					}
				}
			}
		case "ip":
			for i, j := range l.vms {
				for _, m := range j.macs {
					ip := currentBridge.iml.GetMac(m)
					if ip != nil {
						if ip.IP4 == d[1] {
							v = append(v, l.vms[i])
							break
						}
					}
				}
			}
		case "ip6":
			for i, j := range l.vms {
				for _, m := range j.macs {
					ip := currentBridge.iml.GetMac(m)
					if ip != nil {
						if ip.IP6 == d[1] {
							v = append(v, l.vms[i])
							break
						}
					}
				}
			}
		case "vlan":
			vlan, err := strconv.Atoi(d[1])
			if err != nil {
				return cliResponse{
					Error: fmt.Sprintf("invalid tap: %v", d[1]),
				}
			}
			for i, j := range l.vms {
				for _, k := range j.Networks {
					if k == vlan {
						v = append(v, l.vms[i])
						break
					}
				}
			}
		default:
			return cliResponse{
				Error: fmt.Sprintf("invalid search term: %v", d[0]),
			}
		}
	} else { // all vms
		v = l.vms
	}
	if len(v) == 0 {
		return cliResponse{
			Error: "no VMs found",
		}
	}

	// output mask
	var omask []string
	if mask != "" {
		d := strings.Split(mask, ",")
		for _, j := range d {
			switch strings.ToLower(j) {
			case "host":
				omask = append(omask, "host")
			case "id":
				omask = append(omask, "id")
			case "name":
				omask = append(omask, "name")
			case "memory":
				omask = append(omask, "memory")
			case "disk":
				omask = append(omask, "disk")
			case "initrd":
				omask = append(omask, "initrd")
			case "kernel":
				omask = append(omask, "kernel")
			case "cdrom":
				omask = append(omask, "cdrom")
			case "state":
				omask = append(omask, "state")
			case "tap":
				omask = append(omask, "tap")
			case "mac":
				omask = append(omask, "mac")
			case "ip":
				omask = append(omask, "ip")
			case "ip6":
				omask = append(omask, "ip6")
			case "vlan":
				omask = append(omask, "vlan")
			default:
				return cliResponse{
					Error: fmt.Sprintf("invalid output mask: %v", j),
				}
			}
		}
	} else { // print everything
		omask = []string{"host", "id", "name", "state", "memory", "disk", "initrd", "kernel", "cdrom", "tap", "mac", "ip", "ip6", "vlan"}
	}

	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	for i, k := range omask {
		if i != 0 {
			fmt.Fprintf(w, "\t| ")
		}
		fmt.Fprintf(w, k)
	}
	fmt.Fprintf(w, "\n")
	for _, j := range v {
		for i, k := range omask {
			if i != 0 {
				fmt.Fprintf(w, "\t| ")
			}
			switch k {
			case "host":
				host, err := os.Hostname()
				if err != nil {
					log.Errorln(err)
					teardown()
				}
				fmt.Fprintf(w, "%v", host)
			case "id":
				fmt.Fprintf(w, "%v", j.Id)
			case "name":
				fmt.Fprintf(w, "%v", j.Name)
			case "memory":
				fmt.Fprintf(w, "%v", j.Memory)
			case "state":
				switch j.State {
				case VM_BUILDING:
					fmt.Fprintf(w, "building")
				case VM_RUNNING:
					fmt.Fprintf(w, "running")
				case VM_PAUSED:
					fmt.Fprintf(w, "paused")
				case VM_QUIT:
					fmt.Fprintf(w, "quit")
				case VM_ERROR:
					fmt.Fprintf(w, "error")
				default:
					fmt.Fprintf(w, "unknown")
				}
			case "disk":
				fmt.Fprintf(w, "%v", j.DiskPath)
				if j.Snapshot && j.DiskPath != "" {
					fmt.Fprintf(w, " [snapshot]")
				}
			case "initrd":
				fmt.Fprintf(w, "%v", j.InitrdPath)
			case "kernel":
				fmt.Fprintf(w, "%v", j.KernelPath)
			case "cdrom":
				fmt.Fprintf(w, "%v", j.CdromPath)
			case "tap":
				fmt.Fprintf(w, "%v", j.taps)
			case "mac":
				fmt.Fprintf(w, "%v", j.macs)
			case "ip":
				var ips []string
				for _, m := range j.macs {
					ip := currentBridge.iml.GetMac(m)
					if ip != nil {
						ips = append(ips, ip.IP4)
					}
				}
				fmt.Fprintf(w, "%v", ips)
			case "ip6":
				var ips []string
				for _, m := range j.macs {
					ip := currentBridge.iml.GetMac(m)
					if ip != nil {
						ips = append(ips, ip.IP6)
					}
				}
				fmt.Fprintf(w, "%v", ips)
			case "vlan":
				fmt.Fprintf(w, "%v", j.Networks)
			}
		}
		fmt.Fprintf(w, "\n")
	}
	w.Flush()

	return cliResponse{
		Response: o.String(),
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

	vm.state(VM_BUILDING)

	// write the config for this vm
	config := configToString()
	err = ioutil.WriteFile(vm.instancePath+"config", []byte(config), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}
	err = ioutil.WriteFile(vm.instancePath+"name", []byte(vm.Name), 0664)
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	var args []string
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	var cmd *exec.Cmd
	var waitChan = make(chan int)

	// create and add taps if we are associated with any networks
	for _, lan := range vm.Networks {
		tap, err := currentBridge.TapCreate(lan)
		if err != nil {
			log.Errorln(err)
			vm.state(VM_ERROR)
			launchAck <- vm.Id
			return
		}
		vm.taps = append(vm.taps, tap)
	}

	if len(vm.Networks) > 0 {
		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(vm.taps, "\n")), 0666)
		if err != nil {
			log.Errorln(err)
			vm.state(VM_ERROR)
			launchAck <- vm.Id
			return
		}
	}

	args = vm.vmGetArgs()
	cmd = &exec.Cmd{
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
		launchAck <- vm.Id
		return
	}
	go func() {
		err = cmd.Wait()
		vm.state(VM_QUIT)
		if err != nil {
			if err.Error() != "signal: killed" { // because we killed it
				log.Error("%v %v", err, sErr.String())
				vm.state(VM_ERROR)
			}
		}
		waitChan <- vm.Id
	}()

	// we can't just return on error at this point because we'll leave dangling goroutines, we have to clean up on failure

	time.Sleep(launchRate)

	// connect to qmp
	vm.q, err = qmp.Dial(vm.qmpPath())
	if err != nil {
		log.Error("vm %v failed to connect to qmp: %v", vm.Id, err)
		vm.state(VM_ERROR)
		cmd.Process.Kill()
		<-waitChan
		launchAck <- vm.Id
	} else {
		go vm.asyncLogger()

		launchAck <- vm.Id

		select {
		case <-waitChan:
			log.Info("VM %v exited", vm.Id)
		case <-vm.Kill:
			log.Info("Killing VM %v", vm.Id)
			cmd.Process.Kill()
			killAck <- <-waitChan
		}
	}

	for i, l := range vm.Networks {
		currentBridge.TapDestroy(l, vm.taps[i])
	}

	err = os.RemoveAll(vm.instancePath)
	if err != nil {
		log.Errorln(err)
	}
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
	args = append(args, "socket,id=charserial0,path="+vm.instancePath+"serial,server,nowait")

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
		if vm.Snapshot {
			args = append(args, "-snapshot")
		}
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
		mac := randomMac()
		vm.macs = append(vm.macs, mac)
		currentBridge.iml.AddMac(mac)
		args = append(args, fmt.Sprintf("e1000,netdev=%v,mac=%v", tap, mac))
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
		if v == nil {
			return
		}
		log.Info("VM %v received asynchronous message: %v", vm.Id, v)
	}
}

func cliVMQemu(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: process("qemu"),
		}
	} else if len(c.Args) == 1 {
		externalProcesses["qemu"] = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_qemu takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMMemory(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.Memory,
		}
	} else if len(c.Args) == 1 {
		info.Memory = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_memory takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMVCPUs(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.Vcpus,
		}
	} else if len(c.Args) == 1 {
		info.Vcpus = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_vcpus takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMDisk(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.DiskPath,
		}
	} else if len(c.Args) == 1 {
		info.DiskPath = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_disk takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMCdrom(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.CdromPath,
		}
	} else if len(c.Args) == 1 {
		info.CdromPath = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_cdrom takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMKernel(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.KernelPath,
		}
	} else if len(c.Args) == 1 {
		info.KernelPath = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_kernel takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMInitrd(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.InitrdPath,
		}
	} else if len(c.Args) == 1 {
		info.InitrdPath = c.Args[0]
	} else {
		return cliResponse{
			Error: "vm_initrd takes only one argument",
		}
	}
	return cliResponse{}
}

func cliVMQemuAppend(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: strings.Join(info.QemuAppend, " "),
		}
	} else {
		info.QemuAppend = c.Args
	}
	return cliResponse{}
}

func cliVMAppend(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Response: info.Append,
		}
	} else {
		info.Append = strings.Join(c.Args, " ")
	}
	return cliResponse{}
}

func cliVMNet(c cliCommand) cliResponse {
	r := cliResponse{}
	if len(c.Args) == 0 {
		return cliResponse{
			Response: fmt.Sprintf("%v\n", info.Networks),
		}
	} else {
		info.Networks = []int{}
		for _, lan := range c.Args {
			val, err := strconv.Atoi(lan)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			err, ok := currentBridge.LanCreate(val)
			if !ok {
				return cliResponse{
					Error: err.Error(),
				}
			}
			info.Networks = append(info.Networks, val)
		}
	}
	return r
}
