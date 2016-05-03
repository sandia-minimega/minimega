// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	log "minilog"
	"strconv"
	"strings"
	"vlans"
)

var allocatedVLANs = vlans.NewAllocatedVLANs()

var vlansCLIHandlers = []minicli.Handler{
	{ // vlans
		HelpShort: "display allocated VLANs",
		HelpLong: `
Display information about allocated VLANs. With no arguments, prints out the
known VLAN aliases. The following subcommands are supported:

range		- view or set the VLAN range
add   		- add an alias
blacklist 	- view or create blacklisted VLAN

Note: this command is namespace aware so, for example, adding a range applies
to all *new* VLAN aliases in the current namespace.`,
		Patterns: []string{
			"vlans",
			"vlans <range,>",
			"vlans <range,> <min> <max>",
			"vlans <add,> <alias> <vlan>",
			"vlans <blacklist,> [vlan]",
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

	namespace := GetNamespaceName()

	// No match, must want to just print
	resp.Header = []string{"namespace", "alias", "vlan"}
	resp.Tabular = allocatedVLANs.Tabular(namespace)

	return resp
}

func cliVLANsAdd(c *minicli.Command, resp *minicli.Response) {
	namespace := GetNamespaceName()

	// Prepend `<namespace>//` if it doesn't look like the user already
	// included it.
	alias := c.StringArgs["alias"]
	if !strings.Contains(alias, vlans.AliasSep) {
		alias = namespace + vlans.AliasSep + alias
	}

	vlan, err := strconv.Atoi(c.StringArgs["vlan"])
	if err != nil {
		resp.Error = "expected integer VLAN"
	} else if err := allocatedVLANs.AddAlias(alias, vlan); err != nil {
		resp.Error = err.Error()
	}
}

func cliVLANsRange(c *minicli.Command, resp *minicli.Response) {
	namespace := GetNamespaceName()

	if c.StringArgs["min"] != "" && c.StringArgs["max"] != "" {
		min, err := strconv.Atoi(c.StringArgs["min"])
		max, err2 := strconv.Atoi(c.StringArgs["max"])

		if err != nil || err2 != nil {
			resp.Error = "expected integer values for min/max"
		} else if max <= min {
			resp.Error = "expected min > max"
		} else if err := allocatedVLANs.SetRange(namespace, min, max); err != nil {
			resp.Error = err.Error()
		}

		return
	}

	// Must want to display the ranges
	resp.Header = []string{"namespace", "min", "max", "next"}
	resp.Tabular = [][]string{}

	for prefix, r := range allocatedVLANs.GetRanges() {
		if namespace != "" && namespace != prefix {
			continue
		}

		resp.Tabular = append(resp.Tabular,
			[]string{
				prefix,
				strconv.Itoa(r.Min),
				strconv.Itoa(r.Max),
				strconv.Itoa(r.Next),
			})
	}
}

func cliVLANsBlacklist(c *minicli.Command, resp *minicli.Response) {
	if v := c.StringArgs["vlan"]; v != "" {
		vlan, err := strconv.Atoi(v)
		if err != nil {
			resp.Error = "expected integer VLAN"
			return
		}

		allocatedVLANs.Blacklist(vlan)
		return
	}

	// Must want to display the blacklisted VLANs
	resp.Header = []string{"vlan"}
	resp.Tabular = [][]string{}

	for _, v := range allocatedVLANs.GetBlacklist() {
		resp.Tabular = append(resp.Tabular,
			[]string{
				strconv.Itoa(v),
			})
	}
}

func cliClearVLANs(c *minicli.Command) *minicli.Response {
	namespace := GetNamespaceName()

	prefix := c.StringArgs["prefix"]
	if namespace != "" {
		prefix = namespace + vlans.AliasSep + prefix
	}

	if prefix == "" {
		// Clearing everything
		allocatedVLANs = vlans.NewAllocatedVLANs()
	} else {
		allocatedVLANs.Delete(namespace, prefix)
	}

	return &minicli.Response{Host: hostname}
}

// suggestVLAN returns a list of VLAN suggestions for tab completion. Performs
// a bit of extra work to make sure that the suggestions are in the current
// namespace (completes across namespaces if prefix includes vlans.AliasSep).
func suggestVLAN(prefix string) []string {
	namespace := GetNamespaceName()

	if !strings.Contains(prefix, vlans.AliasSep) && namespace != "" {
		prefix = namespace + vlans.AliasSep + prefix
	}

	res := allocatedVLANs.GetAliases(prefix)

	for i, v := range res {
		res[i] = strings.TrimPrefix(v, namespace+vlans.AliasSep)
	}

	return res
}
