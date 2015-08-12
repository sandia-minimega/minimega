// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"ipmac"
	"math/rand"
	log "minilog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"qmp"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

const (
	DEV_PER_BUS    = 32
	DEV_PER_VIRTIO = 30 // Max of 30 virtio ports/device (0 and 32 are reserved)

	DefaultKVMCPU = "host"
)

type KVMConfig struct {
	Append     string
	CdromPath  string
	InitrdPath string
	KernelPath string

	CPU string // not user configurable, yet.

	MigratePath string
	UUID        string

	Snapshot bool

	SerialPorts int
	VirtioPorts int

	DiskPaths  []string
	QemuAppend []string // extra arguments for QEMU
}

type KvmVM struct {
	BaseVM    // embed
	KVMConfig // embed

	// Internal variables
	hotplug map[int]string

	pid int
	q   qmp.Conn // qmp connection for this vm

	ActiveCC bool // Whether CC is active, updated by calling UpdateCCActive
}

type qemuOverride struct {
	match string
	repl  string
}

var (
	QemuOverrides      map[int]*qemuOverride
	qemuOverrideIdChan chan int
)

// Ensure that vmKVM implements the VM interface
var _ VM = (*KvmVM)(nil)

// Valid names for output masks for vm info kvm, in preferred output order
var kvmMasks = []string{
	"id", "name", "state", "memory", "vcpus", "type", "vlan", "bridge", "tap",
	"mac", "ip", "ip6", "bandwidth", "migrate", "disk", "snapshot", "initrd",
	"kernel", "cdrom", "append", "uuid", "cc_active", "tags",
}

func init() {
	QemuOverrides = make(map[int]*qemuOverride)
	qemuOverrideIdChan = makeIDChan()

	// Reset everything to default
	for _, fns := range kvmConfigFns {
		fns.Clear(&vmConfig.KVMConfig)
	}
}

// Copy makes a deep copy and returns reference to the new struct.
func (old *KVMConfig) Copy() *KVMConfig {
	res := new(KVMConfig)

	// Copy all fields
	*res = *old

	// Make deep copy of slices
	res.DiskPaths = make([]string, len(old.DiskPaths))
	copy(res.DiskPaths, old.DiskPaths)
	res.QemuAppend = make([]string, len(old.QemuAppend))
	copy(res.QemuAppend, old.QemuAppend)

	return res
}

func NewKVM(name string) *KvmVM {
	vm := new(KvmVM)

	vm.BaseVM = *NewVM(name)
	vm.Type = KVM

	vm.KVMConfig = *vmConfig.KVMConfig.Copy() // deep-copy configured fields

	vm.hotplug = make(map[int]string)

	return vm
}

// Launch a new KVM VM.
func (vm *KvmVM) Launch(ack chan int) error {
	go vm.launch(ack)

	return nil
}

func (vm *KvmVM) Config() *BaseConfig {
	return &vm.BaseConfig
}

func (vm *KvmVM) UpdateCCActive() {
	vm.ActiveCC = ccHasClient(vm.UUID)
}

func (vm *KvmVM) Start() error {
	s := vm.GetState()

	stateMask := VM_PAUSED | VM_BUILDING | VM_QUIT | VM_ERROR
	if s&stateMask == 0 {
		return nil
	}

	if s == VM_QUIT || s == VM_ERROR {
		log.Info("restarting VM: %v", vm.ID)
		// Create a new channel since we closed the other one to indicate that
		// the VM should quit.
		vm.kill = make(chan bool)
		ack := make(chan int)
		go vm.launch(ack)
		log.Debug("ack restarted VM %v", <-ack)
	}

	log.Info("starting VM: %v", vm.ID)
	err := vm.q.Start()
	if err != nil {
		log.Errorln(err)
		if err != qmp.ERR_READY {
			vm.setState(VM_ERROR)
		}
	} else {
		vm.setState(VM_RUNNING)
	}

	return err
}

func (vm *KvmVM) Stop() error {
	if vm.GetState() != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.ID)
	}

	log.Info("stopping VM: %v", vm.ID)
	err := vm.q.Stop()
	if err == nil {
		vm.setState(VM_PAUSED)
	}

	return err
}

func (vm *KvmVM) String() string {
	return fmt.Sprintf("%s:%d:kvm", hostname, vm.ID)
}

func (vm *KvmVM) Info(mask string) (string, error) {
	// If it's a field handled by the baseVM, use it.
	if v, err := vm.BaseVM.info(mask); err == nil {
		return v, nil
	}

	// If it's a configurable field, use the Print fn.
	if fns, ok := kvmConfigFns[mask]; ok {
		return fns.Print(&vm.KVMConfig), nil
	}

	switch mask {
	case "cc_active":
		return fmt.Sprintf("%v", vm.ActiveCC), nil
	}

	return "", fmt.Errorf("invalid mask: %s", mask)
}

