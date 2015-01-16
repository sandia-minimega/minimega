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
	"strconv"
	"strings"
	"sync"
	"text/tabwriter"
	"time"
)

// a bridge representation that includes a list of vlans and their respective
// taps.
type bridge struct {
	Name     string
	lans     map[int]*vlan
	exists   bool // false until the first usage, then true until destroyed.
	preExist bool
	iml      *ipmac.IPMacLearner
	Lock     sync.Mutex
	nf       *gonetflow.Netflow
}

type vlan struct {
	Taps map[string]*tap // named list of taps.
}

type tap struct {
	host       bool
	hostOption string
}

const (
	DEFAULT_BRIDGE = "mega_bridge"
	OVS_TIMEOUT    = time.Duration(5 * time.Second)
)

var (
	bridges          map[string]*bridge // all bridges. mega_bridge0 will be automatically added
	bridgeLock       sync.Mutex
	tapCount         int         // total number of allocated taps on this host
	tapChan          chan string // atomic feeder of tap names, wraps tapCount
	disconnectedTaps map[string]*tap
	ovsLock          sync.Mutex
)

var bridgeCLIHandlers = []minicli.Handler{
	{ // tap
		HelpShort: "control host taps for communicating between hosts and VMs",
		HelpLong: `
Control host taps on a named vlan for communicating between a host and any VMs
on that vlan.

Calling tap with no arguments will list all created taps.

To create a tap on a particular vlan, invoke tap with the create command:

	tap create <vlan> <ip/dhcp>

For example, to create a host tap with ip and netmask 10.0.0.1/24 on VLAN 5:

	tap create 5 10.0.0.1/24

Optionally, you can specify the bridge to create the host tap on:

	tap create <bridge> <vlan> <ip/dhcp>

You can also optionally specify the tap name, otherwise the tap will be in the
form of mega_tapX.

Additionally, you can bring the tap up with DHCP by using "dhcp" instead of a
ip/netmask:

	tap create 5 dhcp

To delete a host tap, use the delete command and tap name from the tap list:

	tap delete <id>

To delete all host taps, use id -1, or 'clear tap':

	tap delete -1`,
		Patterns: []string{
			"tap",
			"tap create <vlan> [tap name]",
			"tap create <vlan> bridge <bridge> [tap name]",
			"tap create <vlan> <dhcp,> [tap name]",
			"tap create <vlan> ip <ip> [tap name]",
			"tap create <vlan> bridge <bridge> <dhcp,> [tap name]",
			"tap create <vlan> bridge <bridge> ip <ip> [tap name]",
			"tap delete <id or *>",
			"clear tap",
		},
		Call: wrapSimpleCLI(cliHostTap),
	},
	{ // bridge
		HelpShort: "display information about virtual bridges",
		Patterns: []string{
			"bridge",
		},
		Call: wrapSimpleCLI(cliBridgeInfo),
	},
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if isClearCommand(c) {
		err := hostTapDelete("*")
		if err != nil {
			resp.Error = err.Error()
		}
	} else if c.StringArgs["vlan"] != "" {
		// Must be one of the create commands
		vlan := c.StringArgs["vlan"]

		bridge := c.StringArgs["bridge"]
		if bridge == "" {
			bridge = DEFAULT_BRIDGE
		}

		ip := c.StringArgs["ip"]
		if c.BoolArgs["dhcp"] {
			ip = "dhcp"
		} else if ip == "" {
			ip = "none"
		}

		tapName := c.StringArgs["tap"]

		tapName, err := hostTapCreate(bridge, vlan, ip, tapName)
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Response = tapName
		}
	} else if c.StringArgs["id"] != "" {
		// Must be the delete command
		err := hostTapDelete(c.StringArgs["id"])
		if err != nil {
			resp.Error = err.Error()
		}
	} else {
		// Must be the list command
		hostTapList(resp)
	}

	return resp
}

func cliBridgeInfo(c *minicli.Command) *minicli.Response {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	resp := &minicli.Response{
		Host:     hostname,
		Response: bridgeInfo(),
	}

	return resp
}

// create the default bridge struct and create a goroutine to generate
// tap names for this host.
func init() {
	registerHandlers("bridge", bridgeCLIHandlers)

	bridges = make(map[string]*bridge)
	tapChan = make(chan string)
	disconnectedTaps = make(map[string]*tap)
	go func() {
		for {
			tapChan <- fmt.Sprintf("mega_tap%v", tapCount)
			tapCount++
			log.Debug("tapCount: %v", tapCount)
		}
	}()
}

