// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"ron"
	"strconv"
	"text/tabwriter"
)

const (
	CC_PORT = 9002
)

var (
	ccNode *ron.Ron
)

//cc layer syntax should look like:
//
//cc start [port]
//cc command [new <command> [norecord] [background], delete <command id>]
//cc file <send,receive> <file> [<file>,...]
//cc filter [add [uuid=<uuid>,...], delete <filter id>, clear]
//cc responses [command id]
//...

func cliCC(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		if ccNode == nil {
			return cliResponse{
				Response: "running: false",
			}
		}

		port := ccNode.GetPort()
		clients := ccNode.GetActiveClients()

		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "running:\ttrue\n")
		fmt.Fprintf(w, "port:\t%v\n", port)
		fmt.Fprintf(w, "clients:\t%v\n", len(clients))

		w.Flush()

		return cliResponse{
			Response: o.String(),
		}
	}

	if c.Args[0] != "start" {
		if ccNode == nil {
			return cliResponse{
				Error: "cc service not running",
			}
		}
	}

	switch c.Args[0] {
	case "start":
		port := CC_PORT
		if len(c.Args) > 1 {
			p, err := strconv.Atoi(c.Args[1])
			if err != nil {
				return cliResponse{
					Error: fmt.Sprintf("invalid port %v : %v", c.Args[1], err),
				}
			}
			port = p
		}

		var err error
		ccNode, err = ron.New(port, ron.MODE_MASTER, "", *f_base)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("creating cc node %v", err),
			}
		}
		log.Debug("created ron node at %v %v", port, *f_base)
	case "command":
		if len(c.Args) == 1 {
			// command summary
		}

		switch c.Args[1] {
		case "new":
		case "delete":
			if len(c.Args) != 2 {
				return cliResponse{
					Error: fmt.Sprintf("malformed command: %v", c),
				}
			}
			cid, err := strconv.Atoi(c.Args[2])
			if err != nil {
				return cliResponse{
					Error: fmt.Sprintf("invalid command id %v : %v", c.Args[2], err),
				}
			}
			err = ccNode.DeleteCommand(cid)
			if err != nil {
				return cliResponse{
					Error: fmt.Sprintf("deleting command %v: %v", cid, err),
				}
			}
		default:
			return cliResponse{
				Error: fmt.Sprintf("malformed command: %v", c),
			}
		}
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command: %v", c),
		}
	}
	return cliResponse{}
}

func ccClients() map[string]bool {
	clients := make(map[string]bool)
	if ccNode != nil {
		c := ccNode.GetActiveClients()
		for _, v := range c {
			clients[v] = true
		}
		return clients
	}
	return nil
}
