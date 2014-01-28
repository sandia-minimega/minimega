// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"ipmac"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
)

// a bridge representation that includes a list of vlans and their respective
// taps.
type bridge struct {
	Name     string
	lans     map[int]*vlan
	exists   bool // false until the first usage, then true until destroyed.
	preExist bool
	iml      *ipmac.IPMacLearner
}

type vlan struct {
	Taps map[string]*tap // named list of taps.
}

type tap struct {
	host       bool
	hostOption string
}

var (
	currentBridge *bridge     // bridge for the current context, currently the *only* bridge
	tapCount      int         // total number of allocated taps on this host
	tapChan       chan string // atomic feeder of tap names, wraps tapCount
)

// create the default bridge struct and create a goroutine to generate
// tap names for this host.
func init() {
	currentBridge = &bridge{
		Name: "mega_bridge",
	}
	tapChan = make(chan string)
	go func() {
		for {
			tapChan <- fmt.Sprintf("mega_tap%v", tapCount)
			tapCount++
			log.Debug("tapCount: %v", tapCount)
		}
	}()
}

func cliBridgeInfo(c cliCommand) cliResponse {
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "Bridge name:\t%v\n", currentBridge.Name)
	fmt.Fprintf(w, "Exists:\t%v\n", currentBridge.exists)
	fmt.Fprintf(w, "Existed before minimega:\t%v\n", currentBridge.preExist)

	var vlans []int
	for v, _ := range currentBridge.lans {
		vlans = append(vlans, v)
	}

	fmt.Fprintf(w, "Active vlans:\t%v\n", vlans)

	w.Flush()

	return cliResponse{
		Response: o.String(),
	}
}

// create a new vlan. If this is the first vlan being allocated, then the
// bridge will need to be created as well. this allows us to avoid using the
// bridge utils when we create vms with no network.
func (b *bridge) LanCreate(lan int) error {
	err := b.create()
	if err != nil {
		return err
	}
	// start the ipmaclearner if need be
	err = b.startIML()
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
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
		err := cmd.Run()
		if err != nil {
			es := sErr.String()
			if strings.Contains(es, "already exists") {
				b.preExist = true
			} else {
				e := fmt.Errorf("%v: %v", err, sErr.String())
				return e
			}
		}
		b.exists = true
		b.lans = make(map[int]*vlan)
		// special vlan -1 is for disconnected taps
		b.lans[-1] = &vlan{
			Taps: make(map[string]*tap),
		}

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
			e := fmt.Errorf("%v: %v", err, sErr.String())
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
					log.Infoln(err)
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
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
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return "", e
	}

	// the tap add was successful, so try to add it to the bridge
	b.lans[lan].Taps[tapName] = &tap{
		host: false,
	}
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return "", e
	}
	return tapName, nil
}

// destroy and remove a tap from a bridge
func (b *bridge) TapDestroy(lan int, tap string) error {
	err := b.TapRemove(lan, tap)
	if err != nil {
		log.Infoln(err)
	}

	// if it's a host tap, then ovs removed it for us and we don't need to continue
	if v, ok := b.lans[-1]; ok {
		if w, ok := v.Taps[tap]; ok {
			if w.host {
				return nil
			}
		} else {
			return nil
		}
	} else {
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
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
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}
	return nil
}

// add a tap to the bridge
func (b *bridge) TapAdd(lan int, tap string, host bool) error {
	// if this tap is in the disconnected list, move it out
	if _, ok := b.lans[-1].Taps[tap]; ok {
		if _, ok := b.lans[lan]; !ok {
			err := currentBridge.LanCreate(lan)
			if err != nil {
				return err
			}
		}
		b.lans[lan].Taps[tap] = b.lans[-1].Taps[tap]
		delete(b.lans[-1].Taps, tap)
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
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}
	return nil
}

// remove a tap from a bridge
func (b *bridge) TapRemove(lan int, tap string) error {
	// put this tap into the disconnected vlan
	if lan != -1 {
		if !b.lans[lan].Taps[tap].host { // don't move host taps, just delete them
			b.lans[-1].Taps[tap] = b.lans[lan].Taps[tap]
		}
		delete(b.lans[lan].Taps, tap)
	}

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
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, sErr.String())
		return e
	}
	return nil
}

// routines for interfacing bridge mechanisms with the cli

func hostTap(c cliCommand) cliResponse {
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
	case 3: // must be create
		if c.Args[0] != "create" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		return hostTapCreate(c.Args[1], c.Args[2])
	}
	return cliResponse{
		Error: "malformed command",
	}
}

func hostTapList() cliResponse {
	var lans []int
	var taps []string
	var options []string

	// find all the host taps first
	for lan, t := range currentBridge.lans {
		for tap, ti := range t.Taps {
			if ti.host {
				lans = append(lans, lan)
				taps = append(taps, tap)
				options = append(options, ti.hostOption)
			}
		}
	}

	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(w, "tap\tvlan\toption\n")
	for i, _ := range lans {
		fmt.Fprintf(w, "%v\t%v\t%v\n", taps[i], lans[i], options[i])
	}

	w.Flush()

	return cliResponse{
		Response: o.String(),
	}
}

func hostTapDelete(tap string) cliResponse {
	for lan, t := range currentBridge.lans {
		if tap == "-1" {
			// remove all host taps on this vlan
			for k, v := range t.Taps {
				if v.host {
					currentBridge.TapRemove(lan, k)
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
			currentBridge.TapRemove(lan, tap)
		}
	}
	return cliResponse{}
}

func hostTapCreate(lan string, ip string) cliResponse {
	r, err := strconv.Atoi(lan)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	lanErr := currentBridge.LanCreate(r)
	if lanErr != nil {
		return cliResponse{
			Error: lanErr.Error(),
		}
	}

	tapName, err := getNewTap()
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	// create the tap
	currentBridge.lans[r].Taps[tapName] = &tap{
		host:       true,
		hostOption: ip,
	}
	err = currentBridge.TapAdd(r, tapName, true)
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
