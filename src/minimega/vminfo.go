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
	"path/filepath"
	"qmp"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

type vmInfo struct {
	ID int

	Name       string
	Memory     string // memory for the vm, in megabytes
	Vcpus      string // number of virtual cpus
	CdromPath  string
	KernelPath string
	InitrdPath string
	Append     string
	Snapshot   bool
	UUID       string

	MigratePath string
	DiskPaths   []string
	QemuAppend  []string // extra arguments for QEMU
	Networks    []int    // ordered list of networks (matches 1-1 with Taps)
	Bridges     []string // list of bridges, if specified. Unspecified bridges will contain ""
	Taps        []string // list of taps associated with this vm
	Macs        []string // ordered list of macs (matches 1-1 with Taps, Networks)
	NetDrivers  []string // optional non-e1000 driver

	State VmState // one of the VM_ states listed above

	Hotplug map[int]string
	Tags    map[string]string // Additional information

	// Internal variables
	instancePath string
	lock         sync.Mutex
	pid          int
	q            qmp.Conn  // qmp connection for this vm
	kill         chan bool // kill channel to signal to shut a vm down
}

func (vm *vmInfo) start() error {
	stateMask := VM_PAUSED | VM_BUILDING | VM_QUIT | VM_ERROR
	if vm.State&stateMask == 0 {
		return nil
	}

	if vm.State == VM_QUIT || vm.State == VM_ERROR {
		log.Info("restarting VM: %v", vm.ID)
		ack := make(chan int)
		go vm.launchOne(ack)
		log.Debug("ack restarted VM %v", <-ack)
	}

	log.Info("starting VM: %v", vm.ID)
	err := vm.q.Start()
	if err != nil {
		log.Errorln(err)
		if err != qmp.ERR_READY {
			vm.state(VM_ERROR)
		}
	} else {
		vm.state(VM_RUNNING)
	}

	return err
}

func (vm *vmInfo) stop() error {
	if vm.State != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.ID)
	}

	log.Info("stopping VM: %v", vm.ID)
	err := vm.q.Stop()
	if err == nil {
		vm.state(VM_PAUSED)
	}

	return err
}

