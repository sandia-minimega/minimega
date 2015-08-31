// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"gonetflow"
	"io/ioutil"
	"ipmac"
	"minicli"
	log "minilog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

const (
	DisconnectedVLAN = -1
	TrunkVLAN        = -2
)

// Bridge represents a bridge on the host and the Taps connected to it.
type Bridge struct {
	Name     string
	preExist bool
	iml      *ipmac.IPMacLearner
	nf       *gonetflow.Netflow
	Trunk    []string
	Tunnel   []string

	Taps map[string]Tap

	// Embedded mutex
	sync.Mutex

	// Guards startIML
	once sync.Once
}

type Tap struct {
	lan  int
	host bool
}

const (
	DEFAULT_BRIDGE = "mega_bridge"
	OVS_TIMEOUT    = time.Duration(5 * time.Second)
	TYPE_VXLAN     = 1
	TYPE_GRE       = 2
)

var (
	bridges          map[string]*Bridge // all bridges. mega_bridge0 will be automatically added
	disconnectedTaps map[string]Tap

	tapChan chan string // atomic feeder of tap names, wraps tapCount

	bridgeLock sync.Mutex
	ovsLock    sync.Mutex
)

// create the default bridge struct and create a goroutine to generate
// tap names for this host.
func init() {
	bridges = make(map[string]*Bridge)
	disconnectedTaps = make(map[string]Tap)

	tapChan = make(chan string)

	go func() {
		tapCount := 0
		for {
			tapChan <- fmt.Sprintf("mega_tap%v", tapCount)
			tapCount++
			log.Debug("tapCount: %v", tapCount)
		}
	}()
}

// NewBridge creates a new bridge with ovs, assumes that the bridgeLock is held.
func NewBridge(name string) (*Bridge, error) {
	log.Debug("creating new bridge -- %v", name)
	b := &Bridge{
		Name: name,
		Taps: make(map[string]Tap),
	}

	// Create the bridge
	isNew, err := ovsAddBridge(b.Name)
	if err != nil {
		return nil, err
	}

	b.preExist = !isNew

	// Bring the interface up
	if err := toggleInterface(b.Name, true, false); err != nil {
		if err := ovsDelBridge(b.Name); err != nil {
			// Welp, we're boned
			log.Error("defunct bridge -- %v %v", b.Name, err)
		}

		return nil, err
	}

	return b, nil
}

