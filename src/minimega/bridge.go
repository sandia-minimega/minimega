// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
//

package main

import (
	"bytes"
	"errors"
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
	Taps map[string]*tap // named list of taps. If !tap.active, then the tap is destroyed
}

type tap struct {
	active bool
	host   bool
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
func (b *bridge) LanCreate(lan int) (error, bool) {
	err := b.create()
	if err != nil {
		return err, false
	}
	// start the ipmaclearner if need be
	err = b.startIML()
	if err != nil {
		return err, false
	}
	if b.lans[lan] != nil {
		return errors.New("lan already exists"), true
	}
	b.lans[lan] = &vlan{
		Taps: make(map[string]*tap),
	}
	return nil, true
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
					log.Warnln(err)
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
		active: true,
		host:   false,
	}
	err = b.tapAdd(lan, tapName, false)
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
	b.lans[lan].Taps[tap].active = false
	err := b.tapRemove(tap)
	if err != nil {
		return err
	}

	// if it's a host tap, then ovs removed it for us and we don't need to continue
	if b.lans[lan].Taps[tap].host {
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
func (b *bridge) tapAdd(lan int, tap string, host bool) error {
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
func (b *bridge) tapRemove(tap string) error {
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
func hostTapCreate(c cliCommand) cliResponse {
	if len(c.Args) != 2 {
		return cliResponse{
			Error: "host_tap takes two arguments",
		}
	}
	r, err := strconv.Atoi(c.Args[0])
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	lanErr, ok := currentBridge.LanCreate(r)
	if !ok {
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
		active: true,
		host:   true,
	}
	err = currentBridge.tapAdd(r, tapName, true)
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

	if strings.ToLower(c.Args[1]) == "none" {
		return cliResponse{
			Response: tapName,
		}
	}

	if strings.ToLower(c.Args[1]) == "dhcp" {
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
				c.Args[1],
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
