// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>
//

// minimega dhcp server support
package main

// TODO: add support for killing dhcp servers

import (
//	"bytes"
//	"fmt"
//	log "minilog"
//	"os/exec"
	"strconv"
)

type dhcpServer struct {
	Addr string
	MinRange string
	MaxRange string
	Path string
}

var (
	dhcpServers map[int]*dhcpServer
	dhcpServerCount int
)

// generate paths for the leases and pid files (should be unique) so we can support multiple dhcp servers
// maintain a map of dhcp servers that can be listed
// allow killing dhcp servers with dhcp kill 

func dhcpCLI(c cli_command) cli_response {
	var ret cli_response
	switch len(c.Args) {
	case 0:
		// show the list of dhcp servers
		ret.Response = dhcpList()
	case 2:
		if c.Args[0] != "kill" {
			ret.Error = "malformed command"
			break
		}
		val, err := strconv.Atoi(c.Args[1])
		if err != nil {
			ret.Error = err.Error()
			break
		}
		err = dhcpKill(val)
		if err != nil {
			ret.Error = err.Error()
		}
	case 3:
		err := dhcpStart(c.Args[0], c.Args[1], c.Args[2])
		if err != nil {
			ret.Error = err.Error()
		}
	default:
		ret.Error = "malformed command"
	}
	return ret
}

func dhcpList() string {
	return ""
}

func dhcpKill(id int) error {
	return nil
}

func dhcpStart(ip, min, max string) error {
	return nil
}
