// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"ron"
	"strconv"
)

const (
	CC_PORT = 9001
)

var (
	ccNode *ron.Ron
)

//cc layer syntax should look like:
//
//cc start [port]
//cc command new ...
//cc command list
//cc command delete ...
//cc responses [command id]
//...

func cliCC(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		// TODO: summary?
		return cliResponse{
			Response: "not implemented",
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

		ccNode, err := ron.New(port, ron.MODE_MASTER, "", *f_base)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("creating cc node %v", err),
			}
		}
		log.Debug("created ron node at %v %v with UUID %v", port, *f_base, ccNode.UUID)
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command: %v", c),
		}
	}
	return cliResponse{}
}
