// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"os"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type dotVM struct {
	Vlans []string
	State string
	Text  string
}

var stateToColor = map[VMState]string{
	VM_BUILDING: "yellow",
	VM_RUNNING:  "green",
	VM_PAUSED:   "yellow",
	VM_QUIT:     "blue",
	VM_ERROR:    "red",
}

var dotCLIHandlers = []minicli.Handler{
	{ // viz
		HelpShort: "visualize the current experiment as a graph",
		HelpLong: `
Output the current experiment topology as a graphviz readable 'dot' file.`,
		Patterns: []string{
			"viz <filename>",
		},
		Call: wrapSimpleCLI(cliDot),
	},
}

// dot returns a graphviz 'dotfile' string representing the experiment topology
// from the perspective of this node.
func cliDot(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Create the file before running any commands incase there is an error
	fout, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		return err
	}
	defer fout.Close()

	writer := bufio.NewWriter(fout)
	fmt.Fprintln(writer, "graph minimega {")
	fmt.Fprintln(writer, `size="8,11";`)
	fmt.Fprintln(writer, "overlap=false;")
	//fmt.Fprintf(fout, "Legend [shape=box, shape=plaintext, label=\"total=%d\"];\n", len(n.effectiveNetwork))

	vlans := make(map[int]bool)

	// LOCK: This is a CLI handler so we already hold cmdLock (can call
	// globalVMs instaed of GlobalVMs).
	for _, vm := range globalVMs(ns) {
		ip, err := vm.Info("ip")
		ip6, err2 := vm.Info("ip6")
		if err != nil || err2 != nil {
			// Should never happen
			log.Error("bad VM info for %v:%v: %v", vm.GetHost(), vm.GetID(), err)
			continue
		}

		text := fmt.Sprintf(`"%v:%v:%v:%v:%v"`, vm.GetHost(), vm.GetName(), vm.GetID(), ip, ip6)
		color := stateToColor[vm.GetState()]

		fmt.Fprintf(writer, "%v [style=filled, color=%v];\n", text, color)

		for _, net := range vm.GetNetworks() {
			fmt.Fprintf(writer, "%v -- %v\n", text, net.VLAN)
			vlans[net.VLAN] = true
		}
	}

	for vlan := range vlans {
		fmt.Fprintf(writer, "%v;\n", vlan)
	}

	fmt.Fprint(writer, "}")
	return writer.Flush()
}
