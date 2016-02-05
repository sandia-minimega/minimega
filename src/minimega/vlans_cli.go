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
			"vlans",
			"vlans <add,> <alias> <vlan>",
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

	if c.BoolArgs["add"] {
		// Prepend `<namespace>//` if it doesn't look like the user already
		// included it.
		alias := c.StringArgs["alias"]
		if !strings.Contains(alias, VLANAliasSep) {
			alias = namespace + VLANAliasSep + alias
		}

		vlan, err := strconv.Atoi(c.StringArgs["vlan"])
		if err != nil {
			resp.Error = "expected integer VLAN"
		} else if err := allocatedVLANs.AddAlias(alias, vlan); err != nil {
			resp.Error = err.Error()
		}
	} else {
		resp.Header = []string{"namespace", "alias", "vlan"}
		resp.Tabular = [][]string{}

		for alias, vlan := range allocatedVLANs.byAlias {
			parts := strings.Split(alias, VLANAliasSep)
			if namespace != "" && namespace != parts[0] {
				continue
			}

			resp.Tabular = append(resp.Tabular,
				[]string{
					parts[0],
					strings.Join(parts[1:], VLANAliasSep),
					strconv.Itoa(vlan),
				})
		}
	}

	return resp
}

func cliClearVLANs(c *minicli.Command) *minicli.Response {
	prefix := c.StringArgs["prefix"]
	if namespace != "" {
		prefix = namespace + VLANAliasSep + prefix
	}

	allocatedVLANs.Delete(prefix)

	return &minicli.Response{Host: hostname}
}
