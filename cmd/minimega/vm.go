// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var vmID *Counter // channel of new VM IDs, shared across namespaces

type VMType int

const (
	_ VMType = iota
	KVM
	CONTAINER
)

type VM interface {
	GetID() int           // GetID returns the VM's per-host unique ID
	GetPID() int          // GetPID returns the VM's PID
	GetName() string      // GetName returns the VM's per-host unique name
	GetNamespace() string // GetNamespace returns the VM's namespace name
	GetHost() string      // GetHost returns the hostname that the VM is running on
	GetState() VMState
	GetLaunchTime() time.Time // GetLaunchTime returns the time when the VM was launched
	GetType() VMType
	GetInstancePath() string
	GetUUID() string
	GetCPUs() uint64
	GetMem() uint64
	GetCoschedule() int

	GetNetwork(i int) (NetConfig, error) // GetNetwork returns the ith NetConfigs associated with the vm.
	GetNetworks() []NetConfig            // GetNetworks returns an ordered, deep copy of the NetConfigs associated with the vm.

	// Lifecycle functions
	Launch() error
	Kill() error
	Start() error
	Stop() error
	Flush() error

	String() string
	Info(string) (string, error)

	Screenshot(int) ([]byte, error)

	Tag(string) string          // Tag gets the value of the given tag
	SetTag(string, string)      // SetTag updates the given tag
	GetTags() map[string]string // GetTags returns a copy of the tags
	ClearTag(string)            // ClearTag deletes one or all tags

	// Conflicts checks whether the VMs have conflicting configs. Called
	// when we create a VM but before adding it to the list of VMs.
	Conflicts(VM) error

	SetCCActive(bool)
	HasCC() bool
	Connect(*ron.Server, bool) error
	Disconnect(*ron.Server) error

	UpdateNetworks()

	// NetworkConnect updates the VM's config to reflect that it has been
	// connected to the specified VLAN and Bridge.
	NetworkConnect(int, int, string) error

	// NetworkDisconnect updates the VM's config to reflect that the specified
	// tap has been disconnected.
	NetworkDisconnect(int) error

	// Qos functions
	GetQos() [][]bridge.QosOption
	UpdateQos(uint, bridge.QosOption) error
	ClearQos(uint) error
	ClearAllQos() error

	ProcStats() (map[int]*ProcStats, error)

	// WriteConfig writes the VM's config to the provided writer.
	WriteConfig(io.Writer) error

	// Make a deep copy that shouldn't be used for anything but reads
	Copy() VM
}

// BaseVM provides the bare-bones for base VM functionality. It implements
// several functions from the VM interface that are relatively common. All
// newly created VM types will most likely embed this struct to reuse the base
// functionality.
type BaseVM struct {
	BaseConfig // embed

	ID        int
	Name      string
	Namespace string
	Host      string // hostname where this VM is running

	State      VMState
	LaunchTime time.Time
	Type       VMType
	ActiveCC   bool // set when CC is active

	Pid int

	lock sync.Mutex // synchronizes changes to this VM
	cond *sync.Cond

	kill chan bool // channel to signal the vm to shut down

	instancePath string
}

// Valid names for output masks for `vm info`, in preferred output order
var vmInfo = []string{
	// generic fields
	"id", "name", "state", "uptime", "type", "uuid", "cc_active", "pid",
	// network fields
	"vlan", "bridge", "tap", "mac", "ip", "ip6", "qos", "qinq",
	// more generic fields but want next to vcpus
	"memory",
	// kvm fields
	"vcpus", "disks", "snapshot", "initrd", "kernel", "cdrom", "migrate",
	"append", "serial-ports", "virtio-ports", "vnc_port",
	// container fields
	"filesystem", "hostname", "init", "preinit", "fifo", "volume",
	"console_port",
	// more generic fields (tags can be huge so throw it at the end)
	"tags",
}

// Valid names for output masks for `vm summary`, in preferred output order
var vmInfoLite = []string{
	// generic fields
	"id", "name", "state", "type", "uuid", "cc_active",
	// network fields
	"vlan",
}

func init() {
	vmID = NewCounter()

	// for serializing VMs
	gob.Register([]VM{})
	gob.Register(&KvmVM{})
	gob.Register(&ContainerVM{})
}

func NewVM(name, namespace string, vmType VMType, config VMConfig) (VM, error) {
	switch vmType {
	case KVM:
		return NewKVM(name, namespace, config)
	case CONTAINER:
		return NewContainer(name, namespace, config)
	}

	return nil, errors.New("unknown VM type")
}