func (info *vmInfo) Copy() *vmInfo {
	// makes deep copy of info and returns reference to new vmInfo struct
	newInfo := new(vmInfo)
	newInfo.ID = info.ID
	newInfo.Name = info.Name
	newInfo.Memory = info.Memory
	newInfo.Vcpus = info.Vcpus
	newInfo.MigratePath = info.MigratePath
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
	newInfo.Bridges = make([]string, len(info.Bridges))
	copy(newInfo.Bridges, info.Bridges)
	newInfo.Taps = make([]string, len(info.Taps))
	copy(newInfo.Taps, info.Taps)
	newInfo.Networks = make([]int, len(info.Networks))
	copy(newInfo.Networks, info.Networks)
	newInfo.Macs = make([]string, len(info.Macs))
	copy(newInfo.Macs, info.Macs)
	newInfo.NetDrivers = make([]string, len(info.NetDrivers))
	copy(newInfo.NetDrivers, info.NetDrivers)
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
	fmt.Fprintf(w, "Migrate Path:\t%v\n", vm.MigratePath)
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

func (vm *vmInfo) Migrate(filename string) error {
	path := filepath.Join(*f_iomBase, filename)
	return vm.q.MigrateDisk(path)
}

func (vm *vmInfo) QueryMigrate() (string, float64, error) {
	var status string
	var completed float64

	r, err := vm.q.QueryMigrate()
	if err != nil {
		return "", 0.0, err
	}

	// find the status
	if s, ok := r["status"]; ok {
		status = s.(string)
	} else {
		return status, completed, fmt.Errorf("could not decode status: %v", r)
	}

	var ram map[string]interface{}
	switch status {
	case "completed":
		completed = 100.0
		return status, completed, nil
	case "failed":
		return status, completed, nil
	case "active":
		if e, ok := r["ram"]; !ok {
			return status, completed, fmt.Errorf("could not decode ram segment: %v", e)
		} else {
			switch e.(type) {
			case map[string]interface{}:
				ram = e.(map[string]interface{})
			default:
				return status, completed, fmt.Errorf("invalid ram type: %v", e)
			}
		}
	}

	total := ram["total"].(float64)
	transferred := ram["transferred"].(float64)

	if total == 0.0 {
		return status, completed, fmt.Errorf("zero total ram!")
	}

	completed = transferred / total

	return status, completed, nil
}

func (vm *vmInfo) networkString() string {
	s := "["
	for i, vlan := range vm.Networks {
		if vm.Bridges[i] != "" {
			s += vm.Bridges[i] + ","
		}
		s += strconv.Itoa(vlan)
		if vm.Macs[i] != "" {
			s += "," + vm.Macs[i]
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

	vm.instancePath = *f_base + strconv.Itoa(vm.ID) + "/"
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
	for _, mac := range vm.Macs {
		if mac == "" { // don't worry about empty mac addresses
			continue
		}

		_, ok := selfMacMap[mac]
		if ok { // if this vm specified the same mac address for two interfaces
			log.Errorln("Cannot specify the same mac address for two interfaces")
			vm.state(VM_ERROR)
			ack <- vm.ID // signal that this vm is "done" launching
			return false
		}
		selfMacMap[mac] = true
	}

	// populate macMap, diskSnapshotted, and diskPersistent
	for _, vm2 := range vms {
		if vm == vm2 { // ignore this vm
			continue
		}

		s := vm2.getState()
		stateMask := VM_BUILDING | VM_RUNNING | VM_PAUSED
		vmIsActive := (s&stateMask != 0)

		if vmIsActive {
			// populate mac addresses set
			for _, mac := range vm2.Macs {
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
	for i, mac := range vm.Macs {
		if mac == "" { // create mac addresses where unspecified
			existsOther, existsSelf, newMac := true, true, "" // entry condition/initialization
			for existsOther || existsSelf {                   // loop until we generate a random mac that doesn't conflict (already exist)
				newMac = randomMac()               // generate a new mac address
				_, existsOther = macMap[newMac]    // check it against the set of mac addresses from other vms
				_, existsSelf = selfMacMap[newMac] // check it against the set of mac addresses specified from this vm
			}

			vm.Macs[i] = newMac       // set the unspecified mac address
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
			ack <- vm.ID
			return false
		}
	}

	return true
}

func (vm *vmInfo) launchOne(ack chan int) {
	log.Info("launching vm: %v", vm.ID)

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
	vm.Taps = []string{}

	// create and add taps if we are associated with any networks
	for i, lan := range vm.Networks {
		b, err := getBridge(vm.Bridges[i])
		if err != nil {
			log.Error("get bridge: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.ID
			return
		}
		tap, err := b.TapCreate(lan)
		if err != nil {
			log.Error("create tap: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.ID
			return
		}
		vm.Taps = append(vm.Taps, tap)
	}

	if len(vm.Networks) > 0 {
		err := ioutil.WriteFile(vm.instancePath+"taps", []byte(strings.Join(vm.Taps, "\n")), 0666)
		if err != nil {
			log.Error("write instance taps file: %v", err)
			vm.state(VM_ERROR)
			ack <- vm.ID
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
		ack <- vm.ID
		return
	}

	vm.pid = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.ID, vm.pid)

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
		waitChan <- vm.ID
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
		delay := QMP_CONNECT_DELAY * time.Millisecond
		log.Info("qmp dial to %v : %v, redialing in %v", vm.ID, err, delay)
		time.Sleep(delay)
	}

	if !connected {
		log.Error("vm %v failed to connect to qmp: %v", vm.ID, err)
		vm.state(VM_ERROR)
		cmd.Process.Kill()
		<-waitChan
		ack <- vm.ID
	} else {
		log.Debug("qmp dial to %v successful", vm.ID)

		go vm.asyncLogger()

		ack <- vm.ID

		select {
		case <-waitChan:
			log.Info("VM %v exited", vm.ID)
		case <-vm.kill:
			log.Info("Killing VM %v", vm.ID)
			cmd.Process.Kill()
			<-waitChan
			sendKillAck = true // wait to ack until we've cleaned up
		}
	}

	for i, l := range vm.Networks {
		b, err := getBridge(vm.Bridges[i])
		if err != nil {
			log.Error("get bridge: %v", err)
		} else {
			b.TapDestroy(l, vm.Taps[i])
		}
	}

	if sendKillAck {
		killAck <- vm.ID
	}
}

func (vm *vmInfo) getState() VmState {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.State
}

// update the vm state, and write the state to file
func (vm *vmInfo) state(s VmState) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

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

	sId := strconv.Itoa(vm.ID)

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

	if vm.MigratePath != "" {
		args = append(args, "-incoming")
		args = append(args, fmt.Sprintf("exec:cat %v", vm.MigratePath))
	}

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
	for i, tap := range vm.Taps {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", tap, tap))
		args = append(args, "-device")
		if commit {
			b, err := getBridge(vm.Bridges[i])
			if err != nil {
				log.Error("get bridge: %v", err)
			}
			b.iml.AddMac(vm.Macs[i])
		}
		args = append(args, fmt.Sprintf("driver=%v,netdev=%v,mac=%v,bus=pci.%v,addr=0x%x", vm.NetDrivers[i], tap, vm.Macs[i], bus, addr))
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

	log.Info("args for vm %v is: %#v", vm.ID, args)
	return args
}

// log any asynchronous messages, such as vnc connects, to log.Info
func (vm *vmInfo) asyncLogger() {
	for {
		v := vm.q.Message()
		if v == nil {
			return
		}
		log.Info("VM %v received asynchronous message: %v", vm.ID, v)
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

func (vm *vmInfo) info(masks []string) ([]string, error) {
	res := make([]string, 0, len(masks))

	for _, mask := range masks {
		switch mask {
		case "id":
			res = append(res, fmt.Sprintf("%v", vm.ID))
		case "name":
			res = append(res, fmt.Sprintf("%v", vm.Name))
		case "memory":
			res = append(res, fmt.Sprintf("%v", vm.Memory))
		case "vcpus":
			res = append(res, fmt.Sprintf("%v", vm.Vcpus))
		case "state":
			res = append(res, vm.State.String())
		case "migrate":
			res = append(res, fmt.Sprintf("%v", vm.MigratePath))
		case "disk":
			res = append(res, fmt.Sprintf("%v", vm.DiskPaths))
		case "snapshot":
			res = append(res, fmt.Sprintf("%v", vm.Snapshot))
		case "initrd":
			res = append(res, fmt.Sprintf("%v", vm.InitrdPath))
		case "kernel":
			res = append(res, fmt.Sprintf("%v", vm.KernelPath))
		case "cdrom":
			res = append(res, fmt.Sprintf("%v", vm.CdromPath))
		case "append":
			res = append(res, fmt.Sprintf("%v", vm.Append))
		case "bridge":
			res = append(res, fmt.Sprintf("%v", vm.Bridges))
		case "tap":
			res = append(res, fmt.Sprintf("%v", vm.Taps))
		case "bandwidth":
			var bw []string
			bandwidthLock.Lock()
			for _, v := range vm.Taps {
				t := bandwidthStats[v]
				if t == nil {
					bw = append(bw, "0.0/0.0")
				} else {
					bw = append(bw, fmt.Sprintf("%v", t))
				}
			}
			bandwidthLock.Unlock()
			res = append(res, fmt.Sprintf("%v", bw))
		case "mac":
			res = append(res, fmt.Sprintf("%v", vm.Macs))
		case "tags":
			res = append(res, fmt.Sprintf("%v", vm.Tags))
		case "ip":
			var ips []string
			for bIndex, m := range vm.Macs {
				// TODO: This won't work if it's being run from a different host...
				b, err := getBridge(vm.Bridges[bIndex])
				if err != nil {
					log.Errorln(err)
					continue
				}
				ip := b.GetIPFromMac(m)
				if ip != nil {
					ips = append(ips, ip.IP4)
				}
			}
			res = append(res, fmt.Sprintf("%v", ips))
		case "ip6":
			var ips []string
			for bIndex, m := range vm.Macs {
				// TODO: This won't work if it's being run from a different host...
				b, err := getBridge(vm.Bridges[bIndex])
				if err != nil {
					log.Errorln(err)
					continue
				}
				ip := b.GetIPFromMac(m)
				if ip != nil {
					ips = append(ips, ip.IP6)
				}
			}
			res = append(res, fmt.Sprintf("%v", ips))
		case "vlan":
			var vlans []string
			for _, v := range vm.Networks {
				if v == -1 {
					vlans = append(vlans, "disconnected")
				} else {
					vlans = append(vlans, fmt.Sprintf("%v", v))
				}
			}
			res = append(res, fmt.Sprintf("%v", vlans))
		case "uuid":
			res = append(res, fmt.Sprintf("%v", vm.UUID))
		case "cc_active":
			// TODO: This won't work if it's being run from a different host...
			activeClients := ccClients()
			res = append(res, fmt.Sprintf("%v", activeClients[vm.UUID]))
		default:
			return nil, fmt.Errorf("invalid mask: %s", mask)
		}
	}

	return res, nil
}