// return a pointer to the specified bridge, creating it if it doesn't already
// exist. If b == "", return the default bridge
func getBridge(b string) (*bridge, error) {
	if b == "" {
		b = DEFAULT_BRIDGE
	}
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	if v, ok := bridges[b]; ok {
		return v, nil
	}
	bridges[b] = &bridge{
		Name: b,
	}
	err := bridges[b].create()
	if err != nil {
		delete(bridges, b)
		return nil, err
	}
	updateBridgeInfo()
	return bridges[b], nil
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
func getBridgeFromTap(t string) (*bridge, error) {
	log.Debugln("getBridgeFromTap")
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	for k, b := range bridges {
		for _, l := range b.lans {
			for tap, _ := range l.Taps {
				if tap == t {
					log.Debug("found tap %v in bridge %v", t, k)
					return b, nil
				}
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
	fmt.Fprintf(w, "Bridge\tExists\tExisted before minimega\tActive VLANS\n")
	for _, v := range bridges {
		var vlans []int
		for v, _ := range v.lans {
			vlans = append(vlans, v)
		}
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", v.Name, v.exists, v.preExist, vlans)
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

func (b *bridge) CreateBridgeMirror() (string, error) {
	// get a host tap
	tapName, err := hostTapCreate(b.Name, "0", "none", "")
	if err != nil {
		return "", err
	}

	// create the mirror for this bridge
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
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
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("creating bridge mirror with cmd: %v", cmd)
	ovsLock.Lock()
	err = cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return "", e
	}
	return tapName, nil
}

func (b *bridge) DeleteBridgeMirror(tap string) error {
	// delete the mirror for this bridge
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"clear",
			"bridge",
			b.Name,
			"mirrors",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("deleting bridge mirror with cmd: %v", cmd)
	ovsLock.Lock()
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}

	// delete the associated host tap
	err = hostTapDelete(tap)
	if err != nil {
		return err
	}

	return nil
}

// create a new vlan. If this is the first vlan being allocated, then the
// bridge will need to be created as well. this allows us to avoid using the
// bridge utils when we create vms with no network.
func (b *bridge) LanCreate(lan int) error {
	// start the ipmaclearner if need be
	err := b.startIML()
	if err != nil {
		return err
	}
	if b.lans[lan] != nil {
		return nil
	}
	b.Lock.Lock()
	b.lans[lan] = &vlan{
		Taps: make(map[string]*tap),
	}
	b.Lock.Unlock()
	return nil
}

// remove an active netflow object
func (b *bridge) DestroyNetflow() error {
	if b.nf == nil {
		return fmt.Errorf("bridge %v has no netflow object", b.Name)
	}

	b.nf.Stop()

	// connect openvswitch to our new netflow object
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"clear",
			"Bridge",
			b.Name,
			"netflow",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("removing netflow on bridge with cmd: %v", cmd)
	ovsLock.Lock()
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}

	b.Lock.Lock()
	b.nf = nil
	b.Lock.Unlock()

	return nil
}

// update the active timeout on a nf object
func (b *bridge) UpdateNFTimeout(t int) error {
	if b.nf == nil {
		return fmt.Errorf("bridge %v has no netflow object", b.Name)
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"set",
			"NetFlow",
			b.Name,
			fmt.Sprintf("active_timeout=%v", t),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("updating netflow active_timeout with cmd: %v", cmd)
	ovsLock.Lock()
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}

	return nil
}

// create a new netflow object for the specified bridge
func (b *bridge) NewNetflow(timeout int) (*gonetflow.Netflow, error) {
	nf, port, err := gonetflow.NewNetflow()
	if err != nil {
		return nil, err
	}

	// connect openvswitch to our new netflow object
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
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
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("creating netflow to bridge with cmd: %v", cmd)
	ovsLock.Lock()
	err = cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("NewNetflow: could not enable netflow: %v: %v", err, sErr.String())
		return nil, e
	}

	b.Lock.Lock()
	b.nf = nf
	b.Lock.Unlock()
	return nf, nil
}

func (b *bridge) startIML() error {
	if b.iml != nil {
		return nil
	}

	// use openflow to redirect arp and icmp6 traffic to the local tap
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("openflow")

	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"add-flow",
			b.Name,
			"dl_type=0x0806,actions=local,normal",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("adding arp flow with cmd: %v", cmd)
	ovsLock.Lock()
	err := cmd.Run()
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openflow: %v: %v", err, sErr.String())
		return e
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"add-flow",
			b.Name,
			"dl_type=0x86dd,nw_proto=58,icmp_type=135,actions=local,normal",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("adding icmp6 ND flow with cmd: %v", cmd)
	ovsLock.Lock()
	err = cmd.Run()
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openflow: %v: %v", err, sErr.String())
		return e
	}

	iml, err := ipmac.NewLearner(b.Name)
	if err != nil {
		return fmt.Errorf("cannot start ip learner on bridge: %v", err)
	}
	b.Lock.Lock()
	b.iml = iml
	b.Lock.Unlock()
	return nil
}

// create the bridge with ovs
func (b *bridge) create() error {
	var sOut bytes.Buffer
	var sErr bytes.Buffer

	if !b.exists {
		log.Debugln("bridge does not exist")
		p := process("ovs")
		cmd := &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				"add-br",
				b.Name,
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Debug("creating bridge with cmd: %v", cmd)
		ovsLock.Lock()
		err := cmdTimeout(cmd, OVS_TIMEOUT)
		ovsLock.Unlock()
		if err != nil {
			es := sErr.String()
			if strings.Contains(es, "already exists") {
				b.preExist = true
			} else {
				e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
				return e
			}
		}
		b.Lock.Lock()
		b.exists = true
		b.lans = make(map[int]*vlan)
		b.Lock.Unlock()

		p = process("ip")
		cmd = &exec.Cmd{
			Path: p,
			Args: []string{
				p,
				"link",
				"set",
				b.Name,
				"up",
			},
			Env:    nil,
			Dir:    "",
			Stdout: &sOut,
			Stderr: &sErr,
		}
		log.Debug("bringing bridge up with cmd: %v", cmd)
		err = cmd.Run()
		if err != nil {
			e := fmt.Errorf("ip: %v: %v", err, sErr.String())
			return e
		}
	}

	return nil
}

// destroy a bridge with ovs, and remove all of the taps, etc associated with it
func (b *bridge) Destroy() error {
	// first get all of the taps off of this bridge and destroy them
	for name, lan := range b.lans {
		log.Debug("destroying lan %v", name)
		for tapName, t := range lan.Taps {
			if t != nil {
				err := b.TapDestroy(name, tapName)
				if err != nil {
					log.Info("Destroy: could not destroy tap: %v", err)
				}
			}
		}
	}

	// don't destroy the bridge if it existed before we started
	if !b.exists || b.preExist {
		return nil
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			b.Name,
			"down",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("bringing bridge down with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("ip: %v: %v", err, sErr.String())
		return e
	}

	p = process("ovs")
	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"del-br",
			b.Name,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("destroying bridge with cmd: %v", cmd)
	ovsLock.Lock()
	err = cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}
	return nil
}

// create and add a tap to a bridge
func (b *bridge) TapCreate(lan int) (string, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	tapName, err := getNewTap()
	if err != nil {
		return "", err
	}
	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"tuntap",
			"add",
			"mode",
			"tap",
			tapName,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("creating tap with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("ip: %v: %v", err, sErr.String())
		return "", e
	}

	// the tap add was successful, so try to add it to the bridge
	b.Lock.Lock()
	b.lans[lan].Taps[tapName] = &tap{
		host: false,
	}
	b.Lock.Unlock()
	err = b.TapAdd(lan, tapName, false)
	if err != nil {
		return "", err
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tapName,
			"up",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("bringing tap up with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("ip: %v: %v", err, sErr.String())
		return "", e
	}
	return tapName, nil
}

// destroy and remove a tap from a bridge
func (b *bridge) TapDestroy(lan int, tap string) error {
	err := b.TapRemove(lan, tap)
	if err != nil {
		log.Info("TapDestroy: could not remove tap: %v", err)
	}

	// if it's a host tap, then ovs removed it for us and we don't need to continue
	bridgeLock.Lock()
	if _, ok := disconnectedTaps[tap]; !ok {
		bridgeLock.Unlock()
		return nil
	}
	bridgeLock.Unlock()

	var sOut bytes.Buffer
	var sErr bytes.Buffer

	p := process("ip")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tap,
			"down",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("bringing tap down with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("ip: %v: %v", err, sErr.String())
		return e
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"tuntap",
			"del",
			"mode",
			"tap",
			tap,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("destroying tap with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("ip: %v: %v", err, sErr.String())
		return e
	}
	return nil
}

// add a tap to the bridge
func (b *bridge) TapAdd(lan int, tap string, host bool) error {
	err := b.LanCreate(lan)
	if err != nil {
		return err
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"add-port",
			b.Name,
			tap,
			fmt.Sprintf("tag=%v", lan),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	if host {
		cmd.Args = append(cmd.Args, "--")
		cmd.Args = append(cmd.Args, "set")
		cmd.Args = append(cmd.Args, "Interface")
		cmd.Args = append(cmd.Args, tap)
		cmd.Args = append(cmd.Args, "type=internal")
	}

	log.Debug("adding tap with cmd: %v", cmd)
	ovsLock.Lock()
	err = cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		if strings.Contains(sErr.String(), "already exists") {
			// special case - we own the tap, but it already exists
			// on the bridge. simply remove and add it again
			log.Info("tap %v is already on bridge, readding", tap)
			err = b.TapRemove(lan, tap)
			if err != nil {
				return err
			}
			return b.TapAdd(lan, tap, host)
		} else {
			e := fmt.Errorf("TapAdd: %v: %v", err, sErr.String())
			return e
		}
	}

	// if this tap is in the disconnected list, move it out
	bridgeLock.Lock()
	b.Lock.Lock()
	if _, ok := disconnectedTaps[tap]; ok {
		b.lans[lan].Taps[tap] = disconnectedTaps[tap]
		delete(disconnectedTaps, tap)
	}
	b.Lock.Unlock()
	bridgeLock.Unlock()

	return nil
}

// remove a tap from a bridge
func (b *bridge) TapRemove(lan int, tap string) error {
	// put this tap into the disconnected vlan
	bridgeLock.Lock()
	b.Lock.Lock()
	if !b.lans[lan].Taps[tap].host { // don't move host taps, just delete them
		disconnectedTaps[tap] = b.lans[lan].Taps[tap]
	}
	delete(b.lans[lan].Taps, tap)
	b.Lock.Unlock()
	bridgeLock.Unlock()

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"del-port",
			b.Name,
			tap,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("removing tap with cmd: %v", cmd)
	ovsLock.Lock()
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	ovsLock.Unlock()
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}
	return nil
}

func hostTapList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "vlan", "option"}
	resp.Tabular = [][]string{}

	// find all the host taps first
	for k, v := range bridges {
		for lan, t := range v.lans {
			for tap, ti := range t.Taps {
				if ti.host {
					resp.Tabular = append(resp.Tabular, []string{
						k, tap, strconv.Itoa(lan), ti.hostOption,
					})
				}
			}
		}
	}
}

