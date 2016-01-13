// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"ipmac"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
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
	killAck  chan int   // channel that all VMs ack on when killed
	vmIdChan chan int   // channel of new VM IDs
	vmLock   sync.Mutex // lock for synchronizing access to vms

	vmConfig VMConfig // current vm config, updated by CLI

	savedInfo = make(map[string]VMConfig) // saved configs, may be reloaded
)

type VMType int

const (
	_ VMType = iota
	KVM
	CONTAINER
)

type VM interface {
	Config() *BaseConfig

	GetID() int      // GetID returns the VM's per-host unique ID
	GetName() string // GetName returns the VM's per-host unique name
	GetState() VMState
	GetType() VMType
	GetInstancePath() string

	// Launch asynchronously launches the VM and acknoledges the launch via the
	// provided channel.
	Launch(chan int)

	Kill() error
	Start() error
	Stop() error
	Flush() error

	String() string
	Info(string) (string, error)

	Tag(tag string) string
	GetTags() map[string]string
	ClearTags()

	UpdateBW()
	UpdateCCActive()

	// NetworkConnect updates the VM's config to reflect that it has been
	// connected to the specified bridge and VLAN.
	NetworkConnect(int, string, int) error

	// NetworkDisconnect updates the VM's config to reflect that the specified
	// tap has been disconnected.
	NetworkDisconnect(int) error
}

// BaseConfig contains all fields common to all VM types.
type BaseConfig struct {
	Vcpus  string // number of virtual cpus
	Memory string // memory for the vm, in megabytes

	Networks []NetConfig // ordered list of networks

	Snapshot bool
	UUID     string
	ActiveCC bool // Whether CC is active, updated by calling UpdateCCActive
}

// VMConfig contains all the configs possible for a VM. When a VM of a
// particular kind is launched, only the pertinent configuration is copied so
// fields from other configs will have the zero value for the field type.
type VMConfig struct {
	BaseConfig
	KVMConfig
	ContainerConfig
}

// NetConfig contains all the network-related config for an interface. The IP
// addresses are automagically populated by snooping ARP traffic. The bandwidth
// stats are updated on-demand by calling the UpdateBW function of BaseConfig.
type NetConfig struct {
	VLAN   int
	Bridge string
	Tap    string
	MAC    string
	Driver string
	IP4    string
	IP6    string
	Stats  *TapStat // Most recent bandwidth measurements for Tap
}

// BaseVM provides the bare-bones for base VM functionality. It implements
// several functions from the VM interface that are relatively common. All
// newly created VM types will most likely embed this struct to reuse the base
// functionality.
type BaseVM struct {
	BaseConfig // embed

	sync.Mutex // embed, vm is lockble to synchronize changes to VM

	kill chan bool // channel to signal the VM to shut down

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
	"mac", "ip", "ip6", "bandwidth", "migrate", "disk", "snapshot", "initrd",
	"kernel", "cdrom", "append", "uuid", "cc_active", "tags",
}

func init() {
	killAck = make(chan int)

	vmIdChan = makeIDChan()

	// Reset everything to default
	for _, fns := range baseConfigFns {
		fns.Clear(&vmConfig.BaseConfig)
	}

	// for serializing VMs
	gob.Register(VMs{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

// NewVM creates a new VM, copying the currently set configs. After a VM is
// created, it can be Launched.
func NewVM(name string) *BaseVM {
	vm := new(BaseVM)

	vm.BaseConfig = *vmConfig.BaseConfig.Copy() // deep-copy configured fields
	vm.ID = <-vmIdChan
	if name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.ID)
	} else {
		vm.Name = name
	}

	// generate a UUID if we don't have one
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}

	vm.kill = make(chan bool)

	vm.instancePath = filepath.Join(*f_base, strconv.Itoa(vm.ID))

	vm.State = VM_BUILDING
	vm.Tags = make(map[string]string)

	return vm
}

func (s VMType) String() string {
	switch s {
	case KVM:
		return "kvm"
	case CONTAINER:
		return "container"
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
		BaseConfig:      *old.BaseConfig.Copy(),
		KVMConfig:       *old.KVMConfig.Copy(),
		ContainerConfig: *old.ContainerConfig.Copy(),
	}
}

