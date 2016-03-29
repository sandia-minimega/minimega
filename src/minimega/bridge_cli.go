// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"sort"
	"strconv"
	"strings"
)

var bridgeCLIHandlers = []minicli.Handler{
	{ // tap
		HelpShort: "control host taps for communicating between hosts and VMs",
		HelpLong: `
Control host taps on a named vlan for communicating between a host and any VMs
on that vlan.

Calling tap with no arguments will list all created taps.

To create a tap on a particular vlan, invoke tap with the create command:

	tap create <vlan>

For example, to create a host tap with ip and netmask 10.0.0.1/24 on VLAN 5:

	tap create 5 ip 10.0.0.1/24

Optionally, you can specify the bridge to create the host tap on:

	tap create <vlan> bridge <bridge> ip <ip>

You can also optionally specify the tap name, otherwise the tap will be in the
form of mega_tapX.

Additionally, you can bring the tap up with DHCP by using "dhcp" instead of a
ip/netmask:

	tap create 5 dhcp

To delete a host tap, use the delete command and tap name from the tap list:

	tap delete <id>

To delete all host taps, use id all, or 'clear tap':

	tap delete all

Note: tap is not a namespace-aware command.`,
		Patterns: []string{
			"tap",
			"tap <create,> <vlan>",
			"tap <create,> <vlan> name <tap name>",
			"tap <create,> <vlan> <dhcp,> [tap name]",
			"tap <create,> <vlan> ip <ip> [tap name]",
			"tap <create,> <vlan> bridge <bridge>",
			"tap <create,> <vlan> bridge <bridge> name [tap name]",
			"tap <create,> <vlan> bridge <bridge> <dhcp,> [tap name]",
			"tap <create,> <vlan> bridge <bridge> ip <ip> [tap name]",
			"tap <delete,> <id or all>",
		},
		Call: wrapSimpleCLI(cliHostTap),
		Suggest: func(val, prefix string) []string {
			if val == "vlan" {
				return suggestVLAN(prefix)
			} else {
				return nil
			}
		},
	},
	{ // clear tap
		HelpShort: "reset tap state",
		HelpLong: `
Reset state for taps. See "help tap" for more information.`,
		Patterns: []string{
			"clear tap",
		},
		Call: wrapSimpleCLI(cliHostTapClear),
	},
	{ // bridge
		HelpShort: "display information and modify virtual bridges",
		HelpLong: `
When called with no arguments, display information about all managed bridges.

To add a trunk interface to a specific bridge, use 'bridge trunk'. For example,
to add interface bar to bridge foo:

	bridge trunk foo bar

To create a vxlan or GRE tunnel to another bridge, use 'bridge tunnel'. For example, to create a vxlan tunnel to another bridge with IP 10.0.0.1:

	bridge tunnel vxlan, mega_bridge 10.0.0.1

Note: bridge is not a namespace-aware command.`,
		Patterns: []string{
			"bridge",
			"bridge trunk <bridge> <interface>",
			"bridge notrunk <bridge> <interface>",
			"bridge tunnel <vxlan,gre> <bridge> <remote ip>",
			"bridge notunnel <bridge> <interface>",
		},
		Call: wrapSimpleCLI(cliBridge),
	},
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["create"] {
		arg := c.StringArgs["vlan"]
		vlan, err := allocatedVLANs.ParseVLAN(namespace, arg, true)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		bridge := c.StringArgs["bridge"]
		if bridge == "" {
			bridge = DEFAULT_BRIDGE
		}

		if isReserved(bridge) {
			resp.Error = fmt.Sprintf("`%s` is a reserved word -- cannot use for bridge name", bridge)
			return resp
		}

		ip := c.StringArgs["ip"]
		if c.BoolArgs["dhcp"] {
			ip = "dhcp"
		} else if ip == "" {
			ip = "none"
		}

		tap := c.StringArgs["tap"]
		if isReserved(tap) {
			resp.Error = fmt.Sprintf("`%s` is a reserved word -- cannot use for tap name", tap)
			return resp
		}

		if tap, err := hostTapCreate(bridge, ip, tap, vlan); err != nil {
			resp.Error = err.Error()
		} else {
			resp.Response = tap
		}
	} else if c.BoolArgs["delete"] {
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

	err := hostTapDelete(Wildcard)
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliBridge(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	bridge := c.StringArgs["bridge"]
	iface := c.StringArgs["interface"]
	remoteIP := c.StringArgs["remote"]

	if strings.HasPrefix(c.Original, "bridge notrunk") {
		b, err := getBridge(bridge)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		err = b.TrunkRemove(iface)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else if strings.HasPrefix(c.Original, "bridge trunk") {
		b, err := getBridge(bridge)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		err = b.TrunkAdd(iface)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else if strings.HasPrefix(c.Original, "bridge tunnel") {
		b, err := getBridge(bridge)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		var t int
		if c.BoolArgs["gre"] {
			t = TYPE_GRE
		} else {
			t = TYPE_VXLAN
		}

		err = b.TunnelAdd(t, remoteIP)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else if strings.HasPrefix(c.Original, "bridge notunnel") {
		b, err := getBridge(bridge)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		err = b.TunnelRemove(iface)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else {
		resp.Header = []string{"Bridge", "Existed before minimega", "Active VLANS", "Trunk ports", "Tunnels"}
		resp.Tabular = [][]string{}

		bridgeLock.Lock()
		defer bridgeLock.Unlock()

		for _, v := range bridges {
			vlans := map[int]bool{}
			for _, tap := range v.Taps {
				vlans[tap.lan] = true
			}

			vlans2 := []string{}
			for k, _ := range vlans {
				vlans2 = append(vlans2, allocatedVLANs.PrintVLAN(namespace, k))
			}
			sort.Strings(vlans2)

			row := []string{
				v.Name,
				strconv.FormatBool(v.preExist),
				fmt.Sprintf("%v", vlans2),
				fmt.Sprintf("%v", v.Trunk),
				fmt.Sprintf("%v", v.Tunnel)}
			resp.Tabular = append(resp.Tabular, row)
		}
	}

	return resp
}