func hostTapDelete(tap string) error {
	var c []*bridge
	// special case, *, which should delete all host taps from all bridges
	if tap == "*" {
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
		for lan, t := range b.lans {
			if tap == "-1" {
				// remove all host taps on this vlan
				for k, v := range t.Taps {
					if v.host {
						b.TapRemove(lan, k)
					}
				}
				continue
			}
			if tf, ok := t.Taps[tap]; ok {
				if !tf.host {
					return fmt.Errorf("not a host tap")
				}
				b.TapRemove(lan, tap)
			}
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
	err = b.LanCreate(r)
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
	b.Lock.Lock()
	b.lans[r].Taps[tapName] = &tap{
		host:       true,
		hostOption: ip,
	}
	b.Lock.Unlock()
	err = b.TapAdd(r, tapName, true)
	if err != nil {
		return "", err
	}

	// bring the tap up
	p := process("ip")
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tapName,
			"up",
			"promisc",
			"on",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Debug("bringing up host tap %v", tapName)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return "", e
	}

	if strings.ToLower(ip) == "none" {
		return tapName, nil
	}

	if strings.ToLower(ip) == "dhcp" {
		p = process("dhcp")
		cmd = &exec.Cmd{
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
		err = cmd.Run()
		if err != nil {
			e := fmt.Errorf("%v: %v", err, sErr.String())
			return "", e
		}
	} else {
		cmd = &exec.Cmd{
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
		err = cmd.Run()
		if err != nil {
			e := fmt.Errorf("%v: %v", err, sErr.String())
			return "", e
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

func GetIPFromMac(mac string) *ipmac.IP {
	log.Debugln("GetIPFromMac")
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	for k, v := range bridges {
		if v.iml != nil {
			ip := v.iml.GetIPFromMac(mac)
			if ip != nil {
				log.Debug("found mac %v in bridge %v", mac, k)
				return ip
			}
		}
	}
	return nil
}
