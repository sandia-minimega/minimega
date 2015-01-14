// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"qmp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

var (
	info               *vmInfo // current vm info, interfaced be the cli
	savedInfo          map[string]*vmInfo
	killAck            chan int
	vmIdChan           chan int
	qemuOverrideIdChan chan int
	vmLock             sync.Mutex
	QemuOverrides      map[int]*qemuOverride
)

const (
	VM_BUILDING = 1 << iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

const (
	VM_MEMORY_DEFAULT     = "2048"
	VM_NET_DRIVER_DEFAULT = "e1000"
	VM_NOT_FOUND          = -2
	QMP_CONNECT_RETRY     = 50
	QMP_CONNECT_DELAY     = 100
)

type qemuOverride struct {
	match string
	repl  string
}

// total list of vms running on this host
type vmList struct {
	vms map[int]*vmInfo
}

type vmInfo struct {
	Lock         sync.Mutex
	Id           int
	Name         string
	Memory       string // memory for the vm, in megabytes
	Vcpus        string // number of virtual cpus
	DiskPaths    []string
	CdromPath    string
	KernelPath   string
	InitrdPath   string
	Append       string
	QemuAppend   []string  // extra arguments for QEMU
	State        int       // one of the VM_ states listed above
	Kill         chan bool // kill channel to signal to shut a vm down
	instancePath string
	q            qmp.Conn // qmp connection for this vm
	bridges      []string // list of bridges, if specified. Unspecified bridges will contain ""
	taps         []string // list of taps associated with this vm
	Networks     []int    // ordered list of networks (matches 1-1 with Taps)
	macs         []string // ordered list of macs (matches 1-1 with Taps, Networks)
	netDrivers   []string // optional non-e1000 driver
	Snapshot     bool
	Hotplug      map[int]string
	PID          int
	UUID         string
}

// Valid names for output masks for vm info
var vmMasks = map[string]bool{
	"id":        true,
	"host":      true,
	"name":      true,
	"state":     true,
	"memory":    true,
	"vcpus":     true,
	"disk":      true,
	"initrd":    true,
	"kernel":    true,
	"cdrom":     true,
	"append":    true,
	"bridge":    true,
	"tap":       true,
	"mac":       true,
	"ip":        true,
	"ip6":       true,
	"vlan":      true,
	"uuid":      true,
	"cc_active": true,
}

// Valid search patterns for vm info
var vmSearchFn = map[string]func(*vmInfo, string) bool{
	"append": func(v *vmInfo, q string) bool { return v.Append == q },
	"cdrom":  func(v *vmInfo, q string) bool { return v.CdromPath == q },
	"initrd": func(v *vmInfo, q string) bool { return v.InitrdPath == q },
	"kernel": func(v *vmInfo, q string) bool { return v.KernelPath == q },
	"memory": func(v *vmInfo, q string) bool { return v.Memory == q },
	"uuid":   func(v *vmInfo, q string) bool { return v.UUID == q },
	"vcpus":  func(v *vmInfo, q string) bool { return v.Vcpus == q },
	"disk": func(v *vmInfo, q string) bool {
		for _, disk := range v.DiskPaths {
			if disk == q {
				return true
			}
		}
		return false
	},
	"mac": func(v *vmInfo, q string) bool {
		for _, mac := range v.macs {
			if mac == q {
				return true
			}
		}
		return false
	},
	"ip": func(v *vmInfo, q string) bool {
		for _, mac := range v.macs {
			ip := GetIPFromMac(mac)
			if ip != nil && ip.IP4 == q {
				return true
			}
		}
		return false
	},
	"ip6": func(v *vmInfo, q string) bool {
		for _, mac := range v.macs {
			ip := GetIPFromMac(mac)
			if ip != nil && ip.IP6 == q {
				return true
			}
		}
		return false
	},
}

// TODO: This has become a mess... there must be a better way. Perhaps we can
// add an Update, UpdateBool, ... method to the vmInfo struct and then have the
// logic in there to handle the different config types.
var vmConfigFns = map[string]struct {
	Update        func(*vmInfo, string) error
	UpdateBool    func(*vmInfo, bool) error
	UpdateCommand func(*minicli.Command) error
	Clear         func(*vmInfo)
	Print         func(*vmInfo) string
	PrintCLI      func(*vmInfo) string // If not specified, Print is used
}{
	"append": {
		Update: func(vm *vmInfo, v string) error {
			vm.Append += v + " "
			return nil
		},
		Clear: func(vm *vmInfo) { vm.Append = "" },
		Print: func(vm *vmInfo) string { return vm.Append },
	},
	"cdrom": {
		Update: func(vm *vmInfo, v string) error {
			vm.CdromPath = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.CdromPath = "" },
		Print: func(vm *vmInfo) string { return vm.CdromPath },
	},
	"disk": {
		Update: func(vm *vmInfo, v string) error {
			vm.DiskPaths = append(vm.DiskPaths, v)
			return nil
		},
		Clear: func(vm *vmInfo) { vm.DiskPaths = []string{} },
		Print: func(vm *vmInfo) string { return fmt.Sprintf("%v", vm.DiskPaths) },
		PrintCLI: func(vm *vmInfo) string {
			if len(vm.DiskPaths) == 0 {
				return ""
			}
			return "vm config disk " + strings.Join(vm.DiskPaths, " ")
		},
	},
	"initrd": {
		Update: func(vm *vmInfo, v string) error {
			vm.InitrdPath = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.InitrdPath = "" },
		Print: func(vm *vmInfo) string { return vm.InitrdPath },
	},
	"kernel": {
		Update: func(vm *vmInfo, v string) error {
			vm.KernelPath = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.KernelPath = "" },
		Print: func(vm *vmInfo) string { return vm.KernelPath },
	},
	"memory": {
		Update: func(vm *vmInfo, v string) error {
			vm.Memory = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.Memory = VM_MEMORY_DEFAULT },
		Print: func(vm *vmInfo) string { return vm.Memory },
	},
	"net": {
		Update: processVMNet,
		Clear: func(vm *vmInfo) {
			vm.Networks = []int{}
			vm.bridges = []string{}
			vm.macs = []string{}
			vm.netDrivers = []string{}
		},
		Print: func(vm *vmInfo) string {
			return vm.networkString()
		},
		PrintCLI: func(vm *vmInfo) string {
			if len(vm.Networks) == 0 {
				return ""
			}

			nics := []string{}
			for i, vlan := range vm.Networks {
				nic := fmt.Sprintf("%v,%v,%v,%v", vm.bridges[i], vlan, vm.macs[i], vm.netDrivers[i])
				nics = append(nics, nic)
			}
			return "vm config net " + strings.Join(nics, " ")
		},
	},
	"qemu": { // TODO
		Update: func(vm *vmInfo, v string) error {
			customExternalProcesses["qemu"] = v
			return nil
		},
		Clear: func(vm *vmInfo) { delete(customExternalProcesses, "qemu") },
		Print: func(vm *vmInfo) string { return process("qemu") },
	},
	"qemu-append": { // TODO
		Update: func(vm *vmInfo, v string) error {
			vm.QemuAppend = append(vm.QemuAppend, fieldsQuoteEscape(`"`, v)...)
			return nil
		},
		Clear: func(vm *vmInfo) { vm.QemuAppend = []string{} },
		Print: func(vm *vmInfo) string { return fmt.Sprintf("%v", vm.QemuAppend) },
		PrintCLI: func(vm *vmInfo) string {
			if len(vm.QemuAppend) == 0 {
				return ""
			}
			return "vm config qemu-append " + strings.Join(vm.QemuAppend, " ")
		},
	},
	"qemu-override": {
		UpdateCommand: func(c *minicli.Command) error {
			if c.StringArgs["match"] != "" {
				return addVMQemuOverride(c.StringArgs["match"], c.StringArgs["replacement"])
			} else if c.StringArgs["id"] != "" {
				return delVMQemuOverride(c.StringArgs["id"])
			}

			panic("someone goofed the qemu-override patterns")
		},
		Clear: func(vm *vmInfo) { QemuOverrides = make(map[int]*qemuOverride) },
		Print: func(vm *vmInfo) string {
			return qemuOverrideString()
		},
		PrintCLI: func(vm *vmInfo) string {
			overrides := []string{}
			for _, q := range QemuOverrides {
				override := fmt.Sprintf("vm config qemu-override add %s %s", q.match, q.repl)
				overrides = append(overrides, override)
			}
			return strings.Join(overrides, "\n")
		},
	},
	"snapshot": {
		UpdateBool: func(vm *vmInfo, v bool) error {
			vm.Snapshot = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.Snapshot = true },
		Print: func(vm *vmInfo) string { return fmt.Sprintf("%v", vm.Snapshot) },
	},
	"uuid": {
		Update: func(vm *vmInfo, v string) error {
			vm.UUID = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.UUID = "" },
		Print: func(vm *vmInfo) string { return vm.UUID },
	},
	"vcpus": {
		Update: func(vm *vmInfo, v string) error {
			vm.Vcpus = v
			return nil
		},
		Clear: func(vm *vmInfo) { vm.Vcpus = "1" },
		Print: func(vm *vmInfo) string { return vm.Vcpus },
	},
}

func init() {
	QemuOverrides = make(map[int]*qemuOverride)
	killAck = make(chan int)
	vmIdChan = make(chan int)
	qemuOverrideIdChan = make(chan int)
	info = &vmInfo{}
	savedInfo = make(map[string]*vmInfo)
	go func() {
		count := 0
		for {
			vmIdChan <- count
			count++
		}
	}()
	go func() {
		count := 0
		for {
			qemuOverrideIdChan <- count
			count++
		}
	}()

	// default parameters at startup
	info.Memory = VM_MEMORY_DEFAULT
	info.Vcpus = "1"
	info.State = VM_BUILDING
	info.Snapshot = true
}

// satisfy the sort interface for vmInfo
func SortBy(by string, vms []*vmInfo) {
	v := &vmSorter{
		vms: vms,
		by:  by,
	}
	sort.Sort(v)
}

type vmSorter struct {
	vms []*vmInfo
	by  string
}

func (vms *vmSorter) Len() int {
	return len(vms.vms)
}

func (vms *vmSorter) Swap(i, j int) {
	vms.vms[i], vms.vms[j] = vms.vms[j], vms.vms[i]
}

func (vms *vmSorter) Less(i, j int) bool {
	switch vms.by {
	case "id":
		return vms.vms[i].Id < vms.vms[j].Id
	case "host":
		return true
	case "name":
		return vms.vms[i].Name < vms.vms[j].Name
	case "state":
		return vms.vms[i].State < vms.vms[j].State
	case "memory":
		return vms.vms[i].Memory < vms.vms[j].Memory
	case "vcpus":
		return vms.vms[i].Vcpus < vms.vms[j].Vcpus
	case "disk":
		return len(vms.vms[i].DiskPaths) < len(vms.vms[j].DiskPaths)
	case "initrd":
		return vms.vms[i].InitrdPath < vms.vms[j].InitrdPath
	case "kernel":
		return vms.vms[i].KernelPath < vms.vms[j].KernelPath
	case "cdrom":
		return vms.vms[i].CdromPath < vms.vms[j].CdromPath
	case "append":
		return vms.vms[i].Append < vms.vms[j].Append
	case "bridge", "tap", "mac", "ip", "ip6", "vlan":
		return true
	case "uuid":
		return vms.vms[i].UUID < vms.vms[j].UUID
	case "cc_active":
		return true
	default:
		log.Error("invalid sort parameter %v", vms.by)
		return false
	}
}

func (l *vmList) qmp(vm, qmp string) (string, error) {
	id, err := strconv.Atoi(vm)
	if err != nil {
		id = l.findByName(vm)
	}

	if vm, ok := l.vms[id]; ok {
		return vm.QMPRaw(qmp)
	}

	return "", fmt.Errorf("vm %v not found", vm)
}

func (vm *vmInfo) QMPRaw(input string) (string, error) {
	return vm.q.Raw(input)
}

func (l *vmList) save(file *os.File, vms []string) error {
	var allVms bool
	for _, vm := range vms {
		if vm == "*" {
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
		vm := l.getVM(vmStr)
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
			cmds = append(cmds, "vm launch name "+vm.Name)
		} else {
			cmds = append(cmds, "vm launch count 1")
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

// apply applies the provided function to the vm in vmList whose name or ID
// matches the provided vm parameter.
func (l *vmList) apply(vm string, fn func(*vmInfo) error) error {
	id, err := strconv.Atoi(vm)
	if err != nil {
		id = l.findByName(vm)
	}

	if vm, ok := l.vms[id]; ok {
		return fn(vm)
	}

	return fmt.Errorf("vm %v not found", vm)
}

// start vms that are paused or building, or restart vms in the quit state
func (l *vmList) start(vm string, quit bool) []error {
	if vm != "*" {
		err := l.apply(vm, func(vm *vmInfo) error { return vm.start() })
		return []error{err}
	}

	stateMask := VM_PAUSED + VM_BUILDING
	if quit {
		stateMask += VM_QUIT
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

func (vm *vmInfo) start() error {
	if vm.State != VM_PAUSED && vm.State != VM_BUILDING && vm.State != VM_QUIT {
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
		return err
	} else {
		vm.state(VM_RUNNING)
	}
	return nil
}

// stop vms that are paused or building
func (l *vmList) stop(vm string) []error {
	if vm != "*" {
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

func (vm *vmInfo) stop() error {
	if vm.State != VM_RUNNING {
		return fmt.Errorf("VM %v not running", vm.Id)
	}
	log.Info("stopping VM: %v", vm.Id)
	err := vm.q.Stop()
	if err != nil {
		return err
	} else {
		vm.state(VM_PAUSED)
	}
	return nil
}

// findByName returns the id of a VM based on its name. If the VM doesn't exist
// return VM_NOT_FOUND (-2), as -1 is reserved as the wildcard.
func (l *vmList) findByName(name string) int {
	for i, v := range l.vms {
		if v.Name == name {
			return i
		}
	}
	return VM_NOT_FOUND
}

// findByName returns the id of a VM based on its name. If the VM doesn't exist
// return VM_NOT_FOUND (-2), as -1 is reserved as the wildcard.
func (l *vmList) findRunningByName(name string) int {
	for i, v := range l.vms {
		if v.Name == name && v.State == VM_RUNNING {
			return i
		}
	}
	return VM_NOT_FOUND
}

// kill one or all vms (-1 for all)
func (l *vmList) kill(vm string) []error {
	if vm != "*" {
		err := l.apply(vm, func(vm *vmInfo) error {
			if vm.State != VM_RUNNING {
				return fmt.Errorf("vm %v is not running", vm.Name)
			}

			vm.Kill <- true
			log.Info("VM %v killed", <-killAck)
			return nil
		})

		return []error{err}
	}

	killCount := 0
	timedOut := 0

	for _, i := range l.vms {
		s := i.getState()
		if s != VM_QUIT && s != VM_ERROR {
			i.Kill <- true
			killCount++
		}
	}

	// TODO: This isn't quite right... we will wait for killCount *
	// COMMAND_TIMEOUT seconds rather than COMMAND_TIMEOUT seconds.
	for i := 0; i < killCount; i++ {
		select {
		case id := <-killAck:
			log.Info("VM %v killed", id)
		case <-time.After(COMMAND_TIMEOUT * time.Second):
			log.Error("vm kill timeout")
			timedOut++
		}
	}

	if timedOut != 0 {
		return []error{fmt.Errorf("%v killed VMs failed to acknowledge kill", timedOut)}
	}

	return nil
}

func (l *vmList) flush() {
	for i, vm := range vms.vms {
		if vm.State == VM_QUIT || vm.State == VM_ERROR {
			log.Infoln("deleting VM: ", i)
			delete(vms.vms, i)
		}
	}
}

// launch one or more vms. this will copy the info struct, one per vm
// and launch each one in a goroutine. it will not return until all
// vms have reported that they've launched.
func (l *vmList) launch(name string, ack chan int) error {
	// Make sure that there isn't another VM with the same name
	if name != "" {
		for _, vm := range l.vms {
			if vm.Name == name {
				return fmt.Errorf("vm_launch duplicate VM name: %s", name)
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
	vm.State = VM_BUILDING
	vmLock.Lock()
	l.vms[vm.Id] = vm
	vmLock.Unlock()
	go vm.launchOne(ack)

	return nil
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
	// Hotplug isn't allocated until later in launch()
	return newInfo
}

func (l *vmList) info(omask []string, search string) ([][]string, error) {
	var v []*vmInfo

	// did someone do something silly?
	if len(omask) == 0 {
		return make([][]string, 0), nil
	}

	if search != "" {
		d := strings.Split(search, "=")
		if len(d) != 2 {
			return nil, errors.New("malformed search term")
		}

		log.Debug("vm_info search term: %v", d[1])

		key := strings.ToLower(d[0])

		switch key {
		case "host":
			host, err := os.Hostname()
			if err != nil {
				log.Errorln(err)
				teardown()
			}
			if strings.ToLower(d[1]) == strings.ToLower(host) {
				for _, vm := range l.vms {
					v = append(v, vm)
				}
			}
		case "id":
			id, err := strconv.Atoi(d[1])
			if err != nil {
				return nil, fmt.Errorf("invalid ID: %v", d[1])
			}
			if vm, ok := l.vms[id]; ok {
				v = append(v, vm)
			}
		case "name":
			id := l.findByName(d[1])
			if id == VM_NOT_FOUND {
				return make([][]string, 0), nil
			}
			if vm, ok := l.vms[id]; ok {
				v = append(v, vm)
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
				return nil, fmt.Errorf("invalid state: %v", d[1])
			}
			for i, j := range l.vms {
				if j.State == s {
					v = append(v, l.vms[i])
				}
			}
		case "bridge":
		VM_INFO_BRIDGE_LOOP:
			for i, j := range l.vms {
				for _, k := range j.bridges {
					if k == d[1] || (d[1] == DEFAULT_BRIDGE && k == "") {
						v = append(v, l.vms[i])
						break VM_INFO_BRIDGE_LOOP
					}
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
		case "vlan":
			vlan, err := strconv.Atoi(d[1])
			if err != nil {
				return nil, fmt.Errorf("invalid vlan: %v", d[1])
			}
			for i, j := range l.vms {
				for _, k := range j.Networks {
					if k == vlan {
						v = append(v, l.vms[i])
						break
					}
				}
			}
		case "cc_active":
			activeClients := ccClients()
			for i, j := range l.vms {
				if activeClients[j.UUID] && d[1] == "true" {
					v = append(v, l.vms[i])
				} else if !activeClients[j.UUID] && d[1] == "false" {
					v = append(v, l.vms[i])
				}
			}
		default:
			if fn, ok := vmSearchFn[key]; ok {
				for i := range l.vms {
					if fn(l.vms[i], d[1]) {
						v = append(v, l.vms[i])
					}
				}
			} else {
				return nil, fmt.Errorf("invalid search term: %v", d[0])
			}
		}
	} else { // all vms
		for _, vm := range l.vms {
			v = append(v, vm)
		}
	}
	if len(v) == 0 {
		return make([][]string, 0), nil
	}

	// create a sorted list of keys, based on the first column of the output mask
	SortBy(omask[0], v)

	table := make([][]string, 0, len(v))
	for _, j := range v {
		row := make([]string, 0, len(omask))

		for _, k := range omask {
			switch k {
			case "host":
				host, err := os.Hostname()
				if err != nil {
					log.Errorln(err)
					teardown()
				}
				row = append(row, fmt.Sprintf("%v", host))
			case "id":
				row = append(row, fmt.Sprintf("%v", j.Id))
			case "name":
				row = append(row, fmt.Sprintf("%v", j.Name))
			case "memory":
				row = append(row, fmt.Sprintf("%v", j.Memory))
			case "vcpus":
				row = append(row, fmt.Sprintf("%v", j.Vcpus))
			case "state":
				switch j.State {
				case VM_BUILDING:
					row = append(row, "building")
				case VM_RUNNING:
					row = append(row, "running")
				case VM_PAUSED:
					row = append(row, "paused")
				case VM_QUIT:
					row = append(row, "quit")
				case VM_ERROR:
					row = append(row, "error")
				default:
					row = append(row, "unknown")
				}
			case "disk":
				field := fmt.Sprintf("%v", j.DiskPaths)
				if j.Snapshot && len(j.DiskPaths) != 0 {
					field += " [snapshot]"
				}
				row = append(row, field)
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
			}
		}

		table = append(table, row)
	}

	return table, nil
}

func (vm *vmInfo) launchPreamble(ack chan int) bool {
	// check if the vm has a conflict with the disk or mac address of another vm
	// build state of currently running system
	macMap := map[string]bool{}
	selfMacMap := map[string]bool{}
	diskSnapshotted := map[string]bool{}
	diskPersistent := map[string]bool{}

	vmLock.Lock()

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
			vmLock.Unlock()
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
		vmIsActive := s == VM_BUILDING || s == VM_RUNNING || s == VM_PAUSED

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
			vmLock.Unlock()
			ack <- vm.Id
			return false
		}
	}

	vmLock.Unlock()
	return true
}

func (vm *vmInfo) launchOne(ack chan int) {
	log.Info("launching vm: %v", vm.Id)

	s := vm.getState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT {
		if !vm.launchPreamble(ack) {
			return
		}
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

func (vm *vmInfo) getState() int {
	vm.Lock.Lock()
	defer vm.Lock.Unlock()
	return vm.State
}

// update the vm state, and write the state to file
func (vm *vmInfo) state(s int) {
	vm.Lock.Lock()
	defer vm.Lock.Unlock()
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

	args = append(args, "-chardev")
	args = append(args, "socket,id=charserial0,path="+vm.instancePath+"serial,server,nowait")

	args = append(args, "-pidfile")
	args = append(args, vm.instancePath+"qemu.pid")

	args = append(args, "-device")
	args = append(args, "isa-serial,chardev=charserial0,id=serial0")

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

func ParseQemuOverrides(input []string) []string {
	ret := unescapeString(input)
	for _, v := range QemuOverrides {
		ret = strings.Replace(ret, v.match, v.repl, -1)
	}
	return fieldsQuoteEscape("\"", ret)
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

	args := info.vmGetArgs(false)
	preArgs := unescapeString(args)
	postArgs := strings.Join(ParseQemuOverrides(args), " ")

	r := o.String()
	r += fmt.Sprintf("\nBefore overrides:\n%v\n", preArgs)
	r += fmt.Sprintf("\nAfter overrides:\n%v\n", postArgs)

	return r
}

func delVMQemuOverride(arg string) error {
	if arg == "*" {
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

// processVMNet processes the input specifying the bridge, vlan, and mac for
// one interface to a VM and updates the vm config accordingly. This takes a
// bit of parsing, because the entry can be in a few forms:
// 	vlan
//
//	vlan,mac
//	bridge,vlan
//	vlan,driver
//
//	bridge,vlan,mac
//	vlan,mac,driver
//	bridge,vlan,driver
//
//	bridge,vlan,mac,driver
// If there are 2 or 3 fields, just the last field for the presence of a mac
func processVMNet(vm *vmInfo, lan string) error {
	// example: my_bridge,100,00:00:00:00:00:00
	f := strings.Split(lan, ",")

	var b string
	var v string
	var m string
	var d string
	switch len(f) {
	case 1:
		v = f[0]
	case 2:
		if isMac(f[1]) {
			// vlan, mac
			v = f[0]
			m = f[1]
		} else if _, err := strconv.Atoi(f[0]); err == nil {
			// vlan, driver
			v = f[0]
			d = f[1]
		} else {
			// bridge, vlan
			b = f[0]
			v = f[1]
		}
	case 3:
		if isMac(f[2]) {
			// bridge, vlan, mac
			b = f[0]
			v = f[1]
			m = f[2]
		} else if isMac(f[1]) {
			// vlan, mac, driver
			v = f[0]
			m = f[1]
			d = f[2]
		} else {
			// bridge, vlan, driver
			b = f[0]
			v = f[1]
			d = f[2]
		}
	case 4:
		b = f[0]
		v = f[1]
		m = f[2]
		d = f[3]
	default:
		return errors.New("malformed netspec")
	}

	log.Debug("vm_net got b=%v, v=%v, m=%v, d=%v", b, v, m, d)

	// VLAN ID, with optional bridge
	vlan, err := strconv.Atoi(v) // the vlan id
	if err != nil {
		return errors.New("malformed netspec, vlan must be an integer")
	}

	if m != "" && !isMac(m) {
		return errors.New("malformed netspec, invalid mac address: " + m)
	}

	currBridge, err := getBridge(b)
	if err != nil {
		return err
	}

	err = currBridge.LanCreate(vlan)
	if err != nil {
		return err
	}

	vm.Networks = append(vm.Networks, vlan)

	if b == "" {
		b = DEFAULT_BRIDGE
	}
	if d == "" {
		d = VM_NET_DRIVER_DEFAULT
	}

	vm.bridges = append(vm.bridges, b)
	vm.netDrivers = append(vm.netDrivers, d)
	vm.macs = append(vm.macs, strings.ToLower(m))

	return nil
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

func (l *vmList) getVM(idOrName string) *vmInfo {
	id, err := strconv.Atoi(idOrName)
	if err != nil {
		id = l.findByName(idOrName)
	}

	if vm, ok := l.vms[id]; ok {
		return vm
	}
	return nil
}