func (vm VMConfig) String() string {
	return vm.BaseConfig.String() + vm.KVMConfig.String() + vm.ContainerConfig.String()
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
	vm.Lock()
	defer vm.Unlock()

	return vm.State
}

func (vm *BaseVM) GetType() VMType {
	return vm.Type
}

func (vm *BaseVM) GetInstancePath() string {
	return vm.instancePath
}

func (vm *BaseVM) Kill() error {
	vm.Lock()
	defer vm.Unlock()

	if vm.State&VM_KILLABLE == 0 {
		return fmt.Errorf("invalid VM state to kill: %d %v", vm.ID, vm.State)
	}

	// Close the channel to signal to all dependent goroutines that they should
	// stop. Anyone blocking on the channel will unblock immediately.
	// http://golang.org/ref/spec#Receive_operator
	close(vm.kill)

	// TODO: ACK if killed?
	return nil
}

func (vm *BaseVM) Flush() error {
	return os.RemoveAll(vm.instancePath)
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

func (vm *BaseVM) UpdateCCActive() {
	vm.ActiveCC = ccHasClient(vm.UUID)
}

func (vm *BaseVM) NetworkConnect(pos int, bridge string, vlan int) error {
	vm.Lock()
	defer vm.Unlock()

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	net := &vm.Networks[pos]

	log.Debug("moving network connection: %v %v %v -> %v %v", vm.ID, pos, net.VLAN, bridge, vlan)

	// Do this before disconnecting from the old bridge in case the new one was
	// mistyped or invalid.
	newBridge, err := getBridge(bridge)
	if err != nil {
		return err
	}

	// Disconnect from the old bridge, if we were connected
	if net.VLAN != DisconnectedVLAN {
		oldBridge, err := getBridge(net.Bridge)
		if err != nil {
			return err
		}

		err = oldBridge.TapRemove(net.Tap)
		if err != nil {
			return err
		}
	}

	// Connect to the new bridge
	err = newBridge.TapAdd(net.Tap, vlan, false)
	if err != nil {
		return err
	}

	// Record updates to the VM config
	net.VLAN = vlan
	net.Bridge = bridge

	return nil
}

func (vm *BaseVM) NetworkDisconnect(pos int) error {
	vm.Lock()
	defer vm.Unlock()

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	net := &vm.Networks[pos]

	// Don't try to diconnect an interface that is already disconnected...
	if net.VLAN == DisconnectedVLAN {
		return nil
	}

	log.Debug("disconnect network connection: %v %v %v", vm.ID, pos, net)

	b, err := getBridge(net.Bridge)
	if err != nil {
		return err
	}

	err = b.TapRemove(net.Tap)
	if err != nil {
		return err
	}

	net.Bridge = ""
	net.VLAN = DisconnectedVLAN

	return nil
}

func (vm *BaseVM) info(mask string) (string, error) {
	if fns, ok := baseConfigFns[mask]; ok {
		return fns.Print(&vm.BaseConfig), nil
	}

	var vals []string

	vm.Lock()
	defer vm.Unlock()

	switch mask {
	case "id":
		return fmt.Sprintf("%v", vm.ID), nil
	case "name":
		return fmt.Sprintf("%v", vm.Name), nil
	case "state":
		return vm.State.String(), nil
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
	case "cc_active":
		return fmt.Sprintf("%v", vm.ActiveCC), nil
	default:
		return "", errors.New("field not found")
	}

	return fmt.Sprintf("%v", vals), nil
}

// setState updates the vm state, and write the state to file. Assumes that the
// caller has locked the vm.
func (vm *BaseVM) setState(s VMState) {
	vm.State = s

	err := ioutil.WriteFile(filepath.Join(vm.instancePath, "state"), []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}

// setError updates the vm state and records the error in the VM's tags.
// Assumes that the caller has locked the vm.
func (vm *BaseVM) setError(err error) {
	vm.Tags["error"] = err.Error()
	vm.setState(VM_ERROR)
}

func (vm *BaseVM) macSnooper(net *NetConfig, updates chan ipmac.IP) {
	for update := range updates {
		// TODO: need to acquire VM lock?
		if update.IP4 != "" {
			net.IP4 = update.IP4
		} else if update.IP6 != "" && !strings.HasPrefix(update.IP6, "fe80") {
			net.IP6 = update.IP6
		}
	}
}

func (vm *BaseVM) writeTaps() error {
	taps := []string{}
	for _, net := range vm.Networks {
		taps = append(taps, net.Tap)
	}

	f := filepath.Join(vm.instancePath, "taps")
	if err := ioutil.WriteFile(f, []byte(strings.Join(taps, "\n")), 0666); err != nil {
		return fmt.Errorf("write instance taps file: %v", err)
	}

	return nil
}

func (vm *BaseVM) checkInterfaces() error {
	macs := map[string]bool{}

	for _, net := range vm.Networks {
		// Skip unassigned MACs
		if net.MAC == "" {
			continue
		}

		// Check if the VM already has this MAC for one of its interfaces
		if _, ok := macs[net.MAC]; ok {
			return fmt.Errorf("VM has same MAC for more than one interface -- %s", net.MAC)
		}

		macs[net.MAC] = true
	}

	// Ensure that no new VMs are added while we check our interfaces. If a new
	// VM has a conflict with us, it will be noted during thier
	// checkInterfaces. This also ensures that only one VM's checkInterfaces
	// can be running at a given time.
	vmLock.Lock()
	defer vmLock.Unlock()

	for _, vmOther := range vms {
		// Skip ourself
		if vm.ID == vmOther.GetID() {
			continue
		}

		for _, net := range vmOther.Config().Networks {
			// Warn if we see a conflict
			if _, ok := macs[net.MAC]; ok {
				log.Warn("VMs share MAC (%v) -- %v %v", net.MAC, vm.ID, vmOther.GetID())
			}

			macs[net.MAC] = true
		}
	}

	// Find any unassigned MACs and randomly generate a MAC for them
	for i := range vm.Networks {
		net := &vm.Networks[i]
		if net.MAC != "" {
			continue
		}

		for exists := true; exists; _, exists = macs[net.MAC] {
			net.MAC = randomMac()
		}

		macs[net.MAC] = true
	}

	return nil
}

func vmNotFound(idOrName string) error {
	return fmt.Errorf("vm not found: %v", idOrName)
}

func vmNotRunning(idOrName string) error {
	return fmt.Errorf("vm not running: %v", idOrName)
}

func vmNotPhotogenic(idOrName string) error {
	return fmt.Errorf("vm does not support screenshots: %v", idOrName)
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

	var b, v, m, d string
	switch len(f) {
	case 1:
		v = f[0]
	case 2:
		if isMac(f[1]) {
			// vlan, mac
			v, m = f[0], f[1]
		} else if _, err := strconv.Atoi(f[0]); err == nil {
			// vlan, driver
			v, d = f[0], f[1]
		} else {
			// bridge, vlan
			b, v = f[0], f[1]
		}
	case 3:
		if isMac(f[2]) {
			// bridge, vlan, mac
			b, v, m = f[0], f[1], f[2]
		} else if isMac(f[1]) {
			// vlan, mac, driver
			v, m, d = f[0], f[1], f[2]
		} else {
			// bridge, vlan, driver
			b, v, d = f[0], f[1], f[2]
		}
	case 4:
		b, v, m, d = f[0], f[1], f[2], f[3]
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

	// warn on valid but not allocated macs
	if m != "" && !allocatedMac(m) {
		log.Warn("unallocated mac address: %v", m)
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

// Get the VM info from all hosts in the mesh. Callers must specify whether
// they already hold the cmdLock or not. Returns a map where each key is a
// hostname and the value is the VMs for that host.
func globalVMs(hasLock bool) map[string]VMs {
	if !hasLock {
		cmdLock.Lock()
		defer cmdLock.Unlock()
	}

	res := map[string]VMs{}

	cmd := minicli.MustCompile("vm info")
	cmd.Record = false

	cmds := makeCommandHosts(meshageNode.BroadcastRecipients(), cmd)
	cmds = append(cmds, cmd) // add local node

	for resps := range processCommands(cmds...) {
		for _, resp := range resps {
			if resp.Error != "" {
				log.Errorln(resp.Error)
				continue
			}

			if vms, ok := resp.Data.(VMs); ok {
				res[resp.Host] = vms
			} else {
				log.Error("unknown data field in vm info")
			}
		}
	}

	return res
}
