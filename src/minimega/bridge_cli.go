// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import "minicli"

var bridgeCLIHandlers = []minicli.Handler{
	{ // tap
		HelpShort: "control host taps for communicating between hosts and VMs",
		HelpLong: `
Control host taps on a named vlan for communicating between a host and any VMs
on that vlan.

Calling tap with no arguments will list all created taps.

To create a tap on a particular vlan, invoke tap with the create command:

	tap create <vlan> <ip/dhcp>

For example, to create a host tap with ip and netmask 10.0.0.1/24 on VLAN 5:

	tap create 5 10.0.0.1/24

Optionally, you can specify the bridge to create the host tap on:

	tap create <bridge> <vlan> <ip/dhcp>

You can also optionally specify the tap name, otherwise the tap will be in the
form of mega_tapX.

Additionally, you can bring the tap up with DHCP by using "dhcp" instead of a
ip/netmask:

	tap create 5 dhcp

To delete a host tap, use the delete command and tap name from the tap list:

	tap delete <id>

To delete all host taps, use id -1, or 'clear tap':

	tap delete -1`,
		Patterns: []string{
			"tap",
			"tap create <vlan> [tap name]",
			"tap create <vlan> bridge <bridge> [tap name]",
			"tap create <vlan> <dhcp,> [tap name]",
			"tap create <vlan> ip <ip> [tap name]",
			"tap create <vlan> bridge <bridge> <dhcp,> [tap name]",
			"tap create <vlan> bridge <bridge> ip <ip> [tap name]",
			"tap delete <id or *>",
		},
		Call: wrapSimpleCLI(cliHostTap),
	},
	{ // clear tap
		HelpShort: "reset tap state",
		HelpLong: `
Resets state for taps. See "help tap" for more information.`,
		Patterns: []string{
			"clear tap",
		},
		Call: wrapSimpleCLI(cliHostTapClear),
	},
	{ // bridge
		HelpShort: "display information about virtual bridges",
		Patterns: []string{
			"bridge",
		},
		Call: wrapSimpleCLI(cliBridgeInfo),
	},
}

func init() {
	registerHandlers("bridge", bridgeCLIHandlers)
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.StringArgs["vlan"] != "" {
		// Must be one of the create commands
		vlan := c.StringArgs["vlan"]

		bridge := c.StringArgs["bridge"]
		if bridge == "" {
			bridge = DEFAULT_BRIDGE
		}

		ip := c.StringArgs["ip"]
		if c.BoolArgs["dhcp"] {
			ip = "dhcp"
		} else if ip == "" {
			ip = "none"
		}

		tapName := c.StringArgs["tap"]

		tapName, err := hostTapCreate(bridge, vlan, ip, tapName)
		if err != nil {
			resp.Error = err.Error()
		} else {
			resp.Response = tapName
		}
	} else if c.StringArgs["id"] != "" {
		// Must be the delete command
		err := hostTapDelete(c.StringArgs["id"])
		if err != nil {
			resp.Error = err.Error()
		}
	} else {
		// Must be the list command
		hostTapList(resp)
	}

	return resp
}

func cliHostTapClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := hostTapDelete("*")
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliBridgeInfo(c *minicli.Command) *minicli.Response {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	resp := &minicli.Response{
		Host:     hostname,
		Response: bridgeInfo(),
	}

	return resp
}
