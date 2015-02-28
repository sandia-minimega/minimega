// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"os/exec"
	"qmp"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

type vmInfo struct {
	Lock sync.Mutex

	Id int

	Name       string
	Memory     string // memory for the vm, in megabytes
	Vcpus      string // number of virtual cpus
	CdromPath  string
	KernelPath string
	InitrdPath string
	Append     string
	Snapshot   bool
	UUID       string

	DiskPaths  []string
	QemuAppend []string // extra arguments for QEMU
	Networks   []int    // ordered list of networks (matches 1-1 with Taps)
	bridges    []string // list of bridges, if specified. Unspecified bridges will contain ""
	taps       []string // list of taps associated with this vm
	macs       []string // ordered list of macs (matches 1-1 with Taps, Networks)
	netDrivers []string // optional non-e1000 driver

	State        VmState // one of the VM_ states listed above
	instancePath string

	PID  int
	q    qmp.Conn  // qmp connection for this vm
	Kill chan bool // kill channel to signal to shut a vm down

	Hotplug map[int]string

	Tags map[string]string // Additional information
}

func (vm *vmInfo) start() error {
	stateMask := VM_PAUSED | VM_BUILDING | VM_QUIT
	if vm.State&stateMask == 0 {
		return nil
	}

	if vm.State == VM_QUIT {
		log.Info("restarting VM: %v", vm.Id)
		ack := make(chan int)
		go vm.launchOne(ack)
		log.Debugln("ack restarted VM %v", <-ack)
	}

	log.Info("starting VM: %v", vm.Id)
	err := vm.q.Start()
	if err != nil {
		vm.state(VM_ERROR)
	} else {
		vm.state(VM_RUNNING)
	}

	return err
}

func (vm *vmInfo) stop() error {
	if vm.State != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.Id)
	}

	log.Info("stopping VM: %v", vm.Id)
	err := vm.q.Stop()
	if err == nil {
		vm.state(VM_PAUSED)
	}

	return err
}

func (info *vmInfo) Copy() *vmInfo {
	// makes deep copy of info and returns reference to new vmInfo struct
	newInfo := new(vmInfo)
	newInfo.Id = info.Id
	newInfo.Name = info.Name
	newInfo.Memory = info.Memory
	newInfo.Vcpus = info.Vcpus
	newInfo.DiskPaths = make([]string, len(info.DiskPaths))
	copy(newInfo.DiskPaths, info.DiskPaths)
	newInfo.CdromPath = info.CdromPath
	newInfo.KernelPath = info.KernelPath
	newInfo.InitrdPath = info.InitrdPath
	newInfo.Append = info.Append
	newInfo.QemuAppend = make([]string, len(info.QemuAppend))
	copy(newInfo.QemuAppend, info.QemuAppend)
	newInfo.State = info.State
	// Kill isn't allocated until later in launch()
	newInfo.instancePath = info.instancePath
	// q isn't allocated until launchOne()
	newInfo.bridges = make([]string, len(info.bridges))
	copy(newInfo.bridges, info.bridges)
	newInfo.taps = make([]string, len(info.taps))
	copy(newInfo.taps, info.taps)
	newInfo.Networks = make([]int, len(info.Networks))
	copy(newInfo.Networks, info.Networks)
	newInfo.macs = make([]string, len(info.macs))
	copy(newInfo.macs, info.macs)
	newInfo.netDrivers = make([]string, len(info.netDrivers))
	copy(newInfo.netDrivers, info.netDrivers)
	newInfo.Snapshot = info.Snapshot
	newInfo.UUID = info.UUID
	// Hotplug and tags aren't allocated until later in launch()
	return newInfo
}

func (vm *vmInfo) configToString() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current VM configuration:")
	fmt.Fprintf(w, "Memory:\t%v\n", vm.Memory)
	fmt.Fprintf(w, "VCPUS:\t%v\n", vm.Vcpus)
	fmt.Fprintf(w, "Disk Paths:\t%v\n", vm.DiskPaths)
	fmt.Fprintf(w, "CDROM Path:\t%v\n", vm.CdromPath)
	fmt.Fprintf(w, "Kernel Path:\t%v\n", vm.KernelPath)
	fmt.Fprintf(w, "Initrd Path:\t%v\n", vm.InitrdPath)
	fmt.Fprintf(w, "Kernel Append:\t%v\n", vm.Append)
	fmt.Fprintf(w, "QEMU Path:\t%v\n", process("qemu"))
	fmt.Fprintf(w, "QEMU Append:\t%v\n", vm.QemuAppend)
	fmt.Fprintf(w, "Snapshot:\t%v\n", vm.Snapshot)
	fmt.Fprintf(w, "Networks:\t%v\n", vm.networkString())
	fmt.Fprintf(w, "UUID:\t%v\n", vm.UUID)
	w.Flush()
	return o.String()
}

func (vm *vmInfo) QMPRaw(input string) (string, error) {
	return vm.q.Raw(input)
}

