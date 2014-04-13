// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	info       *vmInfo // current vm info, interfaced be the cli
	savedInfo  map[string]*vmInfo
	launchRate time.Duration // launch/kill rate for vms

	// each vm struct acknowledges that it launched. this way, we won't
	// return from a vm_launch command until all have actually launched.
	launchAck chan int
	killAck   chan int
	vmIdChan  chan int
	vmLock    sync.Mutex
)

const (
	VM_BUILDING = iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

const (
	VM_MEMORY_DEFAULT = "2048"
	VM_NOT_FOUND      = -2
)

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
	bridges      []string // list of bridges, if specified. Unspecified bridges will contain ""
	taps         []string // list of taps associated with this vm
	Networks     []int    // ordered list of networks (matches 1-1 with Taps)
	macs         []string // ordered list of macs (matches 1-1 with Taps, Networks)
	netDrivers   []string // optional non-e1000 driver
	Snapshot     bool
	Hotplug      map[int]string
	PID          int
}

type jsonInfo struct {
	Id       int
	Host     string
	Name     string
	Memory   string
	Vcpus    string
	Disk     string
	Snapshot bool
	Initrd   string
	Kernel   string
	Cdrom    string
	Append   string
	State    string
	Bridges  []string
	Taps     []string
	Macs     []string
	IP       []string
	IP6      []string
	Networks []int
}

func init() {
	launchRate = time.Millisecond * 1000
	launchAck = make(chan int)
	killAck = make(chan int)
	vmIdChan = make(chan int)
	info = &vmInfo{}
	savedInfo = make(map[string]*vmInfo)
	go func() {
		count := 0
		for {
			vmIdChan <- count
			count++
		}
	}()

	// default parameters at startup
	info.Memory = VM_MEMORY_DEFAULT
	info.Vcpus = "1"
	info.DiskPath = ""
	info.KernelPath = ""
	info.InitrdPath = ""
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
		return vms.vms[i].DiskPath < vms.vms[j].DiskPath
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
	default:
		log.Fatal("invalid sort parameter %v", vms.by)
		return false
	}
}

func cliVMSave(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Error: "Usage: vm_save <save name> <vm id> [<vm id> ...]",
		}
	}

	path := *f_base + "saved_vms"
	err := os.MkdirAll(path, 0775)
	if err != nil {
		log.Fatalln(err)
	}

	file, err := os.Create(fmt.Sprintf("%v/%v", path, c.Args[0]))
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	var toSave []string
	if len(c.Args) == 1 {
		// get all vms
		for k, _ := range vms.vms {
			toSave = append(toSave, fmt.Sprintf("%v", k))
		}
	} else {
		toSave = c.Args[1:]
	}
	for _, vmStr := range toSave { // iterate over the vm id's specified
		vm := vms.getVM(vmStr)
		if vm == nil {
			return cliResponse{
				Error: fmt.Sprintf("no such vm %v", vmStr),
			}
		}

		// build up the command list to re-launch this vm
		cmds := []string{}
		cmds = append(cmds, "vm_memory "+vm.Memory)
		cmds = append(cmds, "vm_vcpus "+vm.Vcpus)

		if vm.DiskPath != "" {
			cmds = append(cmds, "vm_disk "+vm.DiskPath)
		} else {
			cmds = append(cmds, "clear vm_disk")
		}

		if vm.CdromPath != "" {
			cmds = append(cmds, "vm_cdrom "+vm.CdromPath)
		} else {
			cmds = append(cmds, "clear vm_cdrom")
		}

		if vm.KernelPath != "" {
			cmds = append(cmds, "vm_kernel "+vm.KernelPath)
		} else {
			cmds = append(cmds, "clear vm_kernel")
		}

		if vm.InitrdPath != "" {
			cmds = append(cmds, "vm_initrd "+vm.InitrdPath)
		} else {
			cmds = append(cmds, "clear vm_initrd")
		}

		if vm.Append != "" {
			cmds = append(cmds, "vm_append "+vm.Append)
		} else {
			cmds = append(cmds, "clear vm_append")
		}

		if len(vm.QemuAppend) != 0 {
			cmds = append(cmds, "vm_qemu_append "+strings.Join(vm.QemuAppend, " "))
		} else {
			cmds = append(cmds, "clear vm_qemu_append")
		}

		cmds = append(cmds, fmt.Sprintf("vm_snapshot %v", vm.Snapshot))
		if len(vm.Networks) != 0 {
			netString := "vm_net "
			for i, vlan := range vm.Networks {
				netString += fmt.Sprintf("%v,%v,%v,%v ", vm.bridges[i], vlan, vm.macs[i], vm.netDrivers[i])
			}
			cmds = append(cmds, strings.TrimSpace(netString))
		} else {
			cmds = append(cmds, "clear vm_net")
		}

		if vm.Name != "" {
			cmds = append(cmds, "vm_launch "+vm.Name)
		} else {
			cmds = append(cmds, "vm_launch 1")
		}

		// write commands to file
		for _, cmd := range cmds {
			_, err = file.WriteString(cmd + "\n")
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
		}
	}
	return cliResponse{}
}

