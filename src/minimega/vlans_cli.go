// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	"strconv"
	"strings"
)

var vlansCLIHandlers = []minicli.Handler{
	{ // vlans
		HelpShort: "display allocated VLANs",
		HelpLong: `
Display information about allocated VLANs.`,
		Patterns: []string{
			"vlans [alias or VLAN]",
		},
		Call: wrapSimpleCLI(cliVLANs),
	},
	{ // clear vlans
		HelpShort: "clear allocated VLANs",
		HelpLong: `
Free one or all allocated VLANs for reuse. You should only clear allocated
VLANs once you have killed all the VMs connected to them.`,
		Patterns: []string{
			"clear vlans [prefix]",
		},
		Call: wrapSimpleCLI(cliClearVLANs),
	},
}

func cliVLANs(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if alias, ok := c.StringArgs["alias"]; ok {
		if vlan, err := strconv.Atoi(alias); err != nil {
			resp.Response = allocatedVLANs.GetAlias(vlan)
		} else if vlan := allocatedVLANs.GetVLAN(alias); vlan != DisconnectedVLAN {
			resp.Response = strconv.Itoa(vlan)
		}
	} else {
		resp.Header = []string{"Alias", "VLAN"}
		resp.Tabular = [][]string{}

		for alias, vlan := range allocatedVLANs.byAlias {
			if namespace != "" && !strings.HasPrefix(alias, namespace) {
				continue
			}

			resp.Tabular = append(resp.Tabular,
				[]string{
					alias,
					strconv.Itoa(vlan),
				})
		}
	}

	return resp
}

func cliClearVLANs(c *minicli.Command) *minicli.Response {
	allocatedVLANs.Delete(c.StringArgs["prefix"])

	return &minicli.Response{Host: hostname}
}
