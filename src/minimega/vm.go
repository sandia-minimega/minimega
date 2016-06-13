// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"ipmac"
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
	killAck chan int // channel that all VMs ack on when killed
	vmID    *Counter // channel of new VM IDs
)

type VMType int

const (
	_ VMType = iota
	KVM
	CONTAINER
)

type VM interface {
	GetID() int           // GetID returns the VM's per-host unique ID
	GetName() string      // GetName returns the VM's per-host unique name
	GetNamespace() string // GetNamespace returns the VM's namespace
	GetHost() string      // GetHost returns the hostname that the VM is running on
	GetState() VMState
	GetType() VMType
	GetInstancePath() string
	GetUUID() string

	// Life cycle functions
	Launch() error
	Kill() error
	Start() error
	Stop() error
	Flush() error

	String() string
	Info(string) (string, error)

	Tag(string) string          // Tag gets the value of the given tag
	SetTag(string, string)      // SetTag updates the given tag
	GetTags() map[string]string // GetTags returns a copy of the tags
	ClearTag(string)            // ClearTag deletes one or all tags

	// SaveConfig writes the commands to relaunch this VM with the same
	// config to the io.Writer.
	SaveConfig(io.Writer) error

	// Conflicts checks whether the VMs have conflicting configs. Called
	// when we create a VM but before adding it to the list of VMs.
	Conflicts(VM) error

	SetCCActive(bool)
	UpdateBW()

	// NetworkConnect updates the VM's config to reflect that it has been
	// connected to the specified bridge and VLAN.
	NetworkConnect(int, string, int) error

	// NetworkDisconnect updates the VM's config to reflect that the specified
	// tap has been disconnected.
	NetworkDisconnect(int) error
}

// BaseConfig contains all fields common to all VM types.
type BaseConfig struct {
	Namespace string // namespace this VM belongs to
	Host      string // hostname where this VM is running

	Vcpus  string // number of virtual cpus
	Memory string // memory for the vm, in megabytes

	Networks []NetConfig // ordered list of networks

	Snapshot bool
	UUID     string
	ActiveCC bool // set when CC is active

	Tags map[string]string
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

	RxRate, TxRate float64 // Most recent bandwidth measurements for Tap
}

// BaseVM provides the bare-bones for base VM functionality. It implements
// several functions from the VM interface that are relatively common. All
// newly created VM types will most likely embed this struct to reuse the base
// functionality.
type BaseVM struct {
	BaseConfig // embed

	lock sync.Mutex // synchronizes changes to this VM

	kill chan bool // channel to signal the vm to shut down

	ID    int
	Name  string
	State VMState
	Type  VMType

	instancePath string
}

// Valid names for output masks for `vm info`, in preferred output order
var vmInfo = []string{
	"id", "name", "state", "namespace", "memory", "vcpus", "type", "vlan",
	"bridge", "tap", "mac", "ip", "ip6", "bandwidth", "migrate", "disk",
	"snapshot", "initrd", "kernel", "cdrom", "append", "uuid", "cc_active",
	"tags",
}

// Valid names for output masks for `vm summary`, in preferred output order
var vmInfoLite = []string{
	"id", "name", "state", "namespace", "type", "vlan", "uuid", "cc_active",
}

func init() {
	killAck = make(chan int)

	vmID = NewCounter()

	// Reset everything to default
	for _, fns := range baseConfigFns {
		fns.Clear(&vmConfig.BaseConfig)
	}

	// for serializing VMs
	gob.Register(VMs{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

func NewVM(name string, vmType VMType) (VM, error) {
	switch vmType {
	case KVM:
		return NewKVM(name)
	case CONTAINER:
		return NewContainer(name)
	}

	return nil, errors.New("unknown VM type")
}

// NewBaseVM creates a new VM, copying the currently set configs. After a VM is
// created, it can be Launched.
func NewBaseVM(name string) *BaseVM {
	vm := new(BaseVM)

	vm.BaseConfig = vmConfig.BaseConfig.Copy() // deep-copy configured fields
	vm.ID = vmID.Next()
	if name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.ID)
	} else {
		vm.Name = name
	}

	vm.Namespace = GetNamespaceName()
	vm.Host = hostname

	// generate a UUID if we don't have one
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}

	// generate MAC addresses if any are unassigned. Don't bother checking
	// for collisions -- based on the birthday paradox, there's only a
	// 0.016% chance of collisions when running 10K VMs (one interface/VM).
	for i := range vm.Networks {
		if vm.Networks[i].MAC == "" {
			vm.Networks[i].MAC = randomMac()
		}
	}

	vm.kill = make(chan bool)

	vm.instancePath = filepath.Join(*f_base, strconv.Itoa(vm.ID))

	vm.State = VM_BUILDING

	// New VMs are returned pre-locked. This ensures that the first operation
	// called on a new VM is Launch.
	vm.lock.Lock()

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
		return 0, errors.New("invalid VMType")
	}
}