// NewBaseVM creates a new VM, copying the specified configs. After a VM is
// created, it can be Launched.
func NewBaseVM(name, namespace string, config VMConfig) *BaseVM {
	vm := new(BaseVM)

	vm.BaseConfig = config.BaseConfig.Copy() // deep-copy configured fields
	vm.ID = vmID.Next()
	if name == "" {
		vm.Name = fmt.Sprintf("vm-%d", vm.ID)
	} else {
		vm.Name = name
	}

	vm.Namespace = namespace
	vm.Host = hostname

	// generate a UUID if we don't have one
	if vm.UUID == "" {
		vm.UUID = generateUUID()
	}

	// Initialize tags, if not already
	if vm.Tags == nil {
		vm.Tags = map[string]string{}
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
	vm.LaunchTime = time.Now()

	vm.cond = &sync.Cond{L: &vm.lock}

	// New VMs are returned pre-locked. This ensures that the first operation
	// called on a new VM is Launch.
	vm.lock.Lock()

	return vm
}

// copy a BaseVM... assume that lock is held.
func (vm *BaseVM) copy() *BaseVM {
	vm2 := new(BaseVM)

	// Make copies of all fields except lock/kill
	vm2.BaseConfig = vm.BaseConfig.Copy()
	vm2.ID = vm.ID
	vm2.Name = vm.Name
	vm2.Namespace = vm.Namespace
	vm2.Host = vm.Host
	vm2.State = vm.State
	vm2.LaunchTime = vm.LaunchTime
	vm2.Type = vm.Type
	vm2.ActiveCC = vm.ActiveCC
	vm2.instancePath = vm.instancePath
	vm2.Pid = vm.Pid

	return vm2
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

func (vm *BaseVM) GetID() int {
	return vm.ID
}

func (vm *BaseVM) GetName() string {
	return vm.Name
}

func (vm *BaseVM) GetNamespace() string {
	return vm.Namespace
}

func (vm *BaseVM) GetNetwork(i int) (NetConfig, error) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if len(vm.Networks) <= i {
		return NetConfig{}, fmt.Errorf("no such interface %v for %v", i, vm.Name)
	}

	return vm.Networks[i], nil
}

func (vm *BaseVM) GetNetworks() []NetConfig {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	// Make a deep copy of the NetConfigs
	n := make([]NetConfig, len(vm.Networks))
	copy(n, vm.Networks)

	return n
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

func (vm *BaseVM) GetLaunchTime() time.Time {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.LaunchTime
}

func (vm *BaseVM) GetType() VMType {
	return vm.Type
}

func (vm *BaseVM) GetInstancePath() string {
	return vm.instancePath
}

func (vm *BaseVM) GetCPUs() uint64 {
	return vm.VCPUs
}

func (vm *BaseVM) GetMem() uint64 {
	return vm.Memory
}

func (vm *BaseVM) GetCoschedule() int {
	return int(vm.Coschedule)
}

func (vm *BaseVM) GetPID() int {
	return vm.Pid
}

// Kill a VM. Blocks until the VM process has terminated.
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

	// wait until the VM is in an unkillable state (it must have been killed)
	for vm.State&VM_KILLABLE != 0 {
		vm.cond.Wait()
	}

	return nil
}

func (vm *BaseVM) Flush() error {
	namespacesDir := filepath.Join(*f_base, "namespaces")
	namespaceAliasDir := filepath.Join(namespacesDir, vm.Namespace)
	vmAlias := filepath.Join(namespaceAliasDir, vm.UUID)

	// remove just the symlink to the instance path
	if err := os.Remove(vmAlias); err != nil {
		return err
	}

	// try removing the <namespace> directory, but let it fail if not empty
	os.Remove(namespaceAliasDir)

	// try removing the namespaces/ directory, but let it fail if not empty
	os.Remove(namespacesDir)

	// remove the actual instance path
	if err := os.RemoveAll(vm.instancePath); err != nil {
		return err
	}

	return nil
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

func (vm *BaseVM) UpdateNetworks() {
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

		n.IP4 = tap.IP4
		n.IP6 = tap.IP6
	}
}

func (vm *BaseVM) UpdateQos(tap uint, op bridge.QosOption) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if tap >= uint(len(vm.Networks)) {
		return fmt.Errorf("invalid tap index specified: %d", tap)
	}

	bName := vm.Networks[tap].Bridge
	tapName := vm.Networks[tap].Tap

	br, err := getBridge(bName)
	if err != nil {
		return err
	}
	return br.UpdateQos(tapName, op)
}