func (vm *KVMConfig) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "Current KVM configuration:")
	fmt.Fprintf(w, "Migrate Path:\t%v\n", vm.MigratePath)
	fmt.Fprintf(w, "Disk Paths:\t%v\n", vm.DiskPaths)
	fmt.Fprintf(w, "CDROM Path:\t%v\n", vm.CdromPath)
	fmt.Fprintf(w, "Kernel Path:\t%v\n", vm.KernelPath)
	fmt.Fprintf(w, "Initrd Path:\t%v\n", vm.InitrdPath)
	fmt.Fprintf(w, "Kernel Append:\t%v\n", vm.Append)
	fmt.Fprintf(w, "QEMU Path:\t%v\n", process("qemu"))
	fmt.Fprintf(w, "QEMU Append:\t%v\n", vm.QemuAppend)
	fmt.Fprintf(w, "Snapshot:\t%v\n", vm.Snapshot)
	fmt.Fprintf(w, "UUID:\t%v\n", vm.UUID)
	fmt.Fprintf(w, "SerialPorts:\t%v\n", vm.SerialPorts)
	fmt.Fprintf(w, "Virtio-SerialPorts:\t%v\n", vm.VirtioPorts)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (vm *KvmVM) QMPRaw(input string) (string, error) {
	return vm.q.Raw(input)
}

func (vm *KvmVM) Migrate(filename string) error {
	path := filepath.Join(*f_iomBase, filename)
	return vm.q.MigrateDisk(path)
}