func (vm *vmInfo) networkString() string {
	s := "["
	for i, vlan := range vm.Networks {
		if vm.bridges[i] != "" {
			s += vm.bridges[i] + ","
		}
		s += strconv.Itoa(vlan)
		if vm.macs[i] != "" {
			s += "," + vm.macs[i]
		}
		if i+1 < len(vm.Networks) {
			s += " "
		}
	}
	s += "]"
	return s
}

func (vm *vmInfo) launchPreamble(ack chan int) bool {
	// check if the vm has a conflict with the disk or mac address of another vm
	// build state of currently running system
	macMap := map[string]bool{}
	selfMacMap := map[string]bool{}
	diskSnapshotted := map[string]bool{}
	diskPersistent := map[string]bool{}

	vmLock.Lock()
	defer vmLock.Unlock()

	vm.instancePath = *f_base + strconv.Itoa(vm.Id) + "/"
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
	for _, mac := range vm.macs {
		if mac == "" { // don't worry about empty mac addresses
			continue
		}

		_, ok := selfMacMap[mac]
		if ok { // if this vm specified the same mac address for two interfaces
			log.Errorln("Cannot specify the same mac address for two interfaces")
			vm.state(VM_ERROR)
			ack <- vm.Id // signal that this vm is "done" launching
			return false
		}
		selfMacMap[mac] = true
	}

	// populate macMap, diskSnapshotted, and diskPersistent
	for _, vm2 := range vms.vms {
		if vm == vm2 { // ignore this vm
			continue
		}

		s := vm2.getState()
		stateMask := VM_BUILDING | VM_RUNNING | VM_PAUSED
		vmIsActive := (s&stateMask != 0)

		if vmIsActive {
			// populate mac addresses set
			for _, mac := range vm2.macs {
				macMap[mac] = true
			}

			// populate disk sets
			if len(vm2.DiskPaths) != 0 {
				for _, diskpath := range vm2.DiskPaths {
					if vm2.Snapshot {
						diskSnapshotted[diskpath] = true
					} else {
						diskPersistent[diskpath] = true
					}
				}
			}
		}
	}

	// check for mac address conflicts and fill in unspecified mac addresses without conflict
	for i, mac := range vm.macs {
		if mac == "" { // create mac addresses where unspecified
			existsOther, existsSelf, newMac := true, true, "" // entry condition/initialization
			for existsOther || existsSelf {                   // loop until we generate a random mac that doesn't conflict (already exist)
				newMac = randomMac()               // generate a new mac address
				_, existsOther = macMap[newMac]    // check it against the set of mac addresses from other vms
				_, existsSelf = selfMacMap[newMac] // check it against the set of mac addresses specified from this vm
			}

			vm.macs[i] = newMac       // set the unspecified mac address
			selfMacMap[newMac] = true // add this mac to the set of mac addresses for this vm
		}
	}

	// check for disk conflict
	for _, diskPath := range vm.DiskPaths {
		_, existsSnapshotted := diskSnapshotted[diskPath]                    // check if another vm is using this disk in snapshot mode
		_, existsPersistent := diskPersistent[diskPath]                      // check if another vm is using this disk in persistent mode (snapshot=false)
		if existsPersistent || (vm.Snapshot == false && existsSnapshotted) { // if we have a disk conflict
			log.Error("disk path %v is already in use by another vm.", diskPath)
			vm.state(VM_ERROR)
			ack <- vm.Id
			return false
		}
	}

	return true
}

func (vm *vmInfo) launchOne(ack chan int) {
	log.Info("launching vm: %v", vm.Id)

	s := vm.getState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT && !vm.launchPreamble(ack) {
		return
	}

	vm.state(VM_BUILDING)

	// write the config for this vm
	config := vm.configToString()
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

	var args []string
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	var cmd *exec.Cmd
	var waitChan = make(chan int)

	// clear taps, we may have come from the quit state
	vm.taps = []string{}

	// create and add taps if we are associated with any networks
	for i, lan := range vm.Networks {
		b, err := getBridge(vm.bridges[i])
		if err != nil {
			log.Error("get bridge: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.Id
			return
		}
		tap, err := b.TapCreate(lan)
		if err != nil {
			log.Error("create tap: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.Id
			return
		}
		vm.taps = append(vm.taps, tap)
	}

	if len(vm.Networks) > 0 {
		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(vm.taps, "\n")), 0666)
		if err != nil {
			log.Error("write instance taps file: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.Id
			return
		}
	}

	args = vm.vmGetArgs(true)
	args = ParseQemuOverrides(args)
	log.Debug("final qemu args: %#v", args)

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
		log.Error("start qemu: %v %v", err, sErr.String())
		vm.state(VM_ERROR)
		ack <- vm.Id
		return
	}

	vm.PID = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.Id, vm.PID)

	vm.CheckAffinity()

	go func() {
		err := cmd.Wait()
		vm.state(VM_QUIT)
		if err != nil {
			if err.Error() != "signal: killed" { // because we killed it
				log.Error("kill qemu: %v %v", err, sErr.String())
				vm.state(VM_ERROR)
			}
		}
		waitChan <- vm.Id
	}()

	// we can't just return on error at this point because we'll leave dangling goroutines, we have to clean up on failure
	sendKillAck := false

	// connect to qmp
	connected := false
	for count := 0; count < QMP_CONNECT_RETRY; count++ {
		vm.q, err = qmp.Dial(vm.qmpPath())
		if err == nil {
			connected = true
			break
		}
		time.Sleep(QMP_CONNECT_DELAY * time.Millisecond)
	}

	if !connected {
		log.Error("vm %v failed to connect to qmp: %v", vm.Id, err)
		vm.state(VM_ERROR)
		cmd.Process.Kill()
		<-waitChan
		ack <- vm.Id
	} else {
		go vm.asyncLogger()

		ack <- vm.Id

		select {
		case <-waitChan:
			log.Info("VM %v exited", vm.Id)
		case <-vm.Kill:
			log.Info("Killing VM %v", vm.Id)
			cmd.Process.Kill()
			<-waitChan
			sendKillAck = true // wait to ack until we've cleaned up
		}
	}

	for i, l := range vm.Networks {
		b, err := getBridge(vm.bridges[i])
		if err != nil {
			log.Error("get bridge: %v", err)
		} else {
			b.TapDestroy(l, vm.taps[i])
		}
	}

	if sendKillAck {
		killAck <- vm.Id
	}
}

