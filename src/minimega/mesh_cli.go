// Copyright (2019) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math"
	"minicli"
	"os"
	"sort"
	"strconv"
	"time"
)

var meshageCLIHandlers = []minicli.Handler{
	{ // mesh degree
		HelpShort: "view or set the current degree for this mesh node",
		Patterns: []string{
			"mesh degree [degree]",
		},
		Call: wrapSimpleCLI(cliMeshageDegree),
	},
	{ // mesh dial
		HelpShort: "attempt to connect this node to another node",
		Patterns: []string{
			"mesh dial <hostname>",
		},
		Call: wrapSimpleCLI(cliMeshageDial),
	},
	{ // mesh dot
		HelpShort: "output a graphviz formatted dot file",
		HelpLong: `
Output a graphviz formatted dot file representing the connected topology.`,
		Patterns: []string{
			"mesh dot <filename>",
		},
		Call: wrapSimpleCLI(cliMeshageDot),
	},
	{ // mesh hangup
		HelpShort: "disconnect from a client",
		Patterns: []string{
			"mesh hangup <hostname>",
		},
		Call: wrapSimpleCLI(cliMeshageHangup),
	},
	{ // mesh list
		HelpShort: "display the mesh adjacency list",
		Patterns: []string{
			"mesh list",
		},
		Call: wrapSimpleCLI(cliMeshageList),
	},
	{ // mesh status
		HelpShort: "display a short status report of the mesh",
		Patterns: []string{
			"mesh status",
		},
		Call: wrapSimpleCLI(cliMeshageStatus),
	},
	{ // mesh timeout
		HelpShort: "view or set the mesh timeout",
		HelpLong: `
View or set the timeout on sending mesh commands.

When a mesh command is issued, if a response isn't sent within mesh timeout
seconds, the command will be dropped and any future response will be discarded.
Note that this does not cancel the outstanding command - the node receiving the
command may still complete - but rather this node will stop waiting on a
response.

By default, the mesh timeout is 0 which disables timeouts.`,
		Patterns: []string{
			"mesh timeout [timeout]",
		},
		Call: wrapSimpleCLI(cliMeshageTimeout),
	},
	{ // mesh send
		HelpShort: "send a command to one or more connected clients",
		HelpLong: `
Send a command to one or more connected clients. For example, to get the
vm info from nodes kn1 and kn2:

	mesh send kn[1-2] vm info

You can use 'all' to send a command to all connected clients.`,
		Patterns: []string{
			"mesh send <clients or all> (command)",
		},
		Call: cliMeshageSend,
	},
}

// cli commands for meshage control
func cliMeshageDegree(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.StringArgs["degree"] != "" {
		degree, err := strconv.ParseUint(c.StringArgs["degree"], 0, 10)
		if err != nil {
			return err
		}

		meshageNode.SetDegree(uint(degree))
		return nil
	}

	resp.Response = fmt.Sprintf("%d", meshageNode.GetDegree())
	return nil
}

func cliMeshageDial(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	return meshageNode.Dial(c.StringArgs["hostname"])
}

func cliMeshageHangup(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	return meshageNode.Hangup(c.StringArgs["hostname"])
}

func cliMeshageDot(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	f, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(meshageNode.Dot())
	return err
}

func cliMeshageList(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	mesh := meshageNode.Mesh()

	var keys []string
	for k, _ := range mesh {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		v := mesh[key]
		resp.Response += fmt.Sprintf("%s\n", key)
		sort.Strings(v)
		for _, x := range v {
			resp.Response += fmt.Sprintf(" |--%s\n", x)
		}
	}

	return nil
}

func cliMeshageStatus(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	mesh := meshageNode.Mesh()
	degree := meshageNode.GetDegree()
	nodes := len(mesh)

	resp.Header = []string{"size", "degree", "peers", "context", "port"}
	resp.Tabular = [][]string{
		[]string{
			strconv.Itoa(nodes),
			strconv.FormatUint(uint64(degree), 10),
			strconv.Itoa(len(mesh[hostname])),
			*f_context,
			strconv.Itoa(*f_port),
		},
	}

	return nil
}

func cliMeshageTimeout(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.StringArgs["timeout"] != "" {
		timeout, err := strconv.Atoi(c.StringArgs["timeout"])
		if err != nil {
			return err
		}

		meshageTimeout = time.Duration(timeout) * time.Second
		// If the timeout is 0, set to "unlimited"
		if meshageTimeout == 0 {
			meshageTimeout = math.MaxInt64
		}

		return nil
	}

	if meshageTimeout == math.MaxInt64 {
		resp.Response = "unlimited"
	} else {
		resp.Response = meshageTimeout.String()
	}

	return nil
}

func cliMeshageSend(c *minicli.Command, respChan chan<- minicli.Responses) {
	// HAX: prevent running as a subcommand
	if c.Source == SourceMeshage {
		err := fmt.Errorf("cannot run `%s` via meshage", c.Original)
		respChan <- errResp(err)
		return
	}

	// set the source so that remote nodes do not try to do any non-local
	// behaviors, see wrapBroadcastCLI.
	c.Subcommand.SetSource(SourceMeshage)

	in, err := meshageSend(c.Subcommand, c.StringArgs["clients"])
	if err != nil {
		respChan <- errResp(err)
		return
	}

	forward(in, respChan)
}
