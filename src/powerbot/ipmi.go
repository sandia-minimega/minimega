// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"os/exec"
)

// Wrapper for ipmitool.
type IPMI struct {
	ip       string
	node     string
	password string
	path     string
	username string
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewIPMI(ip, node, password, path, username string) PDU {
	var ipmi IPMI
	ipmi.node = node
	ipmi.path = path
	ipmi.ip = ip
	ipmi.username = username
	ipmi.password = password
	return ipmi
}

func (i IPMI) On(addMap map[string]string) error {
	out, err := i.sendCommand("chassis power on")
	log.Info("%v: %v", i.node, out)
	return err
}

func (i IPMI) Off(addMap map[string]string) error {
	out, err := i.sendCommand("chassis power off")
	log.Info("%v: %v", i.node, out)
	return err
}

func (i IPMI) Cycle(addMap map[string]string) error {
	out, err := i.sendCommand("chassis power cycle")
	log.Info("%v: %v", i.node, out)
	return err
}

func (i IPMI) Status(addMap map[string]string) error {
	out, err := i.sendCommand("chassis power status")
	if err == nil {
		fmt.Printf("%v: %v", i.node, out)
	}
	return err
}

func (i IPMI) Temp() error {
	out, err := i.sendCommand("sdr type Temperature")
	fmt.Println(i.node, ":")
	fmt.Println(out)
	return err
}

func (i IPMI) Info() error {
	fmt.Println(i.node, ":")
	out, err := i.sendCommand("sdr elist full")
	fmt.Println(out)
	return err
}

func (i IPMI) sendCommand(c string) (string, error) {
	opts := append(i.args(), c)
	return run(i.path, opts...)
}

func (i IPMI) args() []string {
	options := []string{
		"-I", "lanplus",
		"-H", i.ip,
		"-U", i.username,
		"-P", i.password,
	}
	return options
}

func run(path string, opts ...string) (string, error) {
	cmd := exec.Command(path, opts...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.String(), err
}
