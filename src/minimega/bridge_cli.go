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
			"tap <delete,> <tap name or all>",
		},
		Call: wrapSimpleCLI(cliHostTap),
		Suggest: wrapSuggest(func(ns *Namespace, val, prefix string) []string {
			if val == "vlan" {
				return cliVLANSuggest(ns, prefix)
			} else if val == "tap" {
				return cliTapSuggest(ns, prefix)
			} else if val == "bridge" {
				return cliBridgeSuggest(ns, prefix)
			}
			return nil
		}),
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
		Suggest: wrapSuggest(func(ns *Namespace, val, prefix string) []string {
			if val == "bridge" {
				return cliBridgeSuggest(ns, prefix)
			}
			return nil
		}),
	},
}

// routines for interfacing bridge mechanisms with the cli
func cliHostTap(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["create"] {
		b := c.StringArgs["bridge"]

		vlan, err := lookupVLAN(ns.Name, c.StringArgs["vlan"])
		if err != nil {
			return err
		}

		tap, err := hostTapCreate(b, c.StringArgs["tap"], vlan)
		if err != nil {
			return err
		}

		if c.BoolArgs["dhcp"] {
			log.Debug("obtaining dhcp on tap %v", tap)

			var out string
			out, err = processWrapper("dhclient", tap)
			if err != nil {
				err = fmt.Errorf("dhcp error %v: `%v`", err, out)
			}
		} else if c.StringArgs["ip"] != "" {
			ip := c.StringArgs["ip"]

			log.Debug("setting ip on tap %v: %v", tap, ip)

			var out string
			out, err = processWrapper("ip", "addr", "add", "dev", tap, ip)
			if err != nil {
				err = fmt.Errorf("ip error %v: `%v`", err, out)
			}
		}

		if err != nil {
			// One of the above cases failed, try to clean up the tap
			br, err := getBridge(b)
			if err == nil {
				err = br.DestroyTap(tap)
			}
			if err != nil {
				// Welp, we're boned
				log.Error("zombie host tap -- %v %v", tap, err)
			}
			return err
		}

		// need lock?
		ns.Taps[tap] = true

		resp.Response = tap

		return nil
	} else if c.BoolArgs["delete"] {
		return hostTapDelete(ns, c.StringArgs["tap"])
	}

	// Must be the list command
	hostTapList(ns, resp)

	return nil
}

func cliHostTapClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	return hostTapDelete(ns, Wildcard)
}

func cliBridge(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	iface := c.StringArgs["interface"]
	remoteIP := c.StringArgs["remote"]

	// Get the specifed bridge. If we're listing the bridges, we'll get the
	// default bridge which should be fine.
	br, err := getBridge(c.StringArgs["bridge"])
	if err != nil {
		return err
	}

	if c.BoolArgs["trunk"] {
		return br.AddTrunk(iface)
	} else if c.BoolArgs["notrunk"] {
		return br.RemoveTrunk(iface)
	} else if c.BoolArgs["tunnel"] {
		t := bridge.TunnelVXLAN
		if c.BoolArgs["gre"] {
			t = bridge.TunnelGRE
		}

		return br.AddTunnel(t, remoteIP)
	} else if c.BoolArgs["notunnel"] {
		return br.RemoveTunnel(iface)
	}

	// Must want to list bridges
	resp.Header = []string{"bridge", "preexisting", "vlans", "trunks", "tunnels"}
	resp.Tabular = [][]string{}

	for _, info := range bridges.Info() {
		vlans := []string{}
		for k, _ := range info.VLANs {
			vlans = append(vlans, printVLAN(ns.Name, k))
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

	return nil
}

func cliTapSuggest(ns *Namespace, prefix string) []string {
	res := []string{}

	if strings.HasPrefix(Wildcard, prefix) {
		res = append(res, Wildcard)
	}

	// TODO: need lock?
	for tap := range ns.Taps {
		if strings.HasPrefix(tap, prefix) {
			res = append(res, tap)
		}
	}

	return res
}

func cliBridgeSuggest(ns *Namespace, prefix string) []string {
	var res []string

	for _, v := range bridges.Info() {
		if strings.HasPrefix(v.Name, prefix) {
			res = append(res, v.Name)
		}
	}

	return res
}
