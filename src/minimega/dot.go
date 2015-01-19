// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"minicli"
	"os"
	"strings"
)

type dotVM struct {
	Vlans []string
	State string
	Text  string
}

var stateToColor = map[string]string{
	"building": "yellow",
	"running":  "green",
	"paused":   "yellow",
	"quit":     "blue",
	"error":    "red",
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

func init() {
	registerHandlers("dot", dotCLIHandlers)
}

// dot returns a graphviz 'dotfile' string representing the experiment topology
// from the perspective of this node.
func cliDot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Create the file before running any commands incase there is an error
	fout, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer fout.Close()

	cmd, err := minicli.CompileCommand("vm info mask host,name,id,ip,ip6,state,vlan")
	if err != nil {
		// Should never happen
		panic(err)
	}

	writer := bufio.NewWriter(fout)

	fmt.Fprintln(writer, "graph minimega {")
	fmt.Fprintln(writer, `size=\"8,11\";`)
	fmt.Fprintln(writer, "overlap=false;")
	//fmt.Fprintf(fout, "Legend [shape=box, shape=plaintext, label=\"total=%d\"];\n", len(n.effectiveNetwork))

	var expVms []*dotVM

	// Get info from local hosts by invoking command directly
	for resp := range minicli.ProcessCommand(c, false) {
		expVms = append(expVms, dotProcessInfo(resp)...)
	}

	// Get info from remote hosts over meshage
	remoteRespChan := make(chan minicli.Responses)
	go meshageBroadcast(cmd, remoteRespChan)
	for resp := range remoteRespChan {
		expVms = append(expVms, dotProcessInfo(resp)...)
	}

	vlans := make(map[string]bool)

	for _, v := range expVms {
		color := stateToColor[v.State]
		fmt.Fprintf(writer, "%s [style=filled, color=%s];\n", v.Text, color)

		for _, c := range v.Vlans {
			fmt.Fprintf(writer, "%s -- %s\n", v.Text, c)
			vlans[c] = true
		}
	}

	for k, _ := range vlans {
		fmt.Fprintf(writer, "%s;\n", k)
	}

	fmt.Fprint(writer, "}")
	err = writer.Flush()
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func dotProcessInfo(resp minicli.Responses) []*dotVM {
	res := []*dotVM{}

	for _, r := range resp {
		// Process Tabular data, order is:
		//   host,name,id,ip,ip6,state,vlan
		row := r.Tabular[0]

		s := strings.Trim(row[6], "[]")
		vlans := strings.Split(s, ", ")

		res = append(res, &dotVM{
			Vlans: vlans,
			State: row[5],
			Text:  strings.Join(row[0:5], ":"),
		})
	}

	return res
}
