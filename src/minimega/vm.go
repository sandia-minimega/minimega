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

const (
	VM_MEMORY_DEFAULT     = "2048"
	VM_NET_DRIVER_DEFAULT = "e1000"
	QMP_CONNECT_RETRY     = 50
	QMP_CONNECT_DELAY     = 100
)

var (
	killAck  chan int
	vmIdChan chan int
	vmLock   sync.Mutex

	vmConfig VMConfig // current kvm config, updated by CLI

	savedInfo = make(map[string]VMConfig)
)

type VMType int

const (
	_ VMType = iota
	KVM
	CONTAINER
)

type VM interface {
	Config() *BaseConfig

	GetID() int
	GetName() string
	GetState() VMState
	GetType() VMType

	Launch(string, chan int) error
	// TODO: Make kill have ack channel?
	Kill() error
	Start() error
	Stop() error

	String() string
	Info(string) (string, error)

	Tag(tag string) string
	GetTags() map[string]string
	ClearTags()

	UpdateBW()
}

type BaseConfig struct {
	Vcpus  string // number of virtual cpus
	Memory string // memory for the vm, in megabytes

	Networks []NetConfig // ordered list of networks

	Snapshot bool
	UUID     string
}

type VMConfig struct {
	BaseConfig
	KVMConfig
	ContainerConfig
}

type NetConfig struct {
	VLAN   int
	Bridge string
	Tap    string
	MAC    string
	Driver string
	IP4    string
	IP6    string
	Stats  *TapStat // Bandwidth stats, updated by calling UpdateBW
}

type BaseVM struct {
	BaseConfig // embed

	lock sync.Mutex

	ID    int
	Name  string
	State VMState
	Type  VMType

	instancePath string

	Tags map[string]string
}

// Valid names for output masks for vm info, in preferred output order
var vmMasks = []string{
	"id", "name", "state", "memory", "vcpus", "type", "vlan", "bridge", "tap",
	"mac", "ip", "ip6", "bandwidth", "tags",
}

func NewVM() *BaseVM {
	vm := new(BaseVM)

	vm.State = VM_BUILDING
	vm.Tags = make(map[string]string)

	return vm
}

func (s VMType) String() string {
	switch s {
	case KVM:
		return "kvm"
	default:
		return "???"
	}
}

func ParseVMType(s string) (VMType, error) {
	switch s {
	case "kvm":
		return KVM, nil
	case "container":
		return CONTAINER, nil
	default:
		return -1, errors.New("invalid VMType")
	}
}

func (old *VMConfig) Copy() *VMConfig {
	return &VMConfig{
		BaseConfig: *old.BaseConfig.Copy(),
		KVMConfig:  *old.KVMConfig.Copy(),
	}
}

func (vm VMConfig) String() string {
	return vm.BaseConfig.String() + vm.KVMConfig.String()
}

func (old *BaseConfig) Copy() *BaseConfig {
	res := new(BaseConfig)

	// Copy all fields
	*res = *old

	// Make deep copy of slices
	res.Networks = make([]NetConfig, len(old.Networks))
	copy(res.Networks, old.Networks)

	return res
}

func (vm *BaseConfig) String() string {
	// create output
	var o bytes.Buffer
	fmt.Fprintln(&o, "Current VM configuration:")
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Memory:\t%v\n", vm.Memory)
	fmt.Fprintf(w, "VCPUS:\t%v\n", vm.Vcpus)
	fmt.Fprintf(w, "Networks:\t%v\n", vm.NetworkString())
	fmt.Fprintf(w, "Snapshot:\t%v\n", vm.Snapshot)
	fmt.Fprintf(w, "UUID:\t%v\n", vm.UUID)
	w.Flush()
	fmt.Fprintln(&o)
	return o.String()
}

func (vm *BaseConfig) NetworkString() string {
	parts := []string{}
	for _, net := range vm.Networks {
		parts = append(parts, net.String())
	}

	return fmt.Sprintf("[%s]", strings.Join(parts, " "))
}

// TODO: Handle if there are spaces or commas in the tap/bridge names
func (net NetConfig) String() (s string) {
	parts := []string{}
	if net.Bridge != "" {
		parts = append(parts, net.Bridge)
	}

	parts = append(parts, strconv.Itoa(net.VLAN))

	if net.MAC != "" {
		parts = append(parts, net.MAC)
	}

	return strings.Join(parts, ",")
}

func (vm *BaseVM) GetID() int {
	return vm.ID
}

func (vm *BaseVM) GetName() string {
	return vm.Name
}

func (vm *BaseVM) GetState() VMState {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.State
}

func (vm *BaseVM) GetType() VMType {
	return vm.Type
}

func (vm *BaseVM) launch(name string, vmType VMType) error {
	vm.BaseConfig = *vmConfig.BaseConfig.Copy() // deep-copy configured fields

	vm.ID = <-vmIdChan
	if name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.ID)
	} else {
		vm.Name = name
	}

	vm.instancePath = *f_base + strconv.Itoa(vm.ID) + "/"

	vm.Type = vmType

	return nil
}

func (vm *BaseVM) Tag(tag string) string {
	return vm.Tags[tag]
}

func (vm *BaseVM) GetTags() map[string]string {
	return vm.Tags
}

func (vm *BaseVM) ClearTags() {
	vm.Tags = make(map[string]string)
}

func (vm *BaseVM) UpdateBW() {
	bandwidthLock.Lock()
	defer bandwidthLock.Unlock()

	for i := range vm.Networks {
		net := &vm.Networks[i]
		net.Stats = bandwidthStats[net.Tap]
	}
}

