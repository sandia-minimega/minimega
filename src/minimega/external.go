// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// routines to check for the presence and path of all external processes.
// you should use process("my process") and register the process here if you
// plan on calling external processes.
package main

import (
	"errors"
	"fmt"
	log "minilog"
	"os/exec"
)

var external_processes = map[string]string{
	"qemu":    "qemu-system-x86_64",
	"ip":      "ip",
	"ovs":     "ovs-vsctl",
	"dnsmasq": "dnsmasq",
	"tunctl":  "tunctl",
	"browser": "x-www-browser",
	"kill":    "kill",
}

// check for the presence of each of the external processes we may call,
// and error if any aren't in our path
func external_check(c cli_command) cli_response {
	if len(c.Args) != 0 {
		return cli_response{
			Error: errors.New("check does not take any arguments"),
		}
	}
	for _, i := range external_processes {
		path, err := exec.LookPath(i)
		if err != nil {
			e := fmt.Sprintf("%v not found", i)
			return cli_response{
				Error: errors.New(e),
			}
		} else {
			log.Info("%v found at: %v", i, path)
		}
	}
	return cli_response{}
}

func process(p string) string {
	path, err := exec.LookPath(external_processes[p])
	if err != nil {
		log.Error("%v", err)
		return ""
	}
	return path
}
