// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"minicli"
	log "minilog"
	"path/filepath"
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
//cc command [new [norecord] [background] [command=<command>] [filesend=<filename>, ...] [filerecv=<filename>, ...], delete <command id>]
//cc filter [add [uuid=<uuid>,...], delete <filter id>, clear]
//cc responses [command id]
//...
//UUID      string
//Hostname  string
//Arch      string
//OS        string
//IP        []string
//MAC       []string

var ccCLIHandlers = []minicli.Handler{
	{ // cc
		HelpShort: "command and control commands",
		HelpLong: `
Command and control virtual machines running the miniccc client. Commands may
include regular commands, backgrounded commands, and any number of sent and/or
received files. Commands will be executed in command creation order. For
example, to send a file 'foo' and display the contents on a remote VM:

	cc command new command="cat foo" filesend=foo

Responses are generated (unless the 'norecord' flag is set) and written out to
'<filebase>/miniccc_responses/<command id>/<client UUID>'. Files to be sent
must be in '<filebase>'.

Filters may be set to limit which clients may execute a posted command. Filters
are the logical sum of products of every filter added. That is, a single given
filter must match all given fields for the command to be executed. Multiple
filters are allowed, in which case any matched filter will allow the command to
execute. For example, to filter on VMs that are running windows AND have a
specific IP, OR nodes that have a range of IPs:

	cc filter add os=windows ip=10.0.0.1 cc filter add ip=12.0.0.0/24

New commands assign any current filters.`,
		Patterns: []string{
			"cc",
			"clear cc",

			"cc start [port]",

			"cc <filter,> <filters>...",
			"cc <filesend,> <file>...",
			"cc <filerecv,> <file>...",
			"cc <command,> <command>...",
			"cc <exec,> [background,]",

			"cc <delete,> <filter,> <id or *>",
			"cc <delete,> <filesend,> <id or *>",
			"cc <delete,> <filerecv,> <id or *>",
			"cc <delete,> <command,> <id or *>",

			"clear cc <filter,>",
			"clear cc <filesend,>",
			"clear cc <filerecv,>",
			"clear cc <command,>",
		},
		Call: nil, // TODO: cliCC,
	},
}

func init() {
	registerHandlers("cc", ccCLIHandlers)

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
		if ccNode != nil {
			return cliResponse{
				Error: "cc service already running",
			}
		}

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
		ccNode, err = ron.New(port, ron.MODE_MASTER, "", *f_iomBase)
		if err != nil {
			return cliResponse{
				Error: fmt.Sprintf("creating cc node %v", err),
			}
		}
		log.Debug("created ron node at %v %v", port, *f_base)
	case "command":
		return ccProcessCommand(c)
	case "filter":
		return ccProcessFilters(c)
	default:
		return cliResponse{
			Error: fmt.Sprintf("malformed command: %v", c),
		}
	}
	return cliResponse{}
}

func ccProcessCommand(c cliCommand) cliResponse {
	if len(c.Args) == 1 {
		// command summary
		return cliResponse{
			Response: ccNode.CommandSummary(),
		}
	}

	switch c.Args[1] {
	case "new":
		if len(c.Args) < 3 {
			return cliResponse{
				Error: fmt.Sprintf("malformed command: %v", c),
			}
		}

		cmd := &ron.Command{
			Record: true,
		}
		for _, cl := range ccFilters {
			cmd.Filter = append(cmd.Filter, cl)
		}

		fields := fieldsQuoteEscape("\"", strings.Join(c.Args[2:], " "))
		log.Debug("got new cc command args: %#v", fields)

		for _, v := range fields {
			if v == "norecord" {
				cmd.Record = false
				continue
			}
			if v == "background" {
				cmd.Background = true
				continue
			}

			// everything else should be an id=value pair
			s := strings.SplitN(v, "=", 2)
			if len(s) != 2 {
				return cliResponse{
					Error: fmt.Sprintf("malformed id=value pair: %v", v),
				}
			}

			switch strings.ToLower(s[0]) {
			case "command":
				cmdFields := strings.Trim(s[1], `"`)
				f := fieldsQuoteEscape("'", cmdFields)
				var c []string
				for _, w := range f {
					c = append(c, strings.Trim(w, "'"))
				}
				log.Debug("command: %#v", c)
				cmd.Command = c
			case "filesend":
				files, err := filepath.Glob(filepath.Join(*f_iomBase, s[1]))
				if err != nil {
					return cliResponse{
						Error: fmt.Sprintf("non-existent files %v", s[1]),
					}
				}
				for _, f := range files {
					file, err := filepath.Rel(*f_iomBase, f)
					if err != nil {
						return cliResponse{
							Error: fmt.Sprintf("parsing filesend: %v", err),
						}
					}
					cmd.FilesSend = append(cmd.FilesSend, file)
				}
			case "filerecv":
				cmd.FilesRecv = append(cmd.FilesRecv, s[1])
			default:
				return cliResponse{
					Error: fmt.Sprintf("no such filter field %v", s[0]),
				}
			}
		}

		id := ccNode.NewCommand(cmd)
		log.Debug("generated command %v : %v", id, cmd)
	case "delete":
		if len(c.Args) != 3 {
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
	case "clear":
		c := ccNode.GetCommands()
		for _, v := range c {
			err := ccNode.DeleteCommand(v.ID)
			if err != nil {
				log.Warn("cc delete command %v : %v", v.ID, err)
			}
		}
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
			s := strings.SplitN(v, "=", 2)
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

func cliClearCC() error {
	ccFilters = make(map[int]*ron.Client)
	c := ccNode.GetCommands()
	for _, v := range c {
		err := ccNode.DeleteCommand(v.ID)
		if err != nil {
			log.Warn("cc delete command %v : %v", v.ID, err)
		}
	}
	return nil
}
