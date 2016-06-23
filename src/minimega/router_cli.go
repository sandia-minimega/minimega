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
			"router <vm> <log,> <level,> <fatal,error,warn,info,debug>",
			"router <vm> <interface,> <network> <IPv4/MASK or IPv6/MASK or dhcp>",
			//			"router <vm> <dhcp,> <listen address> <range,> <low address> <high address>",
			//			"router <vm> <dhcp,> <listen address> <router,> <router address>",
			//			"router <vm> <dhcp,> <listen address> <dns server,> <dns address>",
			//			"router <vm> <dhcp,> <listen address> <static,> <mac> <ip>",
			//			"router <vm> <dns,> <ip> <hostname>",
			//			"router <vm> <ra,> <subnet>",
		},
		Call: wrapBroadcastCLI(cliRouter),
	},
	{ // clear router
		HelpShort: "",
		HelpLong:  ``,
		Patterns: []string{
			"clear router <vm>",
			"clear router <vm> <interface,>",
			"clear router <vm> <interface,> <network>",
			"clear router <vm> <interface,> <network> <IPv4/MASK or IPv6/MASK or dhcp>",
		},
		Call: wrapBroadcastCLI(cliClearRouter),
	},
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
	} else if c.BoolArgs["log"] {
		var level string
		if c.BoolArgs["fatal"] {
			level = "fatal"
		} else if c.BoolArgs["error"] {
			level = "error"
		} else if c.BoolArgs["warn"] {
			level = "warn"
		} else if c.BoolArgs["info"] {
			level = "info"
		} else if c.BoolArgs["debug"] {
			level = "debug"
		}
		RouterLogLevel(vm, level)
		return nil
	} else if c.BoolArgs["interface"] {
		network, err := strconv.Atoi(c.StringArgs["network"])
		if err != nil {
			return fmt.Errorf("invalid network: %v : %v", c.StringArgs["network"], err)
		}
		ip := c.StringArgs["IPv4/MASK"]

		err = RouterInterfaceAdd(vm, network, ip)
		if err != nil {
			return err
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

func cliClearRouter(c *minicli.Command, resp *minicli.Response) error {
	vmName := c.StringArgs["vm"]

	vm := vms.FindVM(vmName)
	if vm == nil {
		return fmt.Errorf("no such vm %v", vmName)
	}

	if c.BoolArgs["interface"] {
		network := c.StringArgs["network"]
		ip := c.StringArgs["IPv4/MASK"]

		err := RouterInterfaceDel(vm, network, ip)
		if err != nil {
			return err
		}
	} else {
		// remove everything about this router
		err := RouterInterfaceDel(vm, "", "")
		if err != nil {
			return err
		}
	}
	return nil
}
