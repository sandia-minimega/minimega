// Copyright (2016) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"strconv"
)

var routerCLIHandlers = []minicli.Handler{
	{ // router
		HelpShort: "",
		HelpLong:  ``,
		Patterns: []string{
			"router",
			"router <vm>",
			"router <vm> <commit,>",
			"router <vm> <interface,> <add,> <network> <IPv4/MASK or IPv6/MASK or dhcp>",
			"router <vm> <interface,> <del,> <network> <IPv4/MASK or IPv6/MASK or dhcp>",
		},
		Call: wrapBroadcastCLI(cliRouter),
	},
	// TODO: clear?
}

func cliRouter(c *minicli.Command, resp *minicli.Response) error {
	vmName := c.StringArgs["vm"]

	vm := vms.FindVM(vmName)
	if vm == nil {
		return fmt.Errorf("no such vm %v", vmName)
	}

	if c.BoolArgs["commit"] {
		err := RouterCommit(vm)
		if err != nil {
			return err
		}
	} else if c.BoolArgs["interface"] {
		network, err := strconv.Atoi(c.StringArgs["network"])
		if err != nil {
			return fmt.Errorf("invalid network: %v : %v", c.StringArgs["network"], err)
		}
		ip := c.StringArgs["IPv4/MASK"]

		if c.BoolArgs["add"] {
			err := RouterInterfaceAdd(vm, network, ip)
			if err != nil {
				return err
			}
		} else {
			err := RouterInterfaceDel(vm, network, ip)
			if err != nil {
				return err
			}
		}
	} else if vmName != "" { // a summary of a specific router
		r := FindRouter(vm)
		if r == nil {
			return fmt.Errorf("vm %v not a router", vmName)
		}
		resp.Response = r.String()
	} else { // a summary of all routers
		resp.Response = "implement me"
	}

	return nil
}