func (vm *BaseVM) ClearAllQos() error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	for _, nc := range vm.Networks {
		b, err := getBridge(nc.Bridge)
		if err != nil {
			log.Error("failed to get bridge %s for vm %s", nc.Bridge, vm.GetName())
			return err
		}
		err = b.RemoveQos(nc.Tap)
		if err != nil {
			log.Error("failed to remove qos from vm %s", vm.GetName())
			return err
		}
	}
	return nil
}

func (vm *BaseVM) ClearQos(tap uint) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	if tap >= uint(len(vm.Networks)) {
		return fmt.Errorf("invalid tap index specified: %d", tap)
	}
	nc := vm.Networks[tap]
	b, err := getBridge(nc.Bridge)
	if err != nil {
		return err
	}

	return b.RemoveQos(nc.Tap)
}

func (vm *BaseVM) GetQos() [][]bridge.QosOption {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	var res [][]bridge.QosOption

	for _, nc := range vm.Networks {
		b, err := getBridge(nc.Bridge)
		if err != nil {
			log.Error("failed to get bridge %s for vm %s", nc.Bridge, vm.GetName())
			continue
		}

		q := b.GetQos(nc.Tap)
		if q != nil {
			res = append(res, q)
		}
	}
	return res
}

func (vm *BaseVM) SetCCActive(active bool) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	vm.ActiveCC = active
}

func (vm *BaseVM) HasCC() bool {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.ActiveCC
}

func (vm *BaseVM) NetworkConnect(pos, vlan int, bridge string) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.networkConnect(pos, vlan, bridge)
}

// networkConnect assumes that the VM lock is held.
func (vm *BaseVM) networkConnect(pos, vlan int, bridge string) error {
	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	nic := &vm.Networks[pos]

	// special case -- if bridge is not specified, reconnect tap to the same
	// bridge if it is already on a bridge.
	if bridge == "" {
		bridge = nic.Bridge
	}
	// fallback -- connect to the default bridge.
	if bridge == "" {
		bridge = DefaultBridge
	}

	log.Info("moving network connection: %v %v %v:%v -> %v:%v", vm.ID, pos, nic.Bridge, nic.VLAN, bridge, vlan)

	// Do this before disconnecting from the old bridge in case the new one was
	// mistyped or invalid.
	dst, err := getBridge(bridge)
	if err != nil {
		return err
	}

	// Disconnect from the old bridge, if we were connected
	if nic.VLAN != DisconnectedVLAN {
		src, err := getBridge(nic.Bridge)
		if err != nil {
			return err
		}

		if err := src.RemoveTap(nic.Tap); err != nil {
			return err
		}
	}

	// Connect to the new bridge
	if err := dst.AddTap(nic.Tap, nic.MAC, vlan, false); err != nil {
		return err
	}

	// Record updates to the VM config
	nic.Alias = ""
	if alias, err := vlans.GetAlias(vlan); err == nil {
		if alias.Namespace != vm.Namespace {
			nic.Alias = alias.String()
		} else {
			nic.Alias = alias.Value
		}
	}
	nic.VLAN = vlan
	nic.Bridge = bridge

	// TODO: what to do with nic.Raw?

	return nil
}

func (vm *BaseVM) NetworkDisconnect(pos int) error {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	return vm.networkDisconnect(pos)
}

// networkDisconnect assumes that the VM lock is held.
func (vm *BaseVM) networkDisconnect(pos int) error {
	if len(vm.Networks) <= pos {
		return fmt.Errorf("no network %v, VM only has %v networks", pos, len(vm.Networks))
	}

	nic := &vm.Networks[pos]

	// Don't try to diconnect an interface that is already disconnected...
	if nic.VLAN == DisconnectedVLAN {
		return nil
	}

	log.Debug("disconnect network connection: %v %v %v", vm.ID, pos, nic)

	br, err := getBridge(nic.Bridge)
	if err != nil {
		return err
	}

	if err := br.RemoveTap(nic.Tap); err != nil {
		return err
	}

	nic.Alias = ""
	nic.Bridge = ""
	nic.VLAN = DisconnectedVLAN

	// TODO: what to do with nic.Raw?

	return nil
}

