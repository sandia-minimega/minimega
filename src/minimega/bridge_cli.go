// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bridge"
	"fmt"
	"minicli"
	log "minilog"
	"sort"
	"strconv"
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

Note: taps created while a namespace is active belong to that namespace and
will only be listed when that namespace is active (or no namespace is active).
Similarly, delete only applies to the taps in the active namespace. Unlike the
"vlans" API, taps with the same name cannot exist in different namespaces.`,
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
			"bridge <trunk,> <bridge> <interface>",
			"bridge <notrunk,> <bridge> <interface>",
			"bridge <tunnel,> <vxlan,gre> <bridge> <remote ip>",
			"bridge <notunnel,> <bridge> <interface>",
		},
		Call: wrapSimpleCLI(cliBridge),
	},
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["create"] {
		b := c.StringArgs["bridge"]

		tap, err := hostTapCreate(b, c.StringArgs["tap"], c.StringArgs["vlan"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		if c.BoolArgs["dhcp"] {
			log.Debug("obtaining dhcp on tap %v", tap)

			out, err := processWrapper("dhcp", tap)
			if err != nil {
				resp.Error = fmt.Sprintf("dhcp error %v: `%v`", err, out)
			}
		} else if c.StringArgs["ip"] != "" {
			ip := c.StringArgs["ip"]

			log.Debug("setting ip on tap %v: %v", tap, ip)

			// Must be a static IP
			out, err := processWrapper("ip", "addr", "add", "dev", tap, ip)
			if err != nil {
				resp.Error = fmt.Sprintf("ip error %v: `%v`", err, out)
			}
		}

		if resp.Error != "" {
			// One of the above cases failed, try to clean up the tap
			if err := hostTapDelete(tap); err != nil {
				// Welp, we're boned
				log.Error("zombie tap -- %v %v", tap, err)
			}
		} else {
			// Success!
			if namespace != "" {
				namespaces[namespace].Taps[tap] = true
			}

			resp.Response = tap
		}

		return resp
	} else if c.BoolArgs["delete"] {
		if err := hostTapDelete(c.StringArgs["id"]); err != nil {
			resp.Error = err.Error()
		}

		return resp
	}

	// Must be the list command
	hostTapList(resp)

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

	iface := c.StringArgs["interface"]
	remoteIP := c.StringArgs["remote"]

	// Get the specifed bridge. If we're listing the bridges, we'll get the
	// default bridge which should be fine.
	br, err := getBridge(c.StringArgs["bridge"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if c.BoolArgs["trunk"] {
		err = br.AddTrunk(iface)
	} else if c.BoolArgs["notrunk"] {
		err = br.RemoveTrunk(iface)
	} else if c.BoolArgs["tunnel"] {
		t := bridge.TunnelVXLAN
		if c.BoolArgs["gre"] {
			t = bridge.TunnelGRE
		}

		err = br.AddTunnel(t, remoteIP)
	} else if c.BoolArgs["notunnel"] {
		err = br.RemoveTunnel(iface)
	} else {
		resp.Header = []string{"Bridge", "Existed before minimega", "Active VLANs", "Trunk ports", "Tunnels"}
		resp.Tabular = [][]string{}

		for _, info := range bridges.Info() {
			vlans := []string{}
			for k, _ := range info.VLANs {
				vlans = append(vlans, allocatedVLANs.PrintVLAN(namespace, k))
			}
			sort.Strings(vlans)

			row := []string{
				info.Name,
				strconv.FormatBool(info.PreExist),
				fmt.Sprintf("%v", vlans),
				fmt.Sprintf("%v", info.Trunks),
				fmt.Sprintf("%v", info.Tunnels)}
			resp.Tabular = append(resp.Tabular, row)
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
