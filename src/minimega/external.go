// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"minicli"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
)

const (
	MIN_QEMU = 1.6
	MIN_OVS  = 1.4
)

var externalProcesses = map[string]string{
	"qemu":     "kvm",
	"ip":       "ip",
	"ovs":      "ovs-vsctl",
	"dnsmasq":  "dnsmasq",
	"kill":     "kill",
	"dhcp":     "dhclient",
	"openflow": "ovs-ofctl",
	"mount":    "mount",
	"umount":   "umount",
	"mkdosfs":  "mkdosfs",
	"qemu-nbd": "qemu-nbd",
	"rm":       "rm",
	"qemu-img": "qemu-img",
	"cp":       "cp",
	"taskset":  "taskset",
	"lsmod":    "lsmod",
	"ntfs-3g":  "ntfs-3g",
}

var externalCLIHandlers = []minicli.Handler{
	{ // check
		HelpShort: "check that all external executables dependencies exist",
		HelpLong: `
Minimega maintains a list of external packages that it depends on, such as
qemu. Calling check will attempt to find each of these executables in the
avaiable path, and returns an error on the first one not found.`,
		Patterns: []string{
			"check",
		},
		Record: true,
		Call:   cliCheckExternal,
	},
}

func init() {
	registerHandlers("external", externalCLIHandlers)
}

// checkExternal checks for the presence of each of the external processes we
// may call, and error if any aren't in our path.
func checkExternal() error {
	for _, i := range externalProcesses {
		path, err := exec.LookPath(i)
		if err != nil {
			return fmt.Errorf("%v not found", i)
		} else {
			log.Info("%v found at: %v", i, path)
		}
	}

	// everything we want exists, but we have a few minimum versions to check
	version, err := qemuVersion()
	if err != nil {
		return err
	}

	log.Debug("got kvm version %v", version)
	if version < MIN_QEMU {
		return fmt.Errorf("kvm version %v does not meet minimum version %v", version, MIN_QEMU)
	}

	version, err = ovsVersion()
	if err != nil {
		return err
	}

	log.Debug("got ovs version %v", version)
	if version < MIN_OVS {
		return fmt.Errorf("ovs version %v does not meet minimum version %v", version, MIN_OVS)
	}

	return nil
}

func cliCheckExternal(c *minicli.Command) minicli.Responses {
	resp := &minicli.Response{Host: hostname}

	err := checkExternal()
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Response = "all external dependencies met"
	}

	return minicli.Responses{resp}
}

func process(p string) string {
	path, err := exec.LookPath(externalProcesses[p])
	if err != nil {
		log.Error("process: %v", err)
		return ""
	}
	return path
}

func qemuVersion() (float64, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("qemu")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-version",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	log.Debug("checking qemu version with cmd: %v", cmd)
	if err := cmd.Run(); err != nil {
		return 0.0, fmt.Errorf("error checking kvm version: %v %v", err, sErr.String())
	}

	f := strings.Fields(sOut.String())
	if len(f) < 4 {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v", sOut.String())
	}

	qemuVersionFields := strings.Split(f[3], ".")
	if len(qemuVersionFields) < 2 {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v", sOut.String())
	}

	log.Debugln(qemuVersionFields)
	qemuVersion, err := strconv.ParseFloat(strings.Join(qemuVersionFields[:2], "."), 64)
	if err != nil {
		return 0.0, fmt.Errorf("cannot parse kvm version: %v %v", sOut.String(), err)
	}

	return qemuVersion, nil
}

func ovsVersion() (float64, error) {
	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p := process("ovs")
	cmd := &exec.Cmd{
		Path: p,
		Args: []string{
			p,
			"-V",
		},
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}

	log.Debug("checking ovs version with cmd: %v", cmd)
	if err := cmd.Run(); err != nil {
		return 0.0, fmt.Errorf("checking ovs version: %v %v", err, sErr.String())
	}

	f := strings.Fields(sOut.String())
	if len(f) < 4 {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v", sOut.String())
	}

	ovsVersionFields := strings.Split(f[3], ".")
	if len(ovsVersionFields) < 2 {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v", sOut.String())
	}

	log.Debugln(ovsVersionFields)
	ovsVersion, err := strconv.ParseFloat(strings.Join(ovsVersionFields[:2], "."), 64)
	if err != nil {
		return 0.0, fmt.Errorf("cannot parse ovs version: %v %v", sOut.String(), err)
	}

	return ovsVersion, nil
}