// info returns information about the VM for the provided field.
func (vm *BaseVM) Info(field string) (string, error) {
	vm.lock.Lock()
	defer vm.lock.Unlock()

	var vals []string

	switch field {
	case "id":
		return strconv.Itoa(vm.ID), nil
	case "pid":
		return strconv.Itoa(vm.Pid), nil
	case "name":
		return vm.Name, nil
	case "state":
		return vm.State.String(), nil
	case "uptime":
		return time.Since(vm.LaunchTime).String(), nil
	case "type":
		return vm.Type.String(), nil
	case "vlan":
		for _, net := range vm.Networks {
			if net.VLAN == DisconnectedVLAN {
				vals = append(vals, "disconnected")
			} else {
				vals = append(vals, printVLAN(vm.Namespace, net.VLAN))
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
	case "qos":
		for idx, v := range vm.Networks {
			s := vm.QosString(v.Bridge, v.Tap, strconv.Itoa(idx))
			if s != "" {
				vals = append(vals, s)
			}
		}
	case "qinq":
		for _, v := range vm.Networks {
			if v.QinQ {
				vals = append(vals, fmt.Sprintf("%s (%d)", v.Tap, v.VLAN))
			}
		}
	case "tags":
		return marshal(vm.Tags), nil
	case "cc_active":
		return strconv.FormatBool(vm.ActiveCC), nil
	default:
		// at this point, hopefully field is part of BaseConfig
		return vm.BaseConfig.Info(field)
	}

	return "[" + strings.Join(vals, ", ") + "]", nil
}

// setState updates the vm state, and write the state to file. Assumes that the
// caller has locked the vm.
func (vm *BaseVM) setState(s VMState) {
	log.Debug("updating vm %v state: %v -> %v", vm.ID, vm.State, s)
	vm.State = s

	mustWrite(vm.path("state"), s.String())
}

// setErrorf logs the error, updates the vm state, and records the error in the
// vm's tags. Assumes that the caller has locked the vm. Returns the final
// error.
func (vm *BaseVM) setErrorf(format string, arg ...interface{}) error {
	// create the error
	err := fmt.Errorf(format, arg...)

	log.Error("vm %v: %v", vm.ID, err)
	vm.Tags["error"] = err.Error()
	vm.setState(VM_ERROR)

	return err
}

// writeTaps writes the vm's taps to disk in the vm's instance path.
func (vm *BaseVM) writeTaps() error {
	taps := []string{}
	for _, net := range vm.Networks {
		taps = append(taps, net.Tap)
	}

	f := vm.path("taps")
	if err := ioutil.WriteFile(f, []byte(strings.Join(taps, "\n")), 0666); err != nil {
		return fmt.Errorf("write instance taps file: %v", err)
	}

	return nil
}

func (vm *BaseVM) conflicts(vm2 *BaseVM) error {
	// Return error if two VMs have same name or UUID
	if vm.Name == vm2.Name {
		return fmt.Errorf("duplicate VM name: %s", vm.Name)
	}

	if vm.UUID == vm2.UUID {
		return fmt.Errorf("duplicate VM UUID: %s", vm.UUID)
	}

	// Warn if we see two VMs that share a MAC on the same VLAN
	for _, n := range vm.Networks {
		for _, n2 := range vm2.Networks {
			if n.MAC == n2.MAC && n.VLAN == n2.VLAN {
				log.Warn("duplicate MAC/VLAN: %v/%v for %v and %v", n.MAC, n.VLAN, vm.ID, vm2.ID)
			}
		}
	}

	return nil
}

// path joins instancePath with provided path
func (vm *BaseVM) path(s string) string {
	return filepath.Join(vm.instancePath, s)
}

func (vm *BaseVM) createInstancePathAlias() error {
	// create the namespaces/<namespace> directory
	namespaceAliasDir := filepath.Join(*f_base, "namespaces", vm.GetNamespace())
	if err := os.MkdirAll(namespaceAliasDir, os.FileMode(0700)); err != nil {
		return fmt.Errorf("unable to create namespace dir: %v", err)
	}

	// create a symlink under namespaces/<namespace> to the instance path
	// only if it does not already exist, otherwise error
	vmAlias := filepath.Join(namespaceAliasDir, vm.GetUUID())
	if _, err := os.Stat(vmAlias); err == nil {
		// symlink already exists
		return fmt.Errorf("unable to create VM dir symlink: %v already exists", vmAlias)
	}
	if err := os.Symlink(vm.GetInstancePath(), vmAlias); err != nil {
		return fmt.Errorf("unable to create VM dir symlink: %v", err)
	}

	return nil
}

func writeVMConfig(vm VM) error {
	log.Info("writing vm config")

	name := filepath.Join(vm.GetInstancePath(), "config")
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)

	if err != nil {
		return err
	}
	defer f.Close()

	return vm.WriteConfig(f)
}

func vmNotFound(name string) error {
	return fmt.Errorf("vm not found: %v", name)
}

func vmNotRunning(name string) error {
	return fmt.Errorf("vm not running: %v", name)
}

func vmNotKVM(name string) error {
	return fmt.Errorf("vm not KVM: %v", name)
}

func vmNotContainer(name string) error {
	return fmt.Errorf("vm not container: %v", name)
}

func isVMNotFound(err string) bool {
	return strings.HasPrefix(err, "vm not found: ")
}