// destroy a bridge with ovs, and remove all of the taps, etc associated with it
func (b *Bridge) Destroy() error {
	// first get all of the taps off of this bridge and destroy them
	for name, tap := range b.Taps {
		log.Debug("destroying tap %v", name)
		if err := b.TapDestroy(tap.lan, name); err != nil {
			log.Info("Destroy: could not destroy tap: %v", err)
		}
	}

	// don't destroy the bridge if it existed before we started
	if b.preExist {
		return nil
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	b.Lock()
	defer b.Unlock()

	err := toggleInterface(b.Name, false, false)
	if err == nil {
		err = ovsDelBridge(b.Name)
	}
	return err
}

// create and add a tap to a bridge
func (b *Bridge) TapCreate(lan int) (tapName string, err error) {
	defer func() {
		// No error => add tap to lan. Do this in a defer so that the bridge
		// isn't locked.
		if err == nil {
			err = b.TapAdd(lan, tapName, false)
		}
	}()

	b.Lock()
	defer b.Unlock()

	if tapName, err = getNewTap(); err != nil {
		return
	}

	if err = addRemoveTap(tapName, true); err != nil {
		return
	}

	defer func() {
		// If there was an error, remove the tap
		if err != nil {
			if err := addRemoveTap(tapName, false); err != nil {
				// Welp, we're boned
				log.Error("defunct tap -- %v %v", tapName, err)
			}

			tapName = ""
		}
	}()

	// start the ipmaclearner, if need be
	b.once.Do(b.startIML)

	if err = toggleInterface(tapName, true, false); err != nil {
		return
	}

	// the tap add was successful, so try to add it to the bridge
	b.Taps[tapName] = Tap{
		lan:  lan,
		host: false,
	}

	return
}

// add a tap to the bridge
func (b *Bridge) TapAdd(lan int, tap string, host bool) (err error) {
	defer func() {
		if err == ErrAlreadyExists {
			// special case - we own the tap, but it already exists
			// on the bridge. simply remove and add it again.
			log.Info("tap %v is already on bridge, readding", tap)
			err = b.TapRemove(lan, tap)
			if err == nil {
				err = b.TapAdd(lan, tap, host)
			}
		}

		if err == nil {
			// if this tap is in the disconnected list, move it out
			bridgeLock.Lock()
			defer bridgeLock.Unlock()

			b.Lock()
			defer b.Unlock()

			if _, ok := disconnectedTaps[tap]; ok {
				b.Taps[tap] = disconnectedTaps[tap]
				delete(disconnectedTaps, tap)
			}
		}
	}()

	b.Lock()
	defer b.Unlock()

	// start the ipmaclearner, if need be
	b.once.Do(b.startIML)

	return ovsAddPort(b.Name, tap, lan, false)
}

// destroy and remove a tap from a bridge
func (b *Bridge) TapDestroy(lan int, tap string) (err error) {
	if err = b.TapRemove(lan, tap); err != nil {
		log.Info("TapDestroy: could not remove tap: %v", err)
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	// if it's a host tap, then ovs removed it for us and we don't need to continue
	if _, ok := disconnectedTaps[tap]; !ok {
		return nil
	}

	b.Lock()
	defer b.Unlock()

	if err = toggleInterface(tap, false, false); err != nil {
		return
	}

	return addRemoveTap(tap, false)
}

// remove a tap from a bridge
func (b *Bridge) TapRemove(lan int, tap string) (err error) {
	// No-op is the VLAN is already disconnected
	if lan == DisconnectedVLAN {
		return
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	b.Lock()
	defer b.Unlock()

	if !b.Taps[tap].host { // don't move host taps, just delete them
		disconnectedTaps[tap] = b.Taps[tap]
	}
	delete(b.Taps, tap)

	return ovsDelPort(b.Name, tap)
}

// startIML starts the MAC listener.
//
// Note: assumes the bridge is locked.
func (b *Bridge) startIML() {
	// use openflow to redirect arp and icmp6 traffic to the local tap
	filters := []string{
		"dl_type=0x0806,actions=local,normal",
		"dl_type=0x86dd,nw_proto=58,icmp_type=135,actions=local,normal",
	}
	for _, filter := range filters {
		if err := addOpenflow(b.Name, filter); err != nil {
			log.Error("cannot start ip learner on bridge: %v", err)
			return
		}
	}

	iml, err := ipmac.NewLearner(b.Name)
	if err != nil {
		log.Error("cannot start ip learner on bridge: %v", err)
		return
	}

	b.iml = iml
}

// update the active timeout on a nf object
func (b *Bridge) UpdateNFTimeout(t int) error {
	if b.nf == nil {
		return fmt.Errorf("bridge %v has no netflow object", b.Name)
	}

	args := []string{
		"set",
		"NetFlow",
		b.Name,
		fmt.Sprintf("active_timeout=%v", t),
	}
	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("UpdateNFTimeout: %v: %v", err, sErr)
	}

	return nil
}

// create a new netflow object for the specified bridge
func (b *Bridge) NewNetflow(timeout int) (*gonetflow.Netflow, error) {
	nf, port, err := gonetflow.NewNetflow()
	if err != nil {
		return nil, err
	}

	b.Lock()
	defer b.Unlock()

	// connect openvswitch to our new netflow object
	args := []string{
		"--",
		"set",
		"Bridge",
		b.Name,
		"netflow=@nf",
		"--",
		"--id=@nf",
		"create",
		"NetFlow",
		fmt.Sprintf("targets=\"127.0.0.1:%v\"", port),
		fmt.Sprintf("active-timeout=%v", timeout),
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return nil, fmt.Errorf("NewNetflow: could not enable netflow: %v: %v", err, sErr)
	}

	b.nf = nf

	return nf, nil
}

// remove an active netflow object
func (b *Bridge) DestroyNetflow() error {
	b.Lock()
	defer b.Unlock()

	if b.nf == nil {
		return fmt.Errorf("bridge %v has no netflow object", b.Name)
	}

	b.nf.Stop()

	// connect openvswitch to our new netflow object
	args := []string{
		"clear",
		"Bridge",
		b.Name,
		"netflow",
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("DestroyNetflow: %v: %v", err, sErr)
	}

	b.nf = nil

	return nil
}

// add a vxlan or GRE tunnel to a bridge
func (b *Bridge) TunnelAdd(t int, remoteIP string) error {
	var tunnelType string
	switch t {
	case TYPE_VXLAN:
		tunnelType = "vxlan"
	case TYPE_GRE:
		tunnelType = "gre"
	default:
		return fmt.Errorf("invalid tunnel type: %v", t)
	}

	tapName, err := getNewTap()
	if err != nil {
		return err
	}

	b.Lock()
	defer b.Unlock()

	args := []string{
		"add-port",
		b.Name,
		tapName,
		"--",
		"set",
		"interface",
		tapName,
		fmt.Sprintf("type=%v", tunnelType),
		fmt.Sprintf("options:remote_ip=%v", remoteIP),
	}
	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("TunnelAdd: %v: %v", err, sErr)
	}

	b.Tunnel = append(b.Tunnel, tapName)

	return nil
}

// remove trunk port from a bridge
func (b *Bridge) TunnelRemove(iface string) error {
	// find this iface in the tunnel list
	index := -1
	for i, v := range b.Tunnel {
		if v == iface {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("no tunnel port %v on bridge %v", iface, b.Name)
	}

	err := ovsDelPort(b.Name, b.Tunnel[index])
	if err == nil {
		b.Tunnel = append(b.Tunnel[:index], b.Tunnel[index+1:]...)
	}

	return err
}

// add an interface as a trunk port to a bridge
func (b *Bridge) TrunkAdd(iface string) error {
	err := ovsAddPort(b.Name, iface, TrunkVLAN, false)
	if err == nil {
		b.Trunk = append(b.Trunk, iface)
	}

	return err
}

// remove trunk port from a bridge
func (b *Bridge) TrunkRemove(iface string) error {
	// find this iface in the trunk list
	index := -1
	for i, v := range b.Trunk {
		if v == iface {
			index = i
			break
		}
	}
	if index == -1 {
		return fmt.Errorf("no trunk port %v on bridge %v", iface, b.Name)
	}

	err := ovsDelPort(b.Name, b.Trunk[index])
	if err == nil {
		b.Trunk = append(b.Trunk[:index], b.Trunk[index+1:]...)
	}

	return err
}

// return a pointer to the specified bridge, creating it if it doesn't already
// exist. If b == "", return the default bridge
func getBridge(b string) (*Bridge, error) {
	if b == "" {
		b = DEFAULT_BRIDGE
	}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	if v, ok := bridges[b]; ok {
		return v, nil
	}

	bridge, err := NewBridge(b)
	if err != nil {
		return nil, err
	}

	bridges[b] = bridge

	updateBridgeInfo()

	return bridge, nil
}

func enumerateBridges() []string {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	var ret []string
	for k, _ := range bridges {
		ret = append(ret, k)
	}
	return ret
}

// return the netflow object of a current bridge
func getNetflowFromBridge(b string) (*gonetflow.Netflow, error) {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	if v, ok := bridges[b]; ok {
		if v.nf == nil {
			return nil, fmt.Errorf("bridge %v has no netflow object", b)
		}
		return v.nf, nil
	} else {
		return nil, fmt.Errorf("no such bridge %v", b)
	}
}

// return a pointer to a bridge that has tap t attached to it, or error
func getBridgeFromTap(t string) (*Bridge, error) {
	log.Debugln("getBridgeFromTap")

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for k, b := range bridges {
		for tap, _ := range b.Taps {
			if tap == t {
				log.Debug("found tap %v in bridge %v", t, k)
				return b, nil
			}
		}
	}

	return nil, fmt.Errorf("tap %v not found", t)
}

// destroy all bridges
func bridgesDestroy() error {
	var e []string
	for k, v := range bridges {
		err := v.Destroy()
		if err != nil {
			e = append(e, err.Error())
		}
		bridgeLock.Lock()
		delete(bridges, k)
		bridgeLock.Unlock()
	}
	bridgeLock.Lock()
	updateBridgeInfo()
	bridgeLock.Unlock()
	bridgeFile := *f_base + "bridges"
	err := os.Remove(bridgeFile)
	if err != nil {
		log.Error("bridgesDestroy: could not remove bridge file: %v", err)
	}
	if len(e) == 0 {
		return nil
	} else {
		return errors.New(strings.Join(e, " : "))
	}
}

// return formatted bridge info. expected to be called with bridgeLock set
func bridgeInfo() string {
	if len(bridges) == 0 {
		return ""
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Bridge\tExisted before minimega\tActive VLANS\n")
	for _, v := range bridges {
		vlans := map[int]bool{}
		for _, tap := range v.Taps {
			vlans[tap.lan] = true
		}

		vlans2 := []int{}
		for k, _ := range vlans {
			vlans2 = append(vlans2, k)
		}
		sort.Ints(vlans2)

		fmt.Fprintf(w, "%v\t%v\t%v\n", v.Name, v.preExist, vlans2)
	}

	w.Flush()
	return o.String()
}

// called with bridgeLock set
func updateBridgeInfo() {
	log.Debugln("updateBridgeInfo")
	i := bridgeInfo()
	path := filepath.Join(*f_base, "bridges")
	err := ioutil.WriteFile(path, []byte(i), 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

func (b *Bridge) CreateBridgeMirror() (string, error) {
	// get a host tap
	tapName, err := hostTapCreate(b.Name, "0", "none", "")
	if err != nil {
		return "", err
	}

	// create the mirror for this bridge
	args := []string{
		"--",
		"--id=@p",
		"get",
		"port",
		tapName,
		"--",
		"--id=@m",
		"create",
		"mirror",
		"name=m0",
		"select-all=true",
		"output-port=@p",
		"--",
		"set",
		"bridge",
		b.Name,
		"mirrors=@m",
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return "", fmt.Errorf("openvswitch: %v: %v", err, sErr)
	}

	return tapName, nil
}

func (b *Bridge) DeleteBridgeMirror(tap string) error {
	// delete the mirror for this bridge
	args := []string{
		"clear",
		"bridge",
		b.Name,
		"mirrors",
	}

	if _, sErr, err := ovsCmdWrapper(args); err != nil {
		return fmt.Errorf("DeleteBridgeMirror: %v: %v", err, sErr)
	}

	// delete the associated host tap
	return hostTapDelete(tap)
}

func hostTapList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "vlan"}
	resp.Tabular = [][]string{}

	// find all the host taps first
	for k, b := range bridges {
		for name, tap := range b.Taps {
			if tap.host {
				resp.Tabular = append(resp.Tabular, []string{
					k, name, strconv.Itoa(tap.lan),
				})
			}
		}
	}
}

func hostTapDelete(tap string) error {
	var c []*Bridge
	// special case, *, which should delete all host taps from all bridges
	if tap == Wildcard {
		for _, v := range bridges {
			c = append(c, v)
		}
	} else {
		b, err := getBridgeFromTap(tap)
		if err != nil {
			return err
		}
		c = append(c, b)
	}
	for _, b := range c {
		if tap == Wildcard {
			// remove all host taps on this vlan
			for name, t := range b.Taps {
				if t.host {
					b.TapRemove(t.lan, name)
				}
			}
			continue
		}
		if t, ok := b.Taps[tap]; ok {
			if !t.host {
				return fmt.Errorf("not a host tap")
			}
			b.TapRemove(t.lan, tap)
		}
	}
	return nil
}

func hostTapCreate(bridge, lan, ip, tapName string) (string, error) {
	b, err := getBridge(bridge)
	if err != nil {
		return "", err
	}
	r, err := strconv.Atoi(lan)
	if err != nil {
		return "", err
	}

	if tapName == "" {
		tapName, err = getNewTap()
		if err != nil {
			return "", err
		}
	}

	// create the tap
	b.Lock()
	b.Taps[tapName] = Tap{
		lan:  r,
		host: true,
	}
	b.Unlock()
	err = b.TapAdd(r, tapName, true)
	if err != nil {
		return "", err
	}

	// bring the tap up
	if err := toggleInterface(tapName, true, true); err != nil {
		return "", err
	}

	if strings.ToLower(ip) == "none" {
		return tapName, nil
	}

	if strings.ToLower(ip) == "dhcp" {
		var sOut bytes.Buffer
		var sErr bytes.Buffer

		p := process("dhcp")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				tapName,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Debug("obtaining dhcp on tap %v", tapName)

		if err = cmd.Run(); err != nil {
			return "", fmt.Errorf("%v: %v", err, sErr.String())
		}
	} else {
		var sOut bytes.Buffer
		var sErr bytes.Buffer

		p := process("ip")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				"addr",
				"add",
				"dev",
				tapName,
				ip,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Debug("setting ip on tap %v", tapName)

		if err = cmd.Run(); err != nil {
			return "", fmt.Errorf("%v: %v", err, sErr.String())
		}
	}

	return tapName, nil
}

// gets a new tap from tapChan and verifies that it doesn't already exist
func getNewTap() (string, error) {
	var t string
	for {
		t = <-tapChan
		taps, err := ioutil.ReadDir("/sys/class/net")
		if err != nil {
			return "", err
		}
		found := false
		for _, v := range taps {
			if v.Name() == t {
				found = true
				log.Warn("tap %v already exists, trying again", t)
			}
		}
		if !found {
			break
		}
	}
	return t, nil
}

// toggleInterface activates or deactivates an interface based on the activate
// parameter using the `ip` command.
func toggleInterface(name string, activate, promisc bool) error {
	var sErr bytes.Buffer

	direction := "up"
	if !activate {
		direction = "down"
	}

	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			name,
			direction,
		},
		Env:    nil,
		Dir:    "",
		Stderr: &sErr,
	}
	if activate && promisc {
		cmd.Args = append(cmd.Args, "promisc")
		cmd.Args = append(cmd.Args, "on")
	}

	log.Debug("bringing bridge %v with cmd: %v", direction, cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip: %v: %v", err, sErr.String())
	}

	return nil
}

// addRemoveTap adds or removes a tuntap based on the add parameter using the
// `ip` command.
func addRemoveTap(name string, add bool) error {
	var sErr bytes.Buffer

	direction := "add"
	if !add {
		direction = "del"
	}

	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"tuntap",
			direction,
			"mode",
			"tap",
			name,
		},
		Env:    nil,
		Dir:    "",
		Stderr: &sErr,
	}
	log.Debug("%v tap %v with cmd: %v", direction, name, cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ip: %v: %v", err, sErr.String())
	}

	return nil
}

func addOpenflow(bridge, filter string) error {
	ovsLock.Lock()
	defer ovsLock.Unlock()

	var sOut bytes.Buffer
	var sErr bytes.Buffer

	p := process("openflow")

	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"add-flow",
			bridge,
			filter,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("adding flow with cmd: %v", cmd)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("openflow: %v: %v", err, sErr.String())
	}

	return nil
}