// findVMType tries to find a key that parses to a valid VMType. Useful for
// hunting through a command's BoolArgs.
func findVMType(args map[string]bool) (VMType, error) {
	for k := range args {
		if res, err := ParseVMType(k); err == nil {
			return res, nil
		}
	}

	return 0, errors.New("invalid VMType")
}

// TODO: Handle if there are spaces or commas in the tap/bridge names
func (net NetConfig) String() (s string) {
	parts := []string{}
	if net.Bridge != "" {
		parts = append(parts, net.Bridge)
	}

	parts = append(parts, printVLAN(net.VLAN))

	if net.MAC != "" {
		parts = append(parts, net.MAC)
	}

	return strings.Join(parts, ",")
}

func (old BaseConfig) Copy() BaseConfig {
	// Copy all fields
	res := old

	// Make deep copy of slices
	res.Networks = make([]NetConfig, len(old.Networks))
	copy(res.Networks, old.Networks)

	// Make deep copy of tags
	res.Tags = map[string]string{}
	for k, v := range old.Tags {
		res.Tags[k] = v
	}

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
	fmt.Fprintf(w, "Tags:\t%v\n", vm.TagsString())
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

func (vm *BaseConfig) TagsString() string {
	res, err := json.Marshal(vm.Tags)
	if err != nil {
		log.Error("unable to marshal vm.Tags: %v", err)
		return ""
	}

	return string(res)
}

func (vm *BaseVM) GetID() int {
	return vm.ID
}

func (vm *BaseVM) GetName() string {
	return vm.Name
}

func (vm *BaseVM) GetNamespace() string {
	return vm.Namespace
}

func (vm *BaseVM) GetHost() string {
	return vm.Host
}

func (vm *BaseVM) GetUUID() string {
	return vm.UUID
}

func (vm *BaseVM) GetState() VMState {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.State
}

func (vm *BaseVM) GetType() VMType {
	return vm.Type
}

func (vm *BaseVM) GetInstancePath() string {
	return vm.instancePath
}

func (vm *BaseVM) Kill() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if vm.State&VM_KILLABLE == 0 {
		return fmt.Errorf("invalid VM state to kill: %d %v", vm.ID, vm.State)
	}

	// Close the channel to signal to all dependent goroutines that they should
	// stop. Anyone blocking on the channel will unblock immediately.
	// http://golang.org/ref/spec#Receive_operator
	close(vm.kill)

	return nil
}

func (vm *BaseVM) Flush() error {
	ccNode.UnregisterVM(vm.UUID)

	return os.RemoveAll(vm.instancePath)
}

func (vm *BaseVM) Tag(t string) string {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.Tags[t]
}

func (vm *BaseVM) SetTag(t, v string) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.Tags[t] = v
}

func (vm *BaseVM) GetTags() map[string]string {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	res := map[string]string{}
	for k, v := range vm.Tags {
		res[k] = v
	}

	return res
}

func (vm *BaseVM) ClearTag(t string) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if t == Wildcard {
		vm.Tags = make(map[string]string)
	} else {
		delete(vm.Tags, t)
	}
}

func (vm *BaseVM) UpdateBW() {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	for i := range vm.Networks {
		n := &vm.Networks[i]
		tap, err := bridges.FindTap(n.Tap)
		if err != nil {
			// weird...
			n.RxRate, n.TxRate = 0, 0
			continue
		}

		n.RxRate, n.TxRate = tap.BandwidthStats()
	}
}

func (vm *BaseVM) SetCCActive(active bool) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.ActiveCC = active
}

func (vm *BaseVM) NetworkConnect(pos int, bridge string, vlan int) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	net := &vm.Networks[pos]

	log.Debug("moving network connection: %v %v %v -> %v %v", vm.ID, pos, net.VLAN, bridge, vlan)

	// Do this before disconnecting from the old bridge in case the new one was
	// mistyped or invalid.
	dst, err := getBridge(bridge)
	if err != nil {
		return err
	}

	// Disconnect from the old bridge, if we were connected
	if net.VLAN != DisconnectedVLAN {
		src, err := getBridge(net.Bridge)
		if err != nil {
			return err
		}

		if err := src.RemoveTap(net.Tap); err != nil {
			return err
		}

		src.ReapTaps()
	}

	// Connect to the new bridge
	if err := dst.AddTap(net.Tap, vlan, false); err != nil {
		return err
	}

	// Record updates to the VM config
	net.VLAN = vlan
	net.Bridge = bridge

	return nil
}

