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
	log "minilog"
	"os"
	"os/exec"
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
)

// create the default bridge struct and create a goroutine to generate
// tap names for this host.
func init() {
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
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	var e []string
	for k, v := range bridges {
		err := v.Destroy()
		if err != nil {
			e = append(e, err.Error())
		}
		delete(bridges, k)
	}
	updateBridgeInfo()
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

func updateBridgeInfo() {
	log.Debugln("updateBridgeInfo")
	i := bridgeInfo()
	path := *f_base + "bridges"
	err := ioutil.WriteFile(path, []byte(i), 0644)
	if err != nil {
		log.Fatalln(err)
	}
}

func cliBridgeInfo(c cliCommand) cliResponse {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	return cliResponse{
		Response: bridgeInfo(),
	}
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
	b.lans[lan] = &vlan{
		Taps: make(map[string]*tap),
	}
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
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}

	b.nf = nil

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
	err := cmdTimeout(cmd, OVS_TIMEOUT)
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
	err = cmdTimeout(cmd, OVS_TIMEOUT)
	if err != nil {
		e := fmt.Errorf("NewNetflow: could not enable netflow: %v: %v", err, sErr.String())
		return nil, e
	}

	b.nf = nf
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
	err := cmd.Run()
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
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("openflow: %v: %v", err, sErr.String())
		return e
	}

	iml, err := ipmac.NewLearner(b.Name)
	if err != nil {
		return fmt.Errorf("cannot start ip learner on bridge: %v", err)
	}
	b.iml = iml
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
		err := cmdTimeout(cmd, OVS_TIMEOUT)
		if err != nil {
			es := sErr.String()
			if strings.Contains(es, "already exists") {
				b.preExist = true
			} else {
				e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
				return e
			}
		}
		b.exists = true
		b.lans = make(map[int]*vlan)

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
	err = cmdTimeout(cmd, OVS_TIMEOUT)
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
	b.Lock.Lock()
	if _, ok := disconnectedTaps[tap]; !ok {
		b.Lock.Unlock()
		return nil
	}
	b.Lock.Unlock()

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
	err = cmdTimeout(cmd, OVS_TIMEOUT)
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
	if _, ok := disconnectedTaps[tap]; ok {
		b.lans[lan].Taps[tap] = disconnectedTaps[tap]
		delete(disconnectedTaps, tap)
	}

	return nil
}

// remove a tap from a bridge
func (b *bridge) TapRemove(lan int, tap string) error {
	// put this tap into the disconnected vlan
	b.Lock.Lock()
	if !b.lans[lan].Taps[tap].host { // don't move host taps, just delete them
		disconnectedTaps[tap] = b.lans[lan].Taps[tap]
	}
	delete(b.lans[lan].Taps, tap)
	b.Lock.Unlock()

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
	err := cmdTimeout(cmd, OVS_TIMEOUT)
	if err != nil {
		e := fmt.Errorf("openvswitch: %v: %v", err, sErr.String())
		return e
	}
	return nil
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		return hostTapList()
	case 2: // must be delete
		if c.Args[0] != "delete" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		return hostTapDelete(c.Args[1])
	case 3: // must be create with the default bridge
		if c.Args[0] != "create" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		return hostTapCreate(DEFAULT_BRIDGE, c.Args[1], c.Args[2], "")
	case 4: // must be create with a specified bridge or a specified tap name
		if c.Args[0] != "create" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		_, err := strconv.Atoi(c.Args[1])
		if err == nil {
			// specified tap name
			return hostTapCreate(DEFAULT_BRIDGE, c.Args[1], c.Args[2], c.Args[3])
		} else {
			// specified bridge name
			return hostTapCreate(c.Args[1], c.Args[2], c.Args[3], "")
		}
	case 5: // must be create with a specified bridge AND tap name
		if c.Args[0] != "create" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		return hostTapCreate(c.Args[1], c.Args[2], c.Args[3], c.Args[4])
	}

	return cliResponse{
		Error: "malformed command",
	}
}

func hostTapList() cliResponse {
	var hostBridge []string
	var lans []int
	var taps []string
	var options []string

	// find all the host taps first
	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	for k, v := range bridges {
		for lan, t := range v.lans {
			for tap, ti := range t.Taps {
				if ti.host {
					hostBridge = append(hostBridge, k)
					lans = append(lans, lan)
					taps = append(taps, tap)
					options = append(options, ti.hostOption)
				}
			}
		}
	}

	if len(lans) == 0 {
		return cliResponse{}
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "bridge\ttap\tvlan\toption\n")
	for i, _ := range lans {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\n", hostBridge[i], taps[i], lans[i], options[i])
	}

	w.Flush()

	return cliResponse{
		Response: o.String(),
	}
}

func hostTapDelete(tap string) cliResponse {
	var c []*bridge
	// special case, -1, which should delete all host taps from all bridges
	if tap == "-1" {
		bridgeLock.Lock()
		for _, v := range bridges {
			c = append(c, v)
		}
		bridgeLock.Unlock()
	} else {
		b, err := getBridgeFromTap(tap)
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
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
					return cliResponse{
						Error: "not a host tap",
					}
				}
				b.TapRemove(lan, tap)
			}
		}
	}
	return cliResponse{}
}

func hostTapCreate(bridge, lan, ip, tapName string) cliResponse {
	b, err := getBridge(bridge)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	r, err := strconv.Atoi(lan)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	lanErr := b.LanCreate(r)
	if lanErr != nil {
		return cliResponse{
			Error: lanErr.Error(),
		}
	}

	if tapName == "" {
		tapName, err = getNewTap()
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
	}

	// create the tap
	b.lans[r].Taps[tapName] = &tap{
		host:       true,
		hostOption: ip,
	}
	err = b.TapAdd(r, tapName, true)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
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
		e := fmt.Sprintf("%v: %v", err, sErr.String())
		return cliResponse{
			Error: e,
		}
	}

	if strings.ToLower(ip) == "none" {
		return cliResponse{
			Response: tapName,
		}
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
			e := fmt.Sprintf("%v: %v", err, sErr.String())
			return cliResponse{
				Error: e,
			}
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
			e := fmt.Sprintf("%v: %v", err, sErr.String())
			return cliResponse{
				Error: e,
			}
		}
	}

	return cliResponse{
		Response: tapName,
	}
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
