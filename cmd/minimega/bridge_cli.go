// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
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

Tap mirror mirrors packets that traverse the source tap to the destination tap.
Both taps should already exist. You can use taps for VMs from "vm info" or host
taps. For example, to mirror traffic that traverse mega_tapX to mega_tapY on
the default bridge:

	tap mirror mega_tapX mega_tapY

Mirroring is also supported via vm names/interface indices. The VM interfaces
should already be on the same bridge. VMs must be colocated.

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
			"tap <mirror,> <src name> <dst name> [bridge]",
			"tap <delete,> <tap name or all>",
		},
		Call: wrapSimpleCLI(cliTap),
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
	{ // tap mirror vm -> vm
		HelpShort: "create vm->vm tap mirror",
		HelpLong: `
Create a mirror from one VM interface to another VM interface. The VMs must be
running on the same physical node.
		`,
		Patterns: []string{
			"tap <mirror,> <vm name> <interface index> <vm2 name> <interface2 index>",
		},
		Call: wrapVMTargetCLI(cliTapMirrorVM),
		Suggest: wrapSuggest(func(ns *Namespace, val, prefix string) []string {
			if val == "vm" || val == "vm2" {
				return cliVMSuggest(ns, prefix, VM_ANY_STATE, false)
			}
			return nil
		}),
	},
	{ // clear tap
		HelpShort: "reset tap state",
		HelpLong: `
Reset state for taps. To delete individual taps, use "tap delete".

"clear tap mirror" can be used to delete one or all mirrors. Mirrors are
identified by the destination for the mirror since a source can have multiple
mirrors. "clear tap" also deletes all mirrors.

Only affects taps on the local node.`,
		Patterns: []string{
			"clear tap",
			"clear tap <mirror,> [name]",
		},
		Call: wrapSimpleCLI(cliTapClear),
	},
	{ // clear tap mirror vm
		HelpShort: "clear tap mirror for vm->vm",
		Patterns: []string{
			"clear tap <mirror,> <vm name> <interface index or all>",
		},
		Call: wrapVMTargetCLI(cliTapClearMirrorVM),
	},
	{ // bridge
		HelpShort: "display information and modify virtual bridges",
		HelpLong: `
When called with no arguments, display information about all managed bridges.

To add a trunk interface to a specific bridge, use 'bridge trunk'. For example,
to add interface bar to bridge foo:

	bridge trunk foo bar

To create a vxlan or GRE tunnel to another bridge, use 'bridge tunnel'. For
example, to create a vxlan tunnel to another bridge with IP 10.0.0.1:

	bridge tunnel vxlan mega_bridge 10.0.0.1

Note: bridge is not a namespace-aware command.`,
		Patterns: []string{
			"bridge",
			"bridge <config,> <bridge> <config>",
			"bridge <trunk,> <bridge> <interface>",
			"bridge <notrunk,> <bridge> <interface>",
			"bridge <tunnel,> <vxlan,gre> <bridge> <remote ip> [key]",
			"bridge <notunnel,> <bridge> <interface>",
			"bridge <destroy,> <bridge>",
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

func cliTap(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["create"]:
		return cliTapCreate(ns, c, resp)
	case c.BoolArgs["mirror"]:
		return cliTapMirror(ns, c, resp)
	case c.BoolArgs["delete"]:
		return cliTapDelete(ns, c, resp)
	}

	// Must be the list command
	hostTapList(ns, resp)

	return nil
}

func cliTapCreate(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
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
}

func cliTapMirror(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	br, err := getBridge(c.StringArgs["bridge"])
	if err != nil {
		return err
	}

	src := c.StringArgs["src"]
	dst := c.StringArgs["dst"]

	if err := br.CreateMirror(src, dst); err != nil {
		return err
	}

	// need lock?
	ns.Mirrors[dst] = true

	return nil
}

func cliTapMirrorVM(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// getNetwork gets the NetConfig for a given VM and interface number
	getNetwork := func(svm, si string) (NetConfig, error) {
		i, err := strconv.Atoi(si)
		if err != nil {
			return NetConfig{}, fmt.Errorf("invalid interface number: `%v`", si)
		}

		vm := ns.FindVM(svm)
		if vm == nil {
			return NetConfig{}, vmNotFound(svm)
		}

		return vm.GetNetwork(i)
	}

	n1, err := getNetwork(c.StringArgs["vm"], c.StringArgs["interface"])
	n2, err2 := getNetwork(c.StringArgs["vm2"], c.StringArgs["interface2"])
	if err != nil {
		if err2 == nil && isVMNotFound(err.Error()) {
			return fmt.Errorf("vms are not colocated or invalid vm name: %v", c.StringArgs["vm"])
		}
		// unknown error
		return err
	}
	if err2 != nil {
		if err == nil && isVMNotFound(err2.Error()) {
			return fmt.Errorf("vms are not colocated or invalid vm name: %v", c.StringArgs["vm2"])
		}
		// unknown error
		return err2
	}

	if n1.Bridge != n2.Bridge {
		return fmt.Errorf("interfaces on different bridges: %v and %v", n1.Bridge, n2.Bridge)
	}

	br, err := getBridge(n1.Bridge)
	if err != nil {
		return err
	}

	if err := br.CreateMirror(n1.Tap, n2.Tap); err != nil {
		return err
	}

	// need lock?
	ns.Mirrors[n2.Tap] = true

	return nil
}

func cliTapDelete(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	return hostTapDelete(ns, c.StringArgs["tap"])
}

func cliTapClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	switch {
	case c.BoolArgs["mirror"]:
		return cliTapClearMirror(ns, c, resp)
	}

	// must be "clear tap", still need to clear mirrors
	if err := mirrorDelete(ns, Wildcard); err != nil {
		return err
	}

	return hostTapDelete(ns, Wildcard)
}

func cliTapClearMirror(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.StringArgs["name"] != "" {
		// clear mirror by name
		return mirrorDelete(ns, c.StringArgs["name"])
	}

	// clear all mirrors
	return mirrorDelete(ns, Wildcard)
}

func cliTapClearMirrorVM(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// clear mirror by VM name (and optional index)
	return mirrorDeleteVM(ns, c.StringArgs["vm"], c.StringArgs["interface"])
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

		return br.AddTunnel(t, remoteIP, c.StringArgs["key"])
	} else if c.BoolArgs["notunnel"] {
		return br.RemoveTunnel(iface)
	} else if c.BoolArgs["config"] {
		return br.Config(c.StringArgs["config"])
	} else if c.BoolArgs["destroy"] {
		return bridges.DestroyBridge(c.StringArgs["bridge"])
	}

	// Must want to list bridges
	resp.Header = []string{"bridge", "preexisting", "vlans", "trunks", "tunnels", "config"}
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
			fmt.Sprintf("%v", info.Tunnels),
			marshal(info.Config),
		}
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