// vm_config
// return a pretty printed list of the current configuration
func cliVMConfig(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		config := configToString()

		return cliResponse{
			Response: config,
		}
	case 1: // must be 'show'
		if c.Args[0] != "show" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		var r string
		for k, _ := range savedInfo {
			r += k + "\n"
		}
		return cliResponse{
			Response: r,
		}
	case 2: // must be 'save' 'restore'
		switch strings.ToLower(c.Args[0]) {
		case "save":
			savedInfo[c.Args[1]] = info.Copy()
		case "restore":
			info = savedInfo[c.Args[1]].Copy()
		default:
			return cliResponse{
				Error: "malformed command",
			}
		}
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}
	return cliResponse{}
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
	fmt.Fprintf(w, "Networks:\t%v\n", networkString())
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
			log.Errorln(err)
		}
	}
}

func networkString() string {
	s := "["
	for i, vlan := range info.Networks {
		if info.bridges[i] != "" {
			s += info.bridges[i] + ","
		}
		s += strconv.Itoa(vlan)
		if info.macs[i] != "" {
			s += "," + info.macs[i]
		}
		if i+1 < len(info.Networks) {
			s += " "
		}
	}
	s += "]"
	return s
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

// start vms that are paused or building, or restart vms in the quit state
func (l *vmList) start(c cliCommand) cliResponse {
	errors := ""
	if len(c.Args) == 0 { // start all paused vms
		for _, i := range l.vms {
			// only bulk start paused/building VMs
			if i.State == VM_PAUSED || i.State == VM_BUILDING {
				err := i.start()
				if err != nil {
					errors += fmt.Sprintln(err)
				}
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

		if vm, ok := l.vms[id]; ok {
			err := vm.start()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		} else {
			return cliResponse{
				Error: fmt.Sprintf("VM %v not found", c.Args[0]),
			}
		}
	}
	return cliResponse{
		Error: errors,
	}
}

func (vm *vmInfo) start() error {
	if vm.State != VM_PAUSED && vm.State != VM_BUILDING && vm.State != VM_QUIT {
		return nil
	}
	if vm.State == VM_QUIT {
		log.Info("restarting VM: %v", vm.Id)
		go vm.launchOne()
		<-launchAck
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

		if vm, ok := l.vms[id]; ok {
			err := vm.stop()
			if err != nil {
				errors += fmt.Sprintln(err)
			}
		} else {
			return cliResponse{
				Error: fmt.Sprintf("VM %v not found", c.Args[0]),
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
// return VM_NOT_FOUND (-2), as -1 is reserved as the wildcard.
func (l *vmList) findByName(name string) int {
	for i, v := range l.vms {
		if v.Name == name {
			return i
		}
	}
	return VM_NOT_FOUND
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

	if id == VM_NOT_FOUND {
		return cliResponse{
			Error: fmt.Sprintf("VM %v not found", c.Args[0]),
		}
	} else if id == -1 {
		killCount := 0
		timedOut := 0
		for _, i := range l.vms {
			s := i.getState()
			if s != VM_QUIT && s != VM_ERROR {
				i.Kill <- true
				killCount++
			}
		}
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
			return cliResponse{
				Error: fmt.Sprintf("%v killed VMs failed to acknowledge kill", timedOut),
			}
		}
	} else {
		if vm, ok := l.vms[id]; ok {
			if vm.State != VM_QUIT && vm.State != VM_ERROR {
				vm.Kill <- true
				log.Info("VM %v killed", <-killAck)
			}
		} else {
			return cliResponse{
				Error: fmt.Sprintf("invalid VM id: %v", id),
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
	for i := 0; i < numVms; i++ {
		vm := info.Copy() // returns reference to deep-copy of info
		vm.Id = <-vmIdChan
		vm.Name = name
		vm.Kill = make(chan bool)
		vm.Hotplug = make(map[int]string)
		vm.State = VM_BUILDING
		vmLock.Lock()
		l.vms[vm.Id] = vm
		vmLock.Unlock()
		go vm.launchOne()
	}
	// get acknowledgements from each vm
	for i := 0; i < numVms; i++ {
		<-launchAck
	}
	return cliResponse{}
}

func (info *vmInfo) Copy() *vmInfo {
	// makes deep copy of info and returns reference to new vmInfo struct
	newInfo := new(vmInfo)
	newInfo.Id = info.Id
	newInfo.Name = info.Name
	newInfo.Memory = info.Memory
	newInfo.Vcpus = info.Vcpus
	newInfo.DiskPath = info.DiskPath
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
	// Hotplug isn't allocated until later in launch()
	return newInfo
}

func (l *vmList) info(c cliCommand) cliResponse {
	var v []*vmInfo

	var output string
	var search string
	var mask string
	switch len(c.Args) {
	case 0:
	case 1: // output, search, or mask
		if strings.Contains(c.Args[0], "output=") {
			output = c.Args[0]
		} else if strings.Contains(c.Args[0], "=") {
			search = c.Args[0]
		} else if strings.HasPrefix(c.Args[0], "[") {
			mask = strings.Trim(c.Args[0], "[]")
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
	case 2:
		// first arg must be output or search
		if strings.Contains(c.Args[0], "output=") {
			output = c.Args[0]
		} else if strings.Contains(c.Args[0], "=") {
			search = c.Args[0]
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}

		// second arg must be search or mask, and cannot be search if
		// already set
		if strings.Contains(c.Args[1], "=") {
			if search != "" {
				return cliResponse{
					Error: "malformed command",
				}
			}
			search = c.Args[1]
		} else if strings.HasPrefix(c.Args[1], "[") {
			mask = strings.Trim(c.Args[1], "[]")
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
	case 3: // must be output, search, mask
		if strings.Contains(c.Args[0], "output=") {
			output = c.Args[0]
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
		if strings.Contains(c.Args[1], "=") {
			search = c.Args[1]
		} else {
			return cliResponse{
				Error: "malformed command",
			}
		}
		if strings.HasPrefix(c.Args[2], "[") {
			mask = strings.Trim(c.Args[2], "[]")
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

	// vm_info takes an output mode, search term, and an output mask, we'll start with the optional output mode
	if output != "" {
		d := strings.Split(output, "=")
		if len(d) != 2 {
			return cliResponse{
				Error: "malformed output mode",
			}
		}

		output = d[1]
		switch output {
		case "quiet":
			log.Debugln("vm_info quiet mode")
		case "json":
			log.Debugln("vm_info json mode")
		default:
			return cliResponse{
				Error: "malformed output mode",
			}
		}
	}

	if search != "" {
		d := strings.Split(search, "=")
		if len(d) != 2 {
			return cliResponse{
				Error: "malformed search term",
			}
		}

		log.Debug("vm_info search term: %v", d[1])

		switch strings.ToLower(d[0]) {
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
				return cliResponse{
					Error: fmt.Sprintf("invalid ID: %v", d[1]),
				}
			}
			if vm, ok := l.vms[id]; ok {
				v = append(v, vm)
			}
		case "name":
			id := l.findByName(d[1])
			if id == VM_NOT_FOUND {
				return cliResponse{}
			}
			if vm, ok := l.vms[id]; ok {
				v = append(v, vm)
			}
		case "memory":
			for i, j := range l.vms {
				if j.Memory == d[1] {
					v = append(v, l.vms[i])
				}
			}
		case "vcpus":
			for i, j := range l.vms {
				if j.Vcpus == d[1] {
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
		case "append":
			for i, j := range l.vms {
				if j.Append == d[1] {
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
					ip := GetIPFromMac(m)
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
					ip := GetIPFromMac(m)
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
					Error: fmt.Sprintf("invalid vlan: %v", d[1]),
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
		for _, vm := range l.vms {
			v = append(v, vm)
		}
	}
	if len(v) == 0 {
		return cliResponse{}
	}

	// short circuit if output == json, as we won't set any output masks
	if output == "json" {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)

		var o []jsonInfo
		host, err := os.Hostname()
		if err != nil {
			log.Errorln(err)
			teardown()
		}
		for _, i := range v {
			var state string
			switch i.State {
			case VM_BUILDING:
				state = "building"
			case VM_RUNNING:
				state = "running"
			case VM_PAUSED:
				state = "paused"
			case VM_QUIT:
				state = "quit"
			case VM_ERROR:
				state = "error"
			default:
				state = "unknown"
			}
			var ips []string
			var ip6 []string
			for _, m := range i.macs {
				ip := GetIPFromMac(m)
				if ip != nil {
					ips = append(ips, ip.IP4)
					ip6 = append(ip6, ip.IP6)
				}
			}
			o = append(o, jsonInfo{
				Id:       i.Id,
				Host:     host,
				Name:     i.Name,
				Memory:   i.Memory,
				Vcpus:    i.Vcpus,
				Disk:     i.DiskPath,
				Snapshot: i.Snapshot,
				Initrd:   i.InitrdPath,
				Kernel:   i.KernelPath,
				Cdrom:    i.CdromPath,
				Append:   i.Append,
				State:    state,
				Bridges:  i.bridges,
				Taps:     i.taps,
				Macs:     i.macs,
				IP:       ips,
				IP6:      ip6,
				Networks: i.Networks,
			})
		}
		err = enc.Encode(&o)
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		return cliResponse{
			Response: buf.String(),
		}
	}

	// output mask
	var omask []string
	if mask != "" {
		d := strings.Split(mask, ",")
		for _, j := range d {
			switch strings.ToLower(j) {
			case "id":
				omask = append(omask, "id")
			case "host":
				omask = append(omask, "host")
			case "name":
				omask = append(omask, "name")
			case "state":
				omask = append(omask, "state")
			case "memory":
				omask = append(omask, "memory")
			case "vcpus":
				omask = append(omask, "vcpus")
			case "disk":
				omask = append(omask, "disk")
			case "initrd":
				omask = append(omask, "initrd")
			case "kernel":
				omask = append(omask, "kernel")
			case "cdrom":
				omask = append(omask, "cdrom")
			case "append":
				omask = append(omask, "append")
			case "bridge":
				omask = append(omask, "bridge")
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
		omask = []string{"id", "host", "name", "state", "memory", "vcpus", "disk", "initrd", "kernel", "cdrom", "append", "bridge", "tap", "mac", "ip", "ip6", "vlan"}
	}

	// did someone do something silly?
	if len(omask) == 0 {
		return cliResponse{}
	}

	// create a sorted list of keys, based on the first column of the output mask
	SortBy(omask[0], v)

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	if output == "" {
		for i, k := range omask {
			if i != 0 {
				fmt.Fprintf(w, "\t| ")
			}
			fmt.Fprintf(w, k)
		}
		fmt.Fprintf(w, "\n")
	}
	for _, j := range v {
		for i, k := range omask {
			if i != 0 {
				if output == "quiet" {
					fmt.Fprintf(w, "\t")
				} else {
					fmt.Fprintf(w, "\t| ")
				}
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
			case "vcpus":
				fmt.Fprintf(w, "%v", j.Vcpus)
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
			case "append":
				fmt.Fprintf(w, "%v", j.Append)
			case "bridge":
				fmt.Fprintf(w, "%v", j.bridges)
			case "tap":
				fmt.Fprintf(w, "%v", j.taps)
			case "mac":
				fmt.Fprintf(w, "%v", j.macs)
			case "ip":
				var ips []string
				for _, m := range j.macs {
					ip := GetIPFromMac(m)
					if ip != nil {
						ips = append(ips, ip.IP4)
					}
				}
				fmt.Fprintf(w, "%v", ips)
			case "ip6":
				var ips []string
				for _, m := range j.macs {
					ip := GetIPFromMac(m)
					if ip != nil {
						ips = append(ips, ip.IP6)
					}
				}
				fmt.Fprintf(w, "%v", ips)
			case "vlan":
				var vlans []string
				for _, v := range j.Networks {
					if v == -1 {
						vlans = append(vlans, "disconnected")
					} else {
						vlans = append(vlans, fmt.Sprintf("%v", v))
					}
				}
				fmt.Fprintf(w, "%v", vlans)
			}
		}
		fmt.Fprintf(w, "\n")
	}
	w.Flush()

	return cliResponse{
		Response: o.String(),
	}
}

func (vm *vmInfo) launchPreamble() bool {
	// check if the vm has a conflict with the disk or mac address of another vm
	// build state of currently running system
	macMap := map[string]bool{}
	selfMacMap := map[string]bool{}
	diskSnapshotted := map[string]bool{}
	diskPersistent := map[string]bool{}

	vmLock.Lock()
	defer vmLock.Unlock()

	// populate selfMacMap
	for _, mac := range vm.macs {
		if mac == "" { // don't worry about empty mac addresses
			continue
		}

		_, ok := selfMacMap[mac]
		if ok { // if this vm specified the same mac address for two interfaces
			log.Errorln("Cannot specify the same mac address for two interfaces")
			vm.state(VM_ERROR)
			launchAck <- vm.Id // signal that this vm is "done" launching
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
			if vm2.DiskPath != "" {
				if vm2.Snapshot {
					diskSnapshotted[vm2.DiskPath] = true
				} else {
					diskPersistent[vm2.DiskPath] = true
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
		} else { // if mac is specified, check for mac address conflict
			// we only need to check against macMap because selfMacMap is collision-free at this point
			_, ok := macMap[mac]
			if ok { // if another vm has this mac address already
				log.Error("mac address %v is already in use by another vm.", mac)
				vm.state(VM_ERROR)
				launchAck <- vm.Id
				return false
			}
		}
	}

	// check for disk conflict
	_, existsSnapshotted := diskSnapshotted[vm.DiskPath] // check if another vm is using this disk in snapshot mode
	_, existsPersistent := diskPersistent[vm.DiskPath]   // check if another vm is using this disk in persistent mode (snapshot=false)

	if existsPersistent || (vm.Snapshot == false && existsSnapshotted) { // if we have a disk conflict
		log.Error("disk path %v is already in use by another vm.", vm.DiskPath)
		vm.state(VM_ERROR)
		launchAck <- vm.Id
		return false
	}

	vm.instancePath = *f_base + strconv.Itoa(vm.Id) + "/"
	err := os.MkdirAll(vm.instancePath, os.FileMode(0700))
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	return true
}

func (vm *vmInfo) launchOne() {
	log.Info("launching vm: %v", vm.Id)

	s := vm.getState()

	// don't repeat the preamble if we're just in the quit state
	if s != VM_QUIT {
		if !vm.launchPreamble() {
			return
		}
	}

	vm.state(VM_BUILDING)

	// write the config for this vm
	config := configToString()
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
		b := getBridge(vm.bridges[i])
		tap, err := b.TapCreate(lan)
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

	vm.PID = cmd.Process.Pid
	log.Debug("vm %v has pid %v", vm.Id, vm.PID)

	vm.CheckAffinity()

	go func() {
		err := cmd.Wait()
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
	sendKillAck := false

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
			<-waitChan
			sendKillAck = true // wait to ack until we've cleaned up
		}
	}

	for i, l := range vm.Networks {
		b := getBridge(vm.bridges[i])
		b.TapDestroy(l, vm.taps[i])
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
	args = append(args, "host")

	args = append(args, "-net")
	args = append(args, "none")

	args = append(args, "-S")

	if vm.DiskPath != "" {
		args = append(args, "-drive")
		args = append(args, "file="+vm.DiskPath+",cache=none,media=disk")
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

	bus := 1
	addr := 1
	args = append(args, fmt.Sprintf("-device"))
	args = append(args, fmt.Sprintf("pci-bridge,id=pci.%v,chassis_nr=%v", bus, bus))
	for i, tap := range vm.taps {
		args = append(args, "-netdev")
		args = append(args, fmt.Sprintf("tap,id=%v,script=no,ifname=%v", tap, tap))
		args = append(args, "-device")
		b := getBridge(vm.bridges[i])
		b.iml.AddMac(vm.macs[i])
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

// clear all vm_ arguments
// which is currently:
//	vm_qemu
//	vm_memory
//	vm_vcpus
//	vm_disk
//	vm_cdrom
//	vm_kernel
//	vm_initrd
//	vm_qemu_append
//	vm_append
//	vm_net
//	vm_snapshot
func cliClearVMConfig() error {
	externalProcesses["qemu"] = "kvm"
	info.Memory = VM_MEMORY_DEFAULT
	info.Vcpus = "1"
	info.DiskPath = ""
	info.CdromPath = ""
	info.KernelPath = ""
	info.InitrdPath = ""
	info.QemuAppend = []string{}
	info.Append = ""
	info.Networks = []int{}
	info.macs = []string{}
	info.bridges = []string{}
	info.netDrivers = []string{}
	info.Snapshot = true
	return nil
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

// CLI vm_net
// Allow specifying the bridge, vlan, and mac for one or more interfaces to a VM
func cliVMNet(c cliCommand) cliResponse {
	// example: vm_net my_bridge,100,00:00:00:00:00:00 101,00:00:00:00:00:01
	r := cliResponse{}
	if len(c.Args) == 0 {
		return cliResponse{
			Response: fmt.Sprintf("%v\n", networkString()),
		}
	} else {
		info.bridges = []string{}
		info.Networks = []int{}
		info.macs = []string{}
		info.netDrivers = []string{}

		for _, lan := range c.Args {
			f := strings.Split(lan, ",")
			// this takes a bit of parsing, because the entry can be in a few forms:
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
				return cliResponse{
					Error: "malformed command",
				}
			}

			log.Debug("vm_net got b=%v, v=%v, m=%v, d=%v", b, v, m, d)

			// VLAN ID, with optional bridge
			val, err := strconv.Atoi(v) // the vlan id
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}

			currBridge := getBridge(b)
			err = currBridge.LanCreate(val)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}

			if b == "" {
				info.bridges = append(info.bridges, DEFAULT_BRIDGE)
			} else {
				info.bridges = append(info.bridges, b)
			}

			info.Networks = append(info.Networks, val)

			if d == "" {
				info.netDrivers = append(info.netDrivers, "e1000")
			} else {
				info.netDrivers = append(info.netDrivers, d)
			}

			// (optional) MAC ADDRESS
			if m != "" {
				if isMac(m) {
					info.macs = append(info.macs, strings.ToLower(m))
				} else {
					info.macs = append(info.macs, m)
					r = cliResponse{
						Error: "Not a valid mac address: " + m,
					}
				}
			} else {
				info.macs = append(info.macs, "")
			}
		}
	}
	return r
}

func cliVMFlush(c cliCommand) cliResponse {
	for i, vm := range vms.vms {
		if vm.State == VM_QUIT || vm.State == VM_ERROR {
			log.Infoln("deleting VM: ", i)
			delete(vms.vms, i)
		}
	}
	return cliResponse{}
}

func cliVMHotplug(c cliCommand) cliResponse {
	if len(c.Args) < 2 {
		return cliResponse{
			Error: "vm_hotplug takes at least two arguments",
		}
	}

	switch c.Args[0] {
	case "show":
		vm := vms.getVM(c.Args[1])
		if vm == nil {
			return cliResponse{
				Error: fmt.Sprintf("no such VM %v", c.Args[1]),
			}
		}
		if len(vm.Hotplug) == 0 {
			return cliResponse{}
		}
		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintln(w, "Hotplug ID\tFile")
		for k, v := range vm.Hotplug {
			fmt.Fprintf(w, "%v\t%v\n", k, v)
		}
		w.Flush()
		return cliResponse{
			Response: o.String(),
		}
	case "add":
		if len(c.Args) != 3 {
			return cliResponse{
				Error: "invalid arguments",
			}
		}
		vm := vms.getVM(c.Args[1])
		if vm == nil {
			return cliResponse{
				Error: fmt.Sprintf("no such VM %v", c.Args[1]),
			}
		}
		disk := c.Args[2]
		// generate an id by adding 1 to the highest in the list for the Hotplug devices, 0 if it's empty
		id := 0
		for k, _ := range vm.Hotplug {
			if k >= id {
				id = k + 1
			}
		}
		hid := fmt.Sprintf("hotplug%v", id)
		log.Debugln("hotplug generated id:", hid)
		resp, err := vm.q.DriveAdd(hid, disk)
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		log.Debugln("hotplug drive_add response:", resp)
		resp, err = vm.q.USBDeviceAdd(hid)
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		log.Debugln("hotplug usb device add response:", resp)
		vm.Hotplug[id] = disk
	case "remove":
		if len(c.Args) != 3 {
			return cliResponse{
				Error: "invalid arguments",
			}
		}
		vm := vms.getVM(c.Args[1])
		if vm == nil {
			return cliResponse{
				Error: fmt.Sprintf("no such VM %v", c.Args[1]),
			}
		}
		id, err := strconv.Atoi(c.Args[2])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		if id == -1 {
			// remove all hotplug devices
			for k, _ := range vm.Hotplug {
				r := vm.hotplugRemove(k)
				if r.Error != "" {
					return r
				}
			}
		} else {
			return vm.hotplugRemove(id)
		}
	default:
		return cliResponse{
			Error: fmt.Sprintf("invalid argument %v", c.Args[0]),
		}
	}
	return cliResponse{}
}

func (vm *vmInfo) hotplugRemove(id int) cliResponse {
	hid := fmt.Sprintf("hotplug%v", id)
	log.Debugln("hotplug id:", hid)
	if _, ok := vm.Hotplug[id]; !ok {
		return cliResponse{
			Error: "no such hotplug device id",
		}
	}
	resp, err := vm.q.USBDeviceDel(hid)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	log.Debugln("hotplug usb device del response:", resp)
	resp, err = vm.q.DriveDel(hid)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	log.Debugln("hotplug usb drive del response:", resp)
	delete(vm.Hotplug, id)
	return cliResponse{}
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

func cliVMNetMod(c cliCommand) cliResponse {
	if len(c.Args) != 3 {
		return cliResponse{
			Error: "vm_netmod takes three arguments",
		}
	}

	// first arg is the vm name or id
	vm := vms.getVM(c.Args[0])
	if vm == nil {
		return cliResponse{
			Error: fmt.Sprintf("no such VM %v", c.Args[0]),
		}
	}

	// second arg is the vm_net position, which must fit within vm.Networks and vm.taps
	pos, err := strconv.Atoi(c.Args[1])
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	if len(vm.taps) < pos {
		return cliResponse{
			Error: fmt.Sprintf("no such netowrk %v, VM only has %v networks", pos, len(vm.taps)),
		}
	}

	// last arg is a number 0 < x < 4096, or the word disconnect
	if strings.ToLower(c.Args[2]) == "disconnect" {
		// disconnect
		log.Debug("disconnect network connection: %v %v %v", vm.Id, pos, vm.Networks[pos])
		b := getBridge(vm.bridges[pos])
		err := b.TapRemove(vm.Networks[pos], vm.taps[pos])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		vm.Networks[pos] = -1
	} else if net, err := strconv.Atoi(c.Args[2]); err == nil {
		if net > 0 && net < 4096 {
			// new network
			log.Debug("moving network connection: %v %v %v -> %v", vm.Id, pos, vm.Networks[pos], net)
			b := getBridge(vm.bridges[pos])
			if vm.Networks[pos] != -1 {
				err := b.TapRemove(vm.Networks[pos], vm.taps[pos])
				if err != nil {
					return cliResponse{
						Error: err.Error(),
					}
				}
			}
			err = b.TapAdd(net, vm.taps[pos], false)
			if err != nil {
				return cliResponse{
					Error: err.Error(),
				}
			}
			vm.Networks[pos] = net
		} else {
			return cliResponse{
				Error: fmt.Sprintf("invalid vlan tag %v", net),
			}
		}
	} else {
		return cliResponse{
			Error: fmt.Sprintf("must be 'disconnect' or a valid vlan tag"),
		}
	}
	return cliResponse{}
}
