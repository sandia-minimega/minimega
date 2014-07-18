// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"ron"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

const (
	CC_PORT = 9002
)

var (
	ccNode        *ron.Ron
	ccFilters     map[int]*ron.Client
	ccFilterCount int
)

//cc layer syntax should look like:
//
//cc start [port]
//cc command [new <command> [norecord] [background], delete <command id>]
//cc file <send,receive> <file> [<file>,...]
//cc filter [add [uuid=<uuid>,...], delete <filter id>, clear]
//cc responses [command id]
//...
//UUID      string
//Hostname  string
//Arch      string
//OS        string
//IP        []string
//MAC       []string

func init() {
	ccFilters = make(map[int]*ron.Client)
}

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
	case "filter":
		return ccProcessFilters(c)
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command: %v", c),
		}
	}
	return cliResponse{}
}

func ccProcessFilters(c cliCommand) cliResponse {
	if len(c.Args) == 1 {
		// summary
		var ids []int
		for i, _ := range ccFilters {
			ids = append(ids, i)
		}
		sort.Ints(ids)

		var o bytes.Buffer
		w := new(tabwriter.Writer)
		w.Init(&o, 5, 0, 1, ' ', 0)
		fmt.Fprintf(w, "ID\tUUID\thostname\tarch\tOS\tIP\tMAC\n")
		for _, i := range ids {
			cl := ccFilters[i]
			fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\t%v\n", i, cl.UUID, cl.Hostname, cl.Arch, cl.OS, cl.IP, cl.MAC)
		}

		w.Flush()

		return cliResponse{
			Response: o.String(),
		}
	}

	if len(c.Args) < 2 {
		return cliResponse{
			Error: fmt.Sprintf("malformed command: %v", c),
		}
	}

	switch c.Args[1] {
	case "add":
		if len(c.Args) < 3 {
			return cliResponse{
				Error: fmt.Sprintf("malformed command: %v", c),
			}
		}

		// the rest of the fields should id=value pairs
		client := &ron.Client{}
		for _, v := range c.Args[2:] {
			s := strings.Split(v, "=")
			if len(s) != 2 {
				return cliResponse{
					Error: fmt.Sprintf("malformed id=value pair: %v", v),
				}
			}

			switch strings.ToLower(s[0]) {
			case "uuid":
				client.UUID = strings.ToLower(s[1])
			case "hostname":
				client.Hostname = s[1]
			case "arch":
				client.Arch = s[1]
			case "os":
				client.OS = s[1]
			case "ip":
				client.IP = append(client.IP, s[1])
			case "mac":
				client.MAC = append(client.MAC, s[1])
			default:
				return cliResponse{
					Error: fmt.Sprintf("no such filter field %v", s[0]),
				}
			}
		}
		id := ccFilterCount
		ccFilterCount++
		ccFilters[id] = client
	case "delete":
		if len(c.Args) < 3 {
			return cliResponse{
				Error: fmt.Sprintf("malformed command: %v", c),
			}
		}

		val, err := strconv.Atoi(c.Args[2])
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("malformed id: %v : %v", c.Args[2], err),
			}
		}

		if _, ok := ccFilters[val]; !ok {
			return cliResponse{
				Error: fmt.Sprintf("invalid filter id: %v", val),
			}
		}

		delete(ccFilters, val)
	case "clear":
		ccFilters = make(map[int]*ron.Client)
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
