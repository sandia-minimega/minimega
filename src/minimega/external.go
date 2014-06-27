// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"os/exec"
	"strconv"
	"strings"
)

const (
	MIN_QEMU = 1.1
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
}

// check for the presence of each of the external processes we may call,
// and error if any aren't in our path
func externalCheck(c cliCommand) cliResponse {
	if len(c.Args) != 0 {
		return cliResponse{
			Error: "check does not take any arguments",
		}
	}
	for _, i := range externalProcesses {
		path, err := exec.LookPath(i)
		if err != nil {
			e := fmt.Sprintf("%v not found", i)
			return cliResponse{
				Error: e,
			}
		} else {
			log.Info("%v found at: %v", i, path)
		}
	}

	// everything we want exists, but we have a few minimum versions to check
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
	err := cmd.Run()
	if err != nil {
		return cliResponse{
			Error: fmt.Sprintf("checking kvm version: %v %v", err, sErr.String()),
		}
	}
	f := strings.Fields(sOut.String())
	if len(f) < 4 {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse kvm version: %v", sOut.String()),
		}
	}
	qemuVersionFields := strings.Split(f[3], ".")
	if len(qemuVersionFields) < 2 {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse kvm version: %v", sOut.String()),
		}
	}
	log.Debugln(qemuVersionFields)
	qemuVersion, err := strconv.ParseFloat(strings.Join(qemuVersionFields[:2], "."), 64)
	if err != nil {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse kvm version: %v %v", sOut.String(), err),
		}
	}
	log.Debug("got kvm version %v", qemuVersion)
	if qemuVersion < MIN_QEMU {
		return cliResponse{
			Error: fmt.Sprintf("kvm version %v does not meet minimum version %v", qemuVersion, MIN_QEMU),
		}
	}

	sErr.Reset()
	sOut.Reset()
	p = process("ovs")
	cmd = &exec.Cmd{
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
	err = cmd.Run()
	if err != nil {
		return cliResponse{
			Error: fmt.Sprintf("checking ovs version: %v %v", err, sErr.String()),
		}
	}
	f = strings.Fields(sOut.String())
	if len(f) < 4 {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse ovs version: %v", sOut.String()),
		}
	}
	ovsVersionFields := strings.Split(f[3], ".")
	if len(ovsVersionFields) < 2 {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse ovs version: %v", sOut.String()),
		}
	}
	log.Debugln(ovsVersionFields)
	ovsVersion, err := strconv.ParseFloat(strings.Join(ovsVersionFields[:2], "."), 64)
	if err != nil {
		return cliResponse{
			Error: fmt.Sprintf("cannot parse ovs version: %v %v", sOut.String(), err),
		}
	}
	log.Debug("got ovs version %v", ovsVersion)
	if ovsVersion < MIN_OVS {
		return cliResponse{
			Error: fmt.Sprintf("ovs version %v does not meet minimum version %v", ovsVersion, MIN_OVS),
		}
	}

	return cliResponse{}
}

func process(p string) string {
	path, err := exec.LookPath(externalProcesses[p])
	if err != nil {
		log.Error("process: %v", err)
		return ""
	}
	return path
}