func (vm *BaseVM) NetworkDisconnect(pos int) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	net := &vm.Networks[pos]

	// Don't try to diconnect an interface that is already disconnected...
	if net.VLAN == DisconnectedVLAN {
		return nil
	}

	log.Debug("disconnect network connection: %v %v %v", vm.ID, pos, net)

	br, err := getBridge(net.Bridge)
	if err != nil {
		return err
	}

	if err := br.RemoveTap(net.Tap); err != nil {
		return err
	}

	net.Bridge = ""
	net.VLAN = DisconnectedVLAN

	return nil
}

// info returns information about the VM for the provided key.
func (vm *BaseVM) info(key string) (string, error) {
	if fns, ok := baseConfigFns[key]; ok {
		return fns.Print(&vm.BaseConfig), nil
	}

	var vals []string

	vm.lock.Lock()
	defer vm.lock.Unlock()

	switch key {
	case "id":
		return strconv.Itoa(vm.ID), nil
	case "name":
		return vm.Name, nil
	case "namespace":
		return vm.Namespace, nil
	case "state":
		return vm.State.String(), nil
	case "type":
		return vm.Type.String(), nil
	case "vlan":
		for _, net := range vm.Networks {
			if net.VLAN == DisconnectedVLAN {
				vals = append(vals, "disconnected")
			} else {
				vals = append(vals, printVLAN(net.VLAN))
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
			s := fmt.Sprintf("%.1f/%.1f (rx/tx MB/s)", v.RxRate, v.TxRate)
			vals = append(vals, s)
		}
	case "tags":
		return vm.TagsString(), nil
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
	log.Debug("updating vm %v state: %v -> %v", vm.ID, vm.State, s)
	vm.State = s

	err := ioutil.WriteFile(filepath.Join(vm.instancePath, "state"), []byte(s.String()), 0666)
	if err != nil {
		log.Error("write instance state file: %v", err)
	}
}

// setError updates the vm state and records the error in the vm's tags.
// Assumes that the caller has locked the vm.
func (vm *BaseVM) setError(err error) {
	vm.Tags["error"] = err.Error()
	vm.setState(VM_ERROR)
}

// macSnooper listens for updates from the ipmac learner and updates the
// specified network config.
func (vm *BaseVM) macSnooper(net *NetConfig, updates <-chan ipmac.IP) {
	for update := range updates {
		// TODO: need to acquire VM lock?
		if update.IP4 != "" {
			net.IP4 = update.IP4
		} else if update.IP6 != "" && !strings.HasPrefix(update.IP6, "fe80") {
			net.IP6 = update.IP6
		}
	}
}

// writeTaps writes the vm's taps to disk in the vm's instance path.
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

func (vm *BaseVM) conflicts(vm2 BaseVM) error {
	// Return error if two VMs have same name or UUID
	if vm.Namespace == vm2.Namespace {
		if vm.Name == vm2.Name {
			return fmt.Errorf("duplicate VM name: %s", vm.Name)
		}

		if vm.UUID == vm2.UUID {
			return fmt.Errorf("duplicate VM UUID: %s", vm.UUID)
		}
	}

	// Warn if we see two VMs that share a MAC on the same VLAN
	for _, n := range vm.Networks {
		for _, n2 := range vm2.Networks {
			if n.MAC == n2.MAC && n.VLAN == n2.VLAN {
				log.Warn("duplicate MAC/VLAN: %v/%v for %v and %v", vm.ID, vm2.ID)
			}
		}
	}

	return nil
}

// inNamespace tests whether vm is part of active namespace, if there is one.
// When there isn't an active namespace, all vms return true.
func inNamespace(vm VM) bool {
	if vm == nil {
		return false
	}

	namespace := GetNamespaceName()

	return namespace == "" || vm.GetNamespace() == namespace
}

func vmNotFound(idOrName string) error {
	return fmt.Errorf("vm not found: %v", idOrName)
}

func vmNotRunning(idOrName string) error {
	return fmt.Errorf("vm not running: %v", idOrName)
}

func vmNotKVM(idOrName string) error {
	return fmt.Errorf("vm not KVM: %v", idOrName)
}

func isVmNotFound(err string) bool {
	return strings.HasPrefix(err, "vm not found: ")
}

func getConfig(vm VM) BaseConfig {
	switch vm := vm.(type) {
	case *KvmVM:
		return vm.BaseConfig
	case *ContainerVM:
		return vm.BaseConfig
	}

	return BaseConfig{}
}
