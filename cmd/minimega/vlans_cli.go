// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var VLANs = vlans.NewVLANs()

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

By default, "clear vlans" only clears aliases for the current namespace. If
given "all" as the prefix, all state about managed VLANs is cleared across
*all* namespaces, including blacklisted VLANS. You should only use this if you
want a completely clean slate.`,
		Patterns: []string{
			"clear vlans [prefix]",
		},
		Call: wrapSimpleCLI(cliClearVLANs),
	},
}

var vlansCLISubHandlers = map[string]wrappedCLIFunc{
	"add":       cliVLANsAdd,
	"range":     cliVLANsRange,
	"blacklist": cliVLANsBlacklist,
}

func cliVLANs(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Look for matching subhandler
	if len(c.BoolArgs) > 0 {
		for k, fn := range vlansCLISubHandlers {
			if c.BoolArgs[k] {
				log.Debug("vlan handler %v", k)
				return fn(ns, c, resp)
			}
		}
	}

	// No match, must want to just print
	resp.Header = []string{"alias", "vlan"}
	resp.Tabular = vlans.Tabular(ns.Name)

	return nil
}

func cliVLANsAdd(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	alias := c.StringArgs["alias"]

	vlan, err := strconv.Atoi(c.StringArgs["vlan"])
	if err != nil {
		return errors.New("expected integer VLAN")
	}

	err = vlans.AddAlias(ns.Name, alias, vlan)
	if err == nil {
		// update file so that we have a copy of the vlans if minimega crashes
		mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())
	}

	return err
}

func cliVLANsRange(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// ranges are special if we're in the default namespace -- a range gets set
	// globally for all namespaces.
	name := ns.Name
	if name == DefaultNamespace {
		name = ""
	}

	if c.StringArgs["min"] != "" && c.StringArgs["max"] != "" {
		min, err := strconv.Atoi(c.StringArgs["min"])
		max, err2 := strconv.Atoi(c.StringArgs["max"])

		if err != nil || err2 != nil {
			return errors.New("expected integer values for min/max")
		} else if max <= min {
			return errors.New("expected min > max")
		}

		return vlans.SetRange(name, min, max)
	}

	// Must want to display the ranges
	resp.Header = []string{"min", "max", "next"}
	resp.Tabular = [][]string{}

	for prefix, r := range vlans.GetRanges() {
		if name != prefix {
			continue
		}

		resp.Tabular = append(resp.Tabular,
			[]string{
				strconv.Itoa(r.Min),
				strconv.Itoa(r.Max),
				strconv.Itoa(r.Next),
			})
	}

	return nil
}

func cliVLANsBlacklist(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if v := c.StringArgs["vlan"]; v != "" {
		vlan, err := strconv.Atoi(v)
		if err != nil {
			return errors.New("expected integer VLAN")
		}

		vlans.Blacklist(vlan)
		return nil
	}

	// Must want to display the blacklisted VLANs
	resp.Header = []string{"vlan"}
	resp.Tabular = [][]string{}

	for _, v := range vlans.GetBlacklist() {
		resp.Tabular = append(resp.Tabular,
			[]string{
				strconv.Itoa(v),
			})
	}

	return nil
}

func cliClearVLANs(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	prefix := c.StringArgs["prefix"]

	if prefix == Wildcard {
		log.Info("resetting VLAN state")
		vlans.Default = vlans.NewVLANs()
		return nil
	}

	vlans.Delete(ns.Name, prefix)
	mustWrite(filepath.Join(*f_base, "vlans"), vlanInfo())

	return nil
}

// cliVLANSuggest returns a list of VLAN suggestions for tab completion.
// Performs a bit of extra work to make sure that the suggestions are in the
// current namespace (completes across namespaces if prefix includes
// vlans.AliasSep).
func cliVLANSuggest(ns *Namespace, prefix string) []string {
	if !strings.Contains(prefix, vlans.AliasSep) {
		prefix = ns.Name + vlans.AliasSep + prefix
	}

	res := vlans.GetAliases(ns.Name)

	for i, v := range res {
		res[i] = strings.TrimPrefix(v, ns.Name+vlans.AliasSep)
	}

	return res
}
