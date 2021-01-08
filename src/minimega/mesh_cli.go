// Copyright 2019-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"math"
	"minicli"
	"os"
	"ranges"
	"sort"
	"strconv"
	"strings"
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
		Call:    wrapSimpleCLI(cliMeshageHangup),
		Suggest: wrapHostnameSuggest(false, true, false),
	},
	{ // mesh list
		HelpShort: "display the mesh adjacency list",
		HelpLong: `
Without "all" or "peers", displays the mesh adjacency list. If "all" is
specified, the hostnames of all nodes in the list are printed. If "peers" is
specified, the hostnames of all peers are printed (the local node is not
included).`,
		Patterns: []string{
			"mesh list [all,peers]",
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
			"mesh send <hostname or range or all> (command)",
		},
		Call:    cliMeshageSend,
		Suggest: wrapHostnameSuggest(false, false, true),
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

	switch {
	case c.BoolArgs["all"]:
		// combine keys into list
		resp.Response = ranges.UnsplitList(keys)
		resp.Data = keys
	case c.BoolArgs["peers"]:
		// reconstruct keys without local node
		for i, v := range keys {
			if v == hostname {
				keys = append(keys[:i], keys[i+1:]...)
				break
			}
		}
		resp.Response = ranges.UnsplitList(keys)
		resp.Data = keys
	default:
		// print adjacency list
		for _, key := range keys {
			v := mesh[key]
			resp.Response += fmt.Sprintf("%s\n", key)
			sort.Strings(v)
			for _, x := range v {
				resp.Response += fmt.Sprintf(" |--%s\n", x)
			}
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

	in, err := meshageSend(c.Subcommand, c.StringArgs["hostname"])
	if err != nil {
		respChan <- errResp(err)
		return
	}

	forward(in, respChan)
}

// cliHostnameSuggest takes a prefix and suggests hostnames based on nodes in
// the mesh. If local is true, the local node will be included in the
// suggestions. If direct is true, only peers of the local node will be
// included in the suggestions.
func cliHostnameSuggest(prefix string, local, direct, wild bool) []string {
	mesh := meshageNode.Mesh()

	res := []string{}

	// suggest peers of the local node
	if direct {
		for _, v := range mesh[hostname] {
			if strings.HasPrefix(v, prefix) {
				res = append(res, v)
			}
		}

		return res
	}

	// suggest nodes in the mesh
	for k := range mesh {
		// skip the local node if local is false
		if k == hostname && !local {
			continue
		}

		if strings.HasPrefix(k, prefix) {
			res = append(res, k)
		}
	}

	if local && strings.HasPrefix("localhost", prefix) {
		res = append(res, "localhost")
	}

	if wild && strings.HasPrefix(Wildcard, prefix) {
		res = append(res, Wildcard)
	}

	return res
}