func (vm *BaseVM) info(mask string) (string, error) {
	if fns, ok := baseConfigFns[mask]; ok {
		return fns.Print(&vm.BaseConfig), nil
	}

	var vals []string

	switch mask {
	case "id":
		return fmt.Sprintf("%v", vm.ID), nil
	case "name":
		return fmt.Sprintf("%v", vm.Name), nil
	case "state":
		return vm.GetState().String(), nil
	case "type":
		return vm.GetType().String(), nil
	case "vlan":
		for _, net := range vm.Networks {
			if net.VLAN == DisconnectedVLAN {
				vals = append(vals, "disconnected")
			} else {
				vals = append(vals, fmt.Sprintf("%v", net.VLAN))
			}
		}
	case "bridge":
		for _, v := range vm.Networks {
			vals = append(vals, v.Bridge)
		}
	case "tap":
		for _, v := range vm.Networks {
			vals = append(vals, v.Tap)
		}
	case "mac":
		for _, v := range vm.Networks {
			vals = append(vals, v.MAC)
		}
	case "ip":
		for _, v := range vm.Networks {
			vals = append(vals, v.IP4)
		}
	case "ip6":
		for _, v := range vm.Networks {
			vals = append(vals, v.IP6)
		}
	case "bandwidth":
		for _, v := range vm.Networks {
			if v.Stats == nil {
				vals = append(vals, "N/A")
			} else {
				vals = append(vals, fmt.Sprintf("%v", v.Stats))
			}
		}
	case "tags":
		return fmt.Sprintf("%v", vm.Tags), nil
	default:
		return "", errors.New("field not found")
	}

	return fmt.Sprintf("%v", vals), nil
}

func init() {
	killAck = make(chan int)

	vmIdChan = makeIDChan()

	// Reset everything to default
	for _, fns := range baseConfigFns {
		fns.Clear(&vmConfig.BaseConfig)
	}
}

func vmNotFound(idOrName string) error {
	return fmt.Errorf("vm not found: %v", idOrName)
}

// satisfy the sort interface for vmInfo
func SortBy(by string, vms []*KvmVM) {
	v := &vmSorter{
		vms: vms,
		by:  by,
	}
	sort.Sort(v)
}

type vmSorter struct {
	vms []*KvmVM
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
		return vms.vms[i].ID < vms.vms[j].ID
	case "host":
		return true
	case "name":
		return vms.vms[i].Name < vms.vms[j].Name
	case "state":
		return vms.vms[i].GetState() < vms.vms[j].GetState()
	case "memory":
		return vms.vms[i].Memory < vms.vms[j].Memory
	case "vcpus":
		return vms.vms[i].Vcpus < vms.vms[j].Vcpus
	case "migrate":
		return vms.vms[i].MigratePath < vms.vms[j].MigratePath
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
	case "tags":
		return true
	default:
		log.Error("invalid sort parameter %v", vms.by)
		return false
	}
}

func vmGetFirstVirtioPort() []string {
	vmLock.Lock()
	defer vmLock.Unlock()

	mask := VM_BUILDING | VM_RUNNING | VM_PAUSED

	var ret []string
	for _, vm := range vms {
		// TODO: non-kvm VMs?
		if vm, ok := vm.(*KvmVM); ok && vm.GetState()&mask != 0 {
			if vm.VirtioPorts > 0 {
				ret = append(ret, vm.instancePath+"virtio-serial0")
			}
		}
	}
	return ret
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
func processVMNet(spec string) (res NetConfig, err error) {
	// example: my_bridge,100,00:00:00:00:00:00
	f := strings.Split(spec, ",")

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
		err = errors.New("malformed netspec")
		return
	}

	log.Debug("vm_net got b=%v, v=%v, m=%v, d=%v", b, v, m, d)

	// VLAN ID, with optional bridge
	vlan, err := strconv.Atoi(v) // the vlan id
	if err != nil {
		err = errors.New("malformed netspec, vlan must be an integer")
		return
	}

	if m != "" && !isMac(m) {
		err = errors.New("malformed netspec, invalid mac address: " + m)
		return
	}

	var currBridge *bridge
	currBridge, err = getBridge(b)
	if err != nil {
		return
	}

	err = currBridge.LanCreate(vlan)
	if err != nil {
		return
	}

	if b == "" {
		b = DEFAULT_BRIDGE
	}
	if d == "" {
		d = VM_NET_DRIVER_DEFAULT
	}

	res = NetConfig{
		VLAN:   vlan,
		Bridge: b,
		MAC:    strings.ToLower(m),
		Driver: d,
	}

	return
}

// Get the VM info from all hosts optionally applying column/row filters.
// Returns a map with keys for the hostnames and values as the tabular data
// from the host.
func globalVmInfo() map[string]VMs {
	cmdStr := "vm info"

	res := map[string]VMs{}

	cmd := minicli.MustCompile(cmdStr)
	cmd.Record = false

	for resps := range runCommandGlobally(cmd) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			switch data := resp.Data.(type) {
			case VMs:
				res[resp.Host] = data
			default:
				log.Error("unknown data field in vm info")
			}
		}
	}

	return res
}

// mustFindMask returns the index of the specified mask in vmMasks. If the
// specified mask is not found, log.Fatal is called.
func mustFindMask(mask string) int {
	for i, v := range vmMasks {
		if v == mask {
			return i
		}
	}

	log.Fatal("missing `%s` in vmMasks", mask)
	return -1
}
