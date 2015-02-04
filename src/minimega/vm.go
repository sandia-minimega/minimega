// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"minicli"
	log "minilog"

	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
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

type VmState int

const (
	VM_BUILDING VmState = 1 << iota
	VM_RUNNING
	VM_PAUSED
	VM_QUIT
	VM_ERROR
)

const (
	VM_MEMORY_DEFAULT     = "2048"
	VM_NET_DRIVER_DEFAULT = "e1000"
	QMP_CONNECT_RETRY     = 50
	QMP_CONNECT_DELAY     = 100
)

type qemuOverride struct {
	match string
	repl  string
}

// Valid names for output masks for vm info, in preferred output order
var vmMasks = []string{
	"id", "host", "name", "state", "memory", "vcpus", "disk", "initrd",
	"kernel", "cdrom", "append", "bridge", "tap", "mac", "ip", "ip6", "vlan",
	"uuid", "cc_active",
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
	"qemu": {
		Update: func(vm *vmInfo, v string) error {
			customExternalProcesses["qemu"] = v
			return nil
		},
		Clear: func(vm *vmInfo) { delete(customExternalProcesses, "qemu") },
		Print: func(vm *vmInfo) string { return process("qemu") },
	},
	"qemu-append": {
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
	info = &vmInfo{}
	savedInfo = make(map[string]*vmInfo)

	vmIdChan = makeIDChan()
	qemuOverrideIdChan = makeIDChan()

	// default parameters at startup
	info.Memory = VM_MEMORY_DEFAULT
	info.Vcpus = "1"
	info.State = VM_BUILDING
	info.Snapshot = true
}

func vmNotFound(idOrName string) error {
	return fmt.Errorf("vm not found: %v", idOrName)
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

func vmGetAllSerialPorts() []string {
	vmLock.Lock()
	defer vmLock.Unlock()

	var ret []string
	for _, v := range vms.vms {
		ret = append(ret, v.instancePath+"serial")
	}
	return ret
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

func (s VmState) String() string {
	switch s {
	case VM_BUILDING:
		return "BUILDING"
	case VM_RUNNING:
		return "RUNNING"
	case VM_PAUSED:
		return "PAUSED"
	case VM_QUIT:
		return "QUIT"
	case VM_ERROR:
		return "ERROR"
	}
	return fmt.Sprintf("VmState(%d)", s)
}

func ParseVmState(s string) (VmState, error) {
	switch strings.ToLower(s) {
	case "building":
		return VM_BUILDING, nil
	case "running":
		return VM_RUNNING, nil
	case "paused":
		return VM_PAUSED, nil
	case "quit":
		return VM_QUIT, nil
	case "error":
		return VM_ERROR, nil
	}

	return VM_ERROR, fmt.Errorf("invalid state: %v", s)
}