func (vm *vmInfo) getState() VmState {
	vm.Lock.Lock()
	defer vm.Lock.Unlock()

	return vm.State
}

// update the vm state, and write the state to file
func (vm *vmInfo) state(s VmState) {
	vm.Lock.Lock()
	defer vm.Lock.Unlock()

	vm.State = s
	err := ioutil.WriteFile(vm.instancePath+"state", []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}

// return the path to the qmp socket
func (vm *vmInfo) qmpPath() string {
	return vm.instancePath + "qmp"
}

// build the horribly long qemu argument string
func (vm *vmInfo) vmGetArgs(commit bool) []string {
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

	args = append(args, "-device")
	args = append(args, "virtio-serial")

	args = append(args, "-chardev")
	args = append(args, "socket,id=charserial0,path="+vm.instancePath+"serial,server,nowait")

	args = append(args, "-device")
	args = append(args, "virtserialport,chardev=charserial0,id=serial0,name=serial0")

	args = append(args, "-pidfile")
	args = append(args, vm.instancePath+"qemu.pid")

	args = append(args, "-k")
	args = append(args, "en-us")

	args = append(args, "-cpu")
	args = append(args, "host")

	args = append(args, "-net")
	args = append(args, "none")

	args = append(args, "-S")

	if len(vm.DiskPaths) != 0 {
		for _, diskPath := range vm.DiskPaths {
			args = append(args, "-drive")
			args = append(args, "file="+diskPath+",media=disk")
		}
	}

	if vm.Snapshot {
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

	bus := 1
	addr := 1
	args = append(args, fmt.Sprintf("-device"))
	args = append(args, fmt.Sprintf("pci-bridge,id=pci.%v,chassis_nr=%v", bus, bus))
	for i, tap := range vm.taps {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", tap, tap))
		args = append(args, "-device")
		if commit {
			b, err := getBridge(vm.bridges[i])
			if err != nil {
				log.Error("get bridge: %v", err)
			}
			b.iml.AddMac(vm.macs[i])
		}
		args = append(args, fmt.Sprintf("driver=%v,netdev=%v,mac=%v,bus=pci.%v,addr=0x%x", vm.netDrivers[i], tap, vm.macs[i], bus, addr))
		addr++
		if addr == 32 {
			addr = 1
			bus++
			args = append(args, fmt.Sprintf("-device"))
			args = append(args, fmt.Sprintf("pci-bridge,id=pci.%v,chassis_nr=%v", bus, bus))
		}
	}

	// hook for hugepage support
	if hugepagesMountPath != "" {
		args = append(args, "-mem-info")
		args = append(args, hugepagesMountPath)
	}

	if len(vm.QemuAppend) > 0 {
		args = append(args, vm.QemuAppend...)
	}

	args = append(args, "-uuid")
	args = append(args, vm.UUID)

	log.Info("args for vm %v is: %#v", vm.Id, args)
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

func (vm *vmInfo) hotplugRemove(id int) error {
	hid := fmt.Sprintf("hotplug%v", id)
	log.Debugln("hotplug id:", hid)
	if _, ok := vm.Hotplug[id]; !ok {
		return errors.New("no such hotplug device id")
	}

	resp, err := vm.q.USBDeviceDel(hid)
	if err != nil {
		return err
	}

	log.Debugln("hotplug usb device del response:", resp)
	resp, err = vm.q.DriveDel(hid)
	if err != nil {
		return err
	}

	log.Debugln("hotplug usb drive del response:", resp)
	delete(vm.Hotplug, id)
	return nil
}
