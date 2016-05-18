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
func cliHostTap(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["create"] {
		vlan, err := lookupVLAN(c.StringArgs["vlan"])
		if err != nil {
			return err
		}

		bridge := c.StringArgs["bridge"]
		if bridge == "" {
			bridge = DEFAULT_BRIDGE
		}

		if isReserved(bridge) {
			return fmt.Errorf("`%s` is a reserved word -- cannot use for bridge name", bridge)
		}

		ip := c.StringArgs["ip"]
		if c.BoolArgs["dhcp"] {
			ip = "dhcp"
		} else if ip == "" {
			ip = "none"
		}

		tap := c.StringArgs["tap"]
		if isReserved(tap) {
			return fmt.Errorf("`%s` is a reserved word -- cannot use for tap name", tap)
		}

		tap, err = hostTapCreate(bridge, ip, tap, vlan)
		if err == nil {
			resp.Response = tap
		}
		return err
	} else if c.BoolArgs["delete"] {
		return hostTapDelete(c.StringArgs["id"])
	}

	// Must be the list command
	hostTapList(resp)
	return nil
}

func cliHostTapClear(c *minicli.Command, resp *minicli.Response) error {
	return hostTapDelete(Wildcard)
}

func cliBridge(c *minicli.Command, resp *minicli.Response) error {
	bridge := c.StringArgs["bridge"]
	iface := c.StringArgs["interface"]
	remoteIP := c.StringArgs["remote"]

	if strings.HasPrefix(c.Original, "bridge notrunk") {
		b, err := getBridge(bridge)
		if err != nil {
			return err
		}

		return b.TrunkRemove(iface)
	} else if strings.HasPrefix(c.Original, "bridge trunk") {
		b, err := getBridge(bridge)
		if err != nil {
			return err
		}

		return b.TrunkAdd(iface)
	} else if strings.HasPrefix(c.Original, "bridge tunnel") {
		b, err := getBridge(bridge)
		if err != nil {
			return err
		}

		var t int
		if c.BoolArgs["gre"] {
			t = TYPE_GRE
		} else {
			t = TYPE_VXLAN
		}

		return b.TunnelAdd(t, remoteIP)
	} else if strings.HasPrefix(c.Original, "bridge notunnel") {
		b, err := getBridge(bridge)
		if err != nil {
			return err
		}

		return b.TunnelRemove(iface)
	}

	// must want to list bridges
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
			vlans2 = append(vlans2, printVLAN(k))
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

	return nil
}