func (vm *KvmVM) QueryMigrate() (string, float64, error) {
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

func (vm *KvmVM) Screenshot(size int) ([]byte, error) {
	suffix := rand.New(rand.NewSource(time.Now().UnixNano())).Int31()
	tmp := filepath.Join(os.TempDir(), fmt.Sprintf("minimega_screenshot_%v", suffix))

	// We have to write this out to a file, because QMP
	err := vm.q.Screendump(tmp)
	if err != nil {
		return nil, err
	}

	ppmFile, err := ioutil.ReadFile(tmp)
	if err != nil {
		return nil, err
	}

	pngResult, err := ppmToPng(ppmFile, size)
	if err != nil {
		return nil, err
	}

	err = os.Remove(tmp)
	if err != nil {
		return nil, err
	}

	return pngResult, nil

}

func (vm *KvmVM) launchPreamble(ack chan int) bool {
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

			// TODO: Check non-kvm as well?
			if vm2, ok := vm2.(*KvmVM); ok {
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
	for _, diskPath := range vm.DiskPaths {
		_, existsSnapshotted := diskSnapshotted[diskPath]                    // check if another vm is using this disk in snapshot mode
		_, existsPersistent := diskPersistent[diskPath]                      // check if another vm is using this disk in persistent mode (snapshot=false)
		if existsPersistent || (vm.Snapshot == false && existsSnapshotted) { // if we have a disk conflict
			log.Error("disk path %v is already in use by another vm.", diskPath)
			vm.setState(VM_ERROR)
			ack <- vm.ID
			return false
		}
	}

	return true
}

func (vm *KvmVM) launch(ack chan int) {
	log.Info("launching vm: %v", vm.ID)

	s := vm.GetState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT && !vm.launchPreamble(ack) {
		return
	}

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

	var args []string
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
		go func(vm *KvmVM, net *NetConfig) {
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

	vmConfig := VMConfig{BaseConfig: vm.BaseConfig, KVMConfig: vm.KVMConfig}
	args = vmConfig.qemuArgs(vm.ID, vm.instancePath)
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
		vm.setState(VM_ERROR)
		ack <- vm.ID
		return
	}

	vm.pid = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.ID, vm.pid)

	vm.CheckAffinity()

	go func() {
		err := cmd.Wait()
		vm.setState(VM_QUIT)
		if err != nil {
			if err.Error() != "signal: killed" { // because we killed it
				log.Error("kill qemu: %v %v", err, sErr.String())
				vm.setState(VM_ERROR)
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
		log.Info("qmp dial to %v : %v, redialing in %v", vm.GetID(), err, delay)
		time.Sleep(delay)
	}

	if !connected {
		log.Error("vm %v failed to connect to qmp: %v", vm.ID, err)
		vm.setState(VM_ERROR)
		cmd.Process.Kill()
		<-waitChan
		ack <- vm.ID
	} else {
		log.Debug("qmp dial to %v successful", vm.GetID())

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

	for i := range vm.Networks {
		if err := vm.NetworkDisconnect(i); err != nil {
			log.Error("unable to disconnect VM: %v %v %v", vm.ID, i, err)
		}
	}

	if sendKillAck {
		log.Info("sending kill ack %v", vm.ID)
		killAck <- vm.ID
	}
}

// update the vm state, and write the state to file
func (vm *KvmVM) setState(s VMState) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.State = s
	err := ioutil.WriteFile(vm.instancePath+"state", []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}

// return the path to the qmp socket
func (vm *KvmVM) qmpPath() string {
	return vm.instancePath + "qmp"
}

// build the horribly long qemu argument string
func (vm VMConfig) qemuArgs(id int, vmPath string) []string {
	var args []string

	sId := strconv.Itoa(id)
	qmpPath := path.Join(vmPath, "qmp")

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
	args = append(args, "unix:"+qmpPath+",server")

	args = append(args, "-vga")
	args = append(args, "cirrus")

	args = append(args, "-rtc")
	args = append(args, "clock=vm,base=utc")

	args = append(args, "-device")
	args = append(args, "virtio-serial")

	// this is non-virtio serial ports
	// for virtio-serial, look below near the net code
	for i := 0; i < vm.SerialPorts; i++ {
		args = append(args, "-chardev")
		args = append(args, fmt.Sprintf("socket,id=charserial%v,path=%vserial%v,server,nowait", i, vmPath, i))

		args = append(args, "-device")
		args = append(args, fmt.Sprintf("isa-serial,chardev=charserial%v,id=serial%v", i, i))
	}

	args = append(args, "-pidfile")
	args = append(args, path.Join(vmPath, "qemu.pid"))

	args = append(args, "-k")
	args = append(args, "en-us")

	if vm.CPU != "" {
		args = append(args, "-cpu")
		args = append(args, vm.CPU)
	}

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

	// net
	var bus, addr int
	addBus := func() {
		addr = 1 // start at 1 because 0 is reserved
		bus++
		args = append(args, fmt.Sprintf("-device"))
		args = append(args, fmt.Sprintf("pci-bridge,id=pci.%v,chassis_nr=%v", bus, bus))
	}

	addBus()
	for _, net := range vm.Networks {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", net.Tap, net.Tap))
		args = append(args, "-device")
		args = append(args, fmt.Sprintf("driver=%v,netdev=%v,mac=%v,bus=pci.%v,addr=0x%x", net.Driver, net.Tap, net.MAC, bus, addr))
		addr++
		if addr == DEV_PER_BUS {
			addBus()
		}
	}

	// virtio-serial
	virtio_slot := -1 // start at -1 since we immediately increment
	for i := 0; i < vm.VirtioPorts; i++ {
		// qemu port number
		nr := i%DEV_PER_VIRTIO + 1

		// If port is 1, we're out of slots on the current virtio-serial-pci
		// device or we're on the first iteration => make a new device
		if nr == 1 {
			virtio_slot++
			args = append(args, "-device")
			args = append(args, fmt.Sprintf("virtio-serial-pci,id=virtio-serial%v,bus=pci.%v,addr=0x%x", virtio_slot, bus, addr))

			addr++
			if addr == DEV_PER_BUS { // check to see if we've run out of addr slots on this bus
				addBus()
			}
		}

		args = append(args, "-chardev")
		args = append(args, fmt.Sprintf("socket,id=charvserial%v,path=%vvirtio-serial%v,server,nowait", i, vmPath, i))

		args = append(args, "-device")
		args = append(args, fmt.Sprintf("virtserialport,nr=%v,bus=virtio-serial%v.0,chardev=charvserial%v,id=charvserial%v,name=virtio-serial%v", nr, virtio_slot, i, i, i))
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

	log.Info("args for vm %v is: %#v", id, args)
	return args
}

// log any asynchronous messages, such as vnc connects, to log.Info
func (vm *KvmVM) asyncLogger() {
	for {
		v := vm.q.Message()
		if v == nil {
			return
		}
		log.Info("VM %v received asynchronous message: %v", vm.ID, v)
	}
}

func (vm *KvmVM) hotplugRemove(id int) error {
	hid := fmt.Sprintf("hotplug%v", id)
	log.Debugln("hotplug id:", hid)
	if _, ok := vm.hotplug[id]; !ok {
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
	delete(vm.hotplug, id)
	return nil
}

func qemuOverrideString() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintln(&o, "id\tmatch\treplacement")
	for i, v := range QemuOverrides {
		fmt.Fprintf(&o, "%v\t\"%v\"\t\"%v\"\n", i, v.match, v.repl)
	}
	w.Flush()

	args := vmConfig.qemuArgs(0, "") // ID doesn't matter -- just testing
	preArgs := unescapeString(args)
	postArgs := strings.Join(ParseQemuOverrides(args), " ")

	r := o.String()
	r += fmt.Sprintf("\nBefore overrides:\n%v\n", preArgs)
	r += fmt.Sprintf("\nAfter overrides:\n%v\n", postArgs)

	return r
}

func delVMQemuOverride(arg string) error {
	if arg == Wildcard {
		QemuOverrides = make(map[int]*qemuOverride)
		return nil
	}

	id, err := strconv.Atoi(arg)
	if err != nil {
		return fmt.Errorf("invalid id %v", arg)
	}

	delete(QemuOverrides, id)
	return nil
}

func addVMQemuOverride(match, repl string) error {
	id := <-qemuOverrideIdChan

	QemuOverrides[id] = &qemuOverride{
		match: match,
		repl:  repl,
	}

	return nil
}

func ParseQemuOverrides(input []string) []string {
	ret := unescapeString(input)
	for _, v := range QemuOverrides {
		ret = strings.Replace(ret, v.match, v.repl, -1)
	}
	return fieldsQuoteEscape("\"", ret)
}
