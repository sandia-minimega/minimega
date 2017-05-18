// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"os/exec"
)

// Wrapper for ipmitool.
type IPMI struct {
	ip	 string
	username string
	password string
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewIPMI(ip, username, password string) (PDU) {
	var ipmi IPMI
	ipmi.ip = ip
	ipmi.username = username
	ipmi.password = password
	return ipmi
}


func (i IPMI) On(addMap map[string]string) error {
	opts := append(i.args(), "chassis power on")
	out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
	fmt.Println(out)
	return err
}

func (i IPMI) Off(addMap map[string]string) error {
	opts := append(i.args(), "chassis power off")
	out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
	fmt.Printf(out)
	return err
}

func (i IPMI) Cycle(addMap map[string]string) error {
	opts := append(i.args(), "chassis power cycle")
	out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
	fmt.Printf(out)
	return err
}

func (i IPMI) Status(addMap map[string]string) error {
	opts := append(i.args(), "chassis power status")
	out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
	fmt.Printf(out)
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
	if err != nil {
		return out.String(), err
	}

	return out.String(), err
}

