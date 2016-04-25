// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	meshageCommandLock sync.Mutex
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

When a mesh command is issued, if a response isn't sent within mesh_timeout
seconds, the command will be dropped and any future response will be discarded.
Note that this does not cancel the outstanding command - the node receiving the
command may still complete - but rather this node will stop waiting on a
response.`,
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

func meshageHandler() {
	for {
		m := <-meshageCommandChan
		go func() {
			mCmd := m.Body.(meshageCommand)

			cmd, err := minicli.Compile(mCmd.Original)
			if err != nil {
				log.Error("invalid command from mesh: `%s`", mCmd.Original)
				return
			}
			cmd.SetSource(SourceMeshage)

			// Copy the Record flag at each level of nested command
			for c, c2 := cmd, &mCmd.Command; c != nil && c2 != nil; {
				// could all be the post statement...
				c.Record = c2.Record
				c, c2 = c.Subcommand, c2.Subcommand
			}

			resps := []minicli.Responses{}
			for resp := range runCommand(cmd) {
				resps = append(resps, resp)
			}

			if len(resps) > 1 || len(resps[0]) > 1 {
				// This should never happen because the only commands that
				// return multiple responses are `read` and `mesh send` which
				// aren't supposed to be sent across meshage.
				log.Error("unsure how to process multiple responses!!")
			}

			resp := meshageResponse{Response: *resps[0][0], TID: mCmd.TID}
			recipient := []string{m.Source}

			_, err = meshageNode.Set(recipient, resp)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}

// cli commands for meshage control
func cliMeshageDegree(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.StringArgs["degree"] != "" {
		degree, err := strconv.ParseUint(c.StringArgs["degree"], 0, 10)
		if err != nil {
			resp.Error = err.Error()
		} else {
			meshageNode.SetDegree(uint(degree))
		}
	} else {
		resp.Response = fmt.Sprintf("%d", meshageNode.GetDegree())
	}

	return resp
}

func cliMeshageDial(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := meshageNode.Dial(c.StringArgs["hostname"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliMeshageDot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	f, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer f.Close()

	f.WriteString(meshageNode.Dot())

	return resp
}

func cliMeshageHangup(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := meshageNode.Hangup(c.StringArgs["hostname"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliMeshageList(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

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

	return resp
}

func cliMeshageStatus(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	mesh := meshageNode.Mesh()
	degree := meshageNode.GetDegree()
	nodes := len(mesh)

	resp.Header = []string{"mesh size", "degree", "peers", "context", "port"}
	resp.Tabular = [][]string{
		[]string{
			strconv.Itoa(nodes),
			strconv.FormatUint(uint64(degree), 10),
			strconv.Itoa(len(mesh[hostname])),
			*f_context,
			strconv.Itoa(*f_port),
		},
	}

	return resp
}

func cliMeshageTimeout(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.StringArgs["timeout"] != "" {
		timeout, err := strconv.Atoi(c.StringArgs["timeout"])
		if err != nil {
			resp.Error = err.Error()
		} else {
			meshageTimeout = time.Duration(timeout) * time.Second
		}
	} else {
		resp.Response = fmt.Sprintf("%v", meshageTimeout)
	}

	return resp
}

func cliMeshageSend(c *minicli.Command, respChan chan minicli.Responses) {
	in, err := meshageSend(c.Subcommand, c.StringArgs["clients"])
	if err != nil {
		respChan <- errResp(err)
		return
	}

	forward(in, respChan)
}
