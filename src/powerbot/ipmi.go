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
	username     string
	password     string
	lanInterface string
}

// Create a new instance. The port should point to the device's telnet
// CLI port, which appears to usually be 5214.
func NewIPMI(username, password, lanInterface string) (PDU) {
	var ipmi IPMI
	ipmi.username = username
	ipmi.password = password
	ipmi.lanInterface = lanInterface
	return ipmi
}


// Log in, say "loadctl on -o <port> --force" for every port
// the user specified, and log out.
func (i IPMI) On(addMap map[string]string) error {
	for d, n := range addMap {
		opts := append(i.args(), "-H", n, "chassis power on")
		out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
		if err != nil {
			return err
		}
		fmt.Printf("%s ", d)
		fmt.Println(out)
	}
	return nil
}

// Log in, say "loadctl off -o <port> --force" for every port
// the user specified, and log out.
func (i IPMI) Off(addMap map[string]string) error {
	for d, n := range addMap {
		opts := append(i.args(), "-H", n, "chassis power off")
		out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
		if err != nil {
			return err
		}
		fmt.Printf("%s ", d)
		fmt.Println(out)
	}
	return nil
}

// Log in, say "loadctl cycle -o <port> --force" for every port
// the user specified, and log out.
func (i IPMI) Cycle(addMap map[string]string) error {
	for d, n := range addMap {
		opts := append(i.args(), "-H", n, "chassis power cycle")
		out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
		if err != nil {
			return err
		}
		fmt.Printf("%s ", d)
		fmt.Println(out)
	}
	return nil
}

// This isn't done yet because I don't want to parse that crap yet.
// Should be pretty easy really, call "loadctl status -o" and then
// look for the lines corresponding to the listed nodes. Print only
// those lines.
func (i IPMI) Status(addMap map[string]string) error {
	for d, n := range addMap {
		opts := append(i.args(), "-H", n, "chassis power status")
		out, err := run("/tmp/ipmi/ipmitool.bash", opts...)
		if err != nil {
			return err
		}
		fmt.Printf("%s ", d)
		fmt.Println(out)
	}
	return nil
}

func (i IPMI) args() []string {
	options := []string{
		"-I", i.lanInterface,
		"-U", i.username,
		"-P", i.password,
	}
	return options
}

// ExecutablePath, Interface, Host, Username, Password, Command
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

