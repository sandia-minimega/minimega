// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	log "minilog"
	"os/exec"
)

// Wrapper for ipmitool.
type IPMI struct {
	ip       string
	password string
	path     string
	username string
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewIPMI(ip, password, path, username string) PDU {
	var ipmi IPMI
	ipmi.path = path
	ipmi.ip = ip
	ipmi.username = username
	ipmi.password = password
	return ipmi
}

func (i IPMI) On(addMap map[string]string) error {
	return i.sendCommand("chassis power on")
}

func (i IPMI) Off(addMap map[string]string) error {
	return i.sendCommand("chassis power off")
}

func (i IPMI) Cycle(addMap map[string]string) error {
	return i.sendCommand("chassis power cycle")
}

func (i IPMI) Status(addMap map[string]string) error {
	return i.sendCommand("chassis power status")
}

func (i IPMI) sendCommand(c string) error {
	opts := append(i.args(), c)
	out, err := run(i.path, opts...)
	log.Info(out)
	return err
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
