// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
//

// minimega bridge utilies including tap creation and teardown
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
)

// a bridge representation that includes a list of vlans and their respective
// taps.
type bridge struct {
	Name     string
	lans     map[int]*vlan
	exists   bool // false until the first usage, then true until destroyed.
	preExist bool
}

type vlan struct {
	Id   int             // the actual id passed to openvswitch
	Taps map[string]bool // named list of taps. If false, the tap is destroyed.
}

var (
	current_bridge *bridge     // bridge for the current context, currently the *only* bridge
	tap_count      int         // total number of allocated taps on this host
	tap_chan       chan string // atomic feeder of tap names, wraps tap_count
)

// create the default bridge struct and create a goroutine to generate
// tap names for this host.
func init() {
	current_bridge = &bridge{
		Name: "mega_bridge",
	}
	tap_chan = make(chan string)
	go func() {
		for {
			tap_chan <- fmt.Sprintf("mega_tap%v", tap_count)
			tap_count++
			log.Info("tap_count: %v", tap_count)
		}
	}()
}

// create a new vlan. If this is the first vlan being allocated, then the 
// bridge will need to be created as well. this allows us to avoid using the
// bridge utils when we create vms with no network.
func (b *bridge) Lan_create(lan int) (error, bool) {
	if !b.exists {
		log.Info("bridge does not exist")
		err := b.create()
		if err != nil {
			return err, false
		}
		b.exists = true
		b.lans = make(map[int]*vlan)
	}
	if b.lans[lan] != nil {
		return errors.New("lan already exists"), true
	}
	b.lans[lan] = &vlan{
		Id:   lan, // vlans start at 1, because 0 is a special vlan
		Taps: make(map[string]bool),
	}
	return nil, true
}

// create the bridge with ovs
func (b *bridge) create() error {
	var s_out bytes.Buffer
	var s_err bytes.Buffer
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("creating bridge with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		es := s_err.String()
		if strings.Contains(es, "already exists") {
			b.preExist = true
			return nil
		}
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("bringing bridge up with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
	}

	return nil
}

// destroy a bridge with ovs, and remove all of the taps, etc associated with it
func (b *bridge) Destroy() error {
	if !b.exists || b.preExist {
		return nil
	}
	// first get all of the taps off of this bridge and destroy them
	for name, lan := range b.lans {
		log.Info("destroying lan %v", name)
		for tap, ok := range lan.Taps {
			if ok {
				err := b.Tap_destroy(name, tap)
				if err != nil {
					log.Error("%v", err)
				}
			}
		}
	}

	var s_out bytes.Buffer
	var s_err bytes.Buffer
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("bringing bridge down with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("destroying bridge with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
	}
	return nil
}

// create and add a tap to a bridge
func (b *bridge) Tap_create(lan int) (string, error) {
	var s_out bytes.Buffer
	var s_err bytes.Buffer
	tap, err := getNewTap()
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
			tap,
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("creating tap with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return "", e
	}

	// the tap add was successful, so try to add it to the bridge
	b.lans[lan].Taps[tap] = true
	err = b.tap_add(lan, tap, false)
	if err != nil {
		return "", err
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tap,
			"up",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("bringing tap up with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return "", e
	}
	return tap, nil
}

// destroy and remove a tap from a bridge
func (b *bridge) Tap_destroy(lan int, tap string) error {
	var s_out bytes.Buffer
	var s_err bytes.Buffer

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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("bringing tap down with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("destroying tap with cmd: %v", cmd)
	err = cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
	}
	b.lans[lan].Taps[tap] = false
	err = b.tap_remove(tap)
	if err != nil {
		return err
	}
	return nil
}

// add a tap to the bridge
func (b *bridge) tap_add(lan int, tap string, host bool) error {
	var s_out bytes.Buffer
	var s_err bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"add-port",
			b.Name,
			tap,
			fmt.Sprintf("tag=%v", b.lans[lan].Id),
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}

	if host {
		cmd.Args = append(cmd.Args, "--")
		cmd.Args = append(cmd.Args, "set")
		cmd.Args = append(cmd.Args, "Interface")
		cmd.Args = append(cmd.Args, tap)
		cmd.Args = append(cmd.Args, "type=internal")
	}

	log.Info("adding tap with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
	}
	return nil
}

// remove a tap from a bridge
func (b *bridge) tap_remove(tap string) error {
	var s_out bytes.Buffer
	var s_err bytes.Buffer
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
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("removing tap with cmd: %v", cmd)
	err := cmd.Run()
	if err != nil {
		e := fmt.Errorf("%v: %v", err, s_err.String())
		return e
	}
	return nil
}

// TODO: allow creating a host tap with dhcp address instead of hardcoded
// routines for interfacing bridge mechanisms with the cli
func host_tap_create(c cli_command) cli_response {
	if len(c.Args) != 2 {
		return cli_response{
			Error: "host_tap takes two arguments",
		}
	}
	r, err := strconv.Atoi(c.Args[0])
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}
	lan_err, ok := current_bridge.Lan_create(r)
	if !ok {
		return cli_response{
			Error: lan_err.Error(),
		}
	}

	tap, err := getNewTap()
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}

	// create the tap
	current_bridge.lans[r].Taps[tap] = true
	err = current_bridge.tap_add(r, tap, true)
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}

	// bring the tap up
	p := process("ip")
	var s_out bytes.Buffer
	var s_err bytes.Buffer
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"link",
			"set",
			tap,
			"up",
			"promisc",
			"on",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("bringing up host tap %v", tap)
	err = cmd.Run()
	if err != nil {
		e := fmt.Sprintf("%v: %v", err, s_err.String())
		return cli_response{
			Error: e,
		}
	}

	cmd = &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"addr",
			"add",
			"dev",
			tap,
			c.Args[1],
		},
		Env:    nil,
		Dir:    "",
		Stdout: &s_out,
		Stderr: &s_err,
	}
	log.Info("setting ip on tap %v", tap)
	err = cmd.Run()
	if err != nil {
		e := fmt.Sprintf("%v: %v", err, s_err.String())
		return cli_response{
			Error: e,
		}
	}

	return cli_response{
		Response: tap,
	}
}

// gets a new tap from tap_chan and verifies that it doesn't already exist
func getNewTap() (string, error) {
	var t string
	for {
		t = <-tap_chan
		taps, err := ioutil.ReadDir("/sys/class/net")
		if err != nil {
			return "", err
		}
		found := false
		for _, v := range taps {
			if v.Name() == t {
				found = true
				log.Info("tap %v already exists, trying again\n", t)
			}
		}
		if !found {
			break
		}
	}
	return t, nil
}
