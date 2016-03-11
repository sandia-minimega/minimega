// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	log "minilog"
	"strconv"
	"strings"
)

var vlansCLIHandlers = []minicli.Handler{
	{ // vlans
		HelpShort: "display allocated VLANs",
		HelpLong: `
Display information about allocated VLANs. With no arguments, prints out the
known VLAN aliases. The following subcommands are supported:

range		- view or set the VLAN range
add   		- add an alias
blacklist 	- blacklist a VLAN so that it is not used, even if it is in range

Note: this command is namespace aware so, for example, adding a range applies
to all *new* VLAN aliases in the current namespace.`,
		Patterns: []string{
			"vlans",
			"vlans <range,>",
			"vlans <range,> <min> <max>",
			"vlans <add,> <alias> <vlan>",
			"vlans <blacklist,> <vlan>",
		},
		Call: wrapSimpleCLI(cliVLANs),
	},
	{ // clear vlans
		HelpShort: "clear allocated VLANs",
		HelpLong: `
Clear one or more aliases, freeing the VLANs for reuse. You should only clear
allocated VLANs once you have killed all the VMs connected to them.

Note: When no prefix is specified and a namespace is not active, all state
about managed VLANs is cleared.`,
		Patterns: []string{
			"clear vlans [prefix]",
		},
		Call: wrapSimpleCLI(cliClearVLANs),
	},
}

var vlansCLISubHandlers = map[string]func(*minicli.Command, *minicli.Response){
	"add":       cliVLANsAdd,
	"range":     cliVLANsRange,
	"blacklist": cliVLANsBlacklist,
}

func cliVLANs(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Look for matching subhandler
	if len(c.BoolArgs) > 0 {
		for k, fn := range vlansCLISubHandlers {
			if c.BoolArgs[k] {
				log.Debug("vlan handler %v", k)
				fn(c, resp)
				return resp
			}
		}
	}

	// No match, must want to just print
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

	return resp
}

func cliVLANsAdd(c *minicli.Command, resp *minicli.Response) {
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
}

func cliVLANsRange(c *minicli.Command, resp *minicli.Response) {
	if c.StringArgs["min"] != "" && c.StringArgs["max"] != "" {
		min, err := strconv.Atoi(c.StringArgs["min"])
		max, err2 := strconv.Atoi(c.StringArgs["max"])

		if err != nil || err2 != nil {
			resp.Error = "expected integer values for min/max"
		} else if max < min {
			resp.Error = "expected min > max"
		} else if err := allocatedVLANs.SetRange(namespace, min, max); err != nil {
			resp.Error = err.Error()
		}

		return
	}

	// Must want to display the ranges
	resp.Header = []string{"namespace", "min", "max", "next"}
	resp.Tabular = [][]string{}

	for prefix, r := range allocatedVLANs.ranges {
		if namespace != "" && namespace != prefix {
			continue
		}

		resp.Tabular = append(resp.Tabular,
			[]string{
				prefix,
				strconv.Itoa(r.min),
				strconv.Itoa(r.max),
				strconv.Itoa(r.next),
			})
	}
}

func cliVLANsBlacklist(c *minicli.Command, resp *minicli.Response) {
	vlan, err := strconv.Atoi(c.StringArgs["vlan"])
	if err != nil {
		resp.Error = "expected integer VLAN"
		return
	}

	allocatedVLANs.Blacklist(vlan)
}

func cliClearVLANs(c *minicli.Command) *minicli.Response {
	prefix := c.StringArgs["prefix"]
	if namespace != "" {
		prefix = namespace + VLANAliasSep + prefix
	}

	if prefix == "" {
		// Clearing everything
		allocatedVLANs = NewAllocatedVLANs()
	} else {
		allocatedVLANs.Delete(prefix)
	}

	return &minicli.Response{Host: hostname}
}

// suggestVLAN returns a list of VLAN suggestions for tab completion. Performs
// a bit of extra work to make sure that the suggestions are in the current
// namespace (or complete across namespaces if the user included VLANAliasSep).
func suggestVLAN(prefix string) []string {
	if !strings.Contains(prefix, VLANAliasSep) && namespace != "" {
		prefix = namespace + VLANAliasSep + prefix
	}

	res := allocatedVLANs.GetAliases(prefix)

	for i, v := range res {
		res[i] = strings.TrimPrefix(v, namespace+VLANAliasSep)
	}

	return res
}
