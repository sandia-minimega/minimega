// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"path/filepath"
	"ron"
	"sort"
	"strconv"
	"strings"
)

const (
	CC_PORT = 9002
)

var (
	ccNode       *ron.Ron
	ccBackground bool
	ccFileRecv   map[int]string
	ccFileSend   map[int]string
	ccFilters    map[int]*ron.Client

	ccFileRecvIDChan chan int
	ccFileSendIDChan chan int
	ccFilerIDChan    chan int
)

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
			"cc <start,> [port]",

			"cc <background,> [true,false]",
			"cc <filerecv,> [file]...",
			"cc <filesend,> [file]...",
			"cc <filter,> [filter]...",
			"cc <command,> <command>...",

			"cc <delete,> <filerecv,> <id or *>",
			"cc <delete,> <filesend,> <id or *>",
			"cc <delete,> <filter,> <id or *>",
			"cc <delete,> <command,> <id or *>",

			"clear cc",
			"clear cc <background,>",
			"clear cc <command,>",
			"clear cc <filerecv,>",
			"clear cc <filesend,>",
			"clear cc <filter,>",
		},
		Call: wrapSimpleCLI(cliCC),
	},
}

func init() {
	registerHandlers("cc", ccCLIHandlers)

	ccFileRecvIDChan = makeIDChan()
	ccFileSendIDChan = makeIDChan()
	ccFilerIDChan = makeIDChan()
}

func cliCC(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	if c.BoolArgs["start"] {
		err = ccStart(c.StringArgs["port"])
		if err != nil {
			resp.Error = err.Error()
		}

		return resp
	}

	// Ensure that cc is running before proceeding
	if ccNode == nil {
		resp.Error = "cc service not running"
		return resp
	}

	// Functions pointers to the various handlers for the subcommands
	handlers := map[string]func(*minicli.Command) *minicli.Response{
		"filter":     cliCCFilter,
		"filesend":   cliCCFileSend,
		"filerecv":   cliCCFileRecv,
		"command":    cliCCCommand,
		"background": cliCCBackground,
	}

	if isClearCommand(c) {
		for k := range handlers {
			// We only want to clear something if it was specified on the
			// command line or if we're clearing everything (nothing was
			// specified).
			if c.BoolArgs[k] || len(c.BoolArgs) == 0 {
				err = ccClear(k, "*")
				if err != nil {
					break
				}
			}
		}
	} else if c.BoolArgs["delete"] {
		delete(c.BoolArgs, "delete")
		// Deleting a specific ID, only one other BoolArgs should be set
		for k := range c.BoolArgs {
			err = ccClear(k, c.StringArgs["id"])
		}
	} else if len(c.BoolArgs) > 0 {
		// Invoke a particular handler
		for k, fn := range handlers {
			if c.BoolArgs[k] {
				return fn(c)
			}
		}
	} else {
		// Getting status
		port := ccNode.GetPort()
		clients := ccNode.GetActiveClients()

		resp.Header = []string{"port", "clients"}
		resp.Tabular = [][]string{
			[]string{
				strconv.Itoa(port),
				fmt.Sprintf("%v", clients),
			},
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func ccStart(portStr string) (err error) {
	port := CC_PORT
	if portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port: %v", portStr)
		}
	}

	ccNode, err = ron.New(port, ron.MODE_MASTER, "", *f_iomBase)
	if err != nil {
		return fmt.Errorf("creating cc node %v", err)
	}

	log.Debug("created ron node at %v %v", port, *f_base)
	return nil
}

func ccClear(what, idStr string) (err error) {
	log.Debug("cc clear -- %v:%v", what, idStr)
	var id int

	deleteAll := (idStr == "*")
	if !deleteAll {
		id, err = strconv.Atoi(idStr)
		if err != nil {
			return fmt.Errorf("invalid id %v", idStr)
		}
	}

	if deleteAll {
		switch what {
		case "filter":
			ccFilters = make(map[int]*ron.Client)
		case "filesend":
			ccFileSend = make(map[int]string)
		case "filerecv":
			ccFileRecv = make(map[int]string)
		case "background":
			ccBackground = false
		case "command":
			errs := []string{}
			for _, v := range ccNode.GetCommands() {
				err := ccNode.DeleteCommand(v.ID)
				if err != nil {
					errMsg := fmt.Sprintf("cc delete command %v : %v", v.ID, err)
					errs = append(errs, errMsg)
				}
			}
			err = errors.New(strings.Join(errs, "\n"))
		}
	} else {
		switch what {
		case "filter":
			if _, ok := ccFilters[id]; !ok {
				return fmt.Errorf("invalid filter id: %v", id)
			}
			delete(ccFilters, id)
		case "filesend":
			if _, ok := ccFileSend[id]; !ok {
				return fmt.Errorf("invalid file send id: %v", id)
			}
			delete(ccFileSend, id)
		case "filerecv":
			if _, ok := ccFileRecv[id]; !ok {
				return fmt.Errorf("invalid file recv id: %v", id)
			}
			delete(ccFileRecv, id)
		case "background":
			ccBackground = false
		case "command":
			err = ccNode.DeleteCommand(id)
		}
	}

	return
}

// Adding filter
func cliCCFilter(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.ListArgs["filter"]) == 0 {
		// Summary of current filters
		var ids []int
		for id := range ccFilters {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		resp.Header = []string{"ID", "UUID", "hostname", "arch", "OS", "IP", "MAC"}
		resp.Tabular = [][]string{}
		for _, id := range ids {
			filter := ccFilters[id]
			row := []string{
				strconv.Itoa(id),
				filter.UUID,
				filter.Hostname,
				filter.Arch,
				filter.OS,
				fmt.Sprintf("%v", filter.IP),
				fmt.Sprintf("%v", filter.MAC),
			}
			resp.Tabular = append(resp.Tabular, row)
		}
	} else {
		if ccFilters == nil {
			ccFilters = make(map[int]*ron.Client)
		}

		filter := &ron.Client{}
		// Process the id=value pairs
		for _, v := range c.ListArgs["filter"] {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				resp.Error = fmt.Sprintf("malformed id=value pair: %v", v)
				return resp
			}

			switch strings.ToLower(parts[0]) {
			case "uuid":
				filter.UUID = strings.ToLower(parts[1])
			case "hostname":
				filter.Hostname = parts[1]
			case "arch":
				filter.Arch = parts[1]
			case "os":
				filter.OS = parts[1]
			case "ip":
				filter.IP = append(filter.IP, parts[1])
			case "mac":
				filter.MAC = append(filter.MAC, parts[1])
			default:
				resp.Error = fmt.Sprintf("no such filter field %v", parts[0])
				return resp
			}
		}

		ccFilters[<-ccFilerIDChan] = filter
	}

	return resp
}

// Adding filesend
func cliCCFileSend(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.ListArgs["file"]) == 0 {
		// Summary of current file sends
		var ids []int
		for id := range ccFileSend {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		resp.Header = []string{"ID", "File"}
		resp.Tabular = [][]string{}
		for _, id := range ids {
			row := []string{
				strconv.Itoa(id),
				ccFileSend[id],
			}
			resp.Tabular = append(resp.Tabular, row)
		}
	} else {
		if ccFileSend == nil {
			ccFileSend = make(map[int]string)
		}

		// Add new files to send, expand globs
		for _, fglob := range c.ListArgs["file"] {
			files, err := filepath.Glob(filepath.Join(*f_iomBase, fglob))
			if err != nil {
				resp.Error = fmt.Sprintf("non-existent files %v", fglob)
				return resp
			}

			for _, f := range files {
				file, err := filepath.Rel(*f_iomBase, f)
				if err != nil {
					resp.Error = fmt.Sprintf("parsing filesend: %v", err)
					return resp
				}
				ccFileSend[<-ccFileSendIDChan] = file
			}
		}
	}

	return resp
}

// Adding filerecv
func cliCCFileRecv(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.ListArgs["file"]) == 0 {
		// Summary of current file recvs
		var ids []int
		for id := range ccFileSend {
			ids = append(ids, id)
		}
		sort.Ints(ids)

		resp.Header = []string{"ID", "File"}
		resp.Tabular = [][]string{}
		for _, id := range ids {
			row := []string{
				strconv.Itoa(id),
				ccFileSend[id],
			}
			resp.Tabular = append(resp.Tabular, row)
		}
	} else {
		if ccFileRecv == nil {
			ccFileRecv = make(map[int]string)
		}

		// Add new files to receive
		for _, file := range c.ListArgs["file"] {
			ccFileRecv[<-ccFileRecvIDChan] = file
		}
	}

	return resp
}

// Get/set whether cc command runs in the background
func cliCCBackground(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if !c.BoolArgs["true"] && !c.BoolArgs["false"] {
		resp.Response = strconv.FormatBool(ccBackground)
	} else {
		ccBackground = c.BoolArgs["true"]
	}

	return resp
}

// Setting command
func cliCCCommand(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	cmd := &ron.Command{
		Record:  true,
		Command: c.ListArgs["command"],
	}

	// Copy fields into cmd
	for _, filter := range ccFilters {
		cmd.Filter = append(cmd.Filter, filter)
	}
	for _, fsend := range ccFileSend {
		cmd.FilesSend = append(cmd.FilesSend, fsend)
	}
	for _, frecv := range ccFileRecv {
		cmd.FilesRecv = append(cmd.FilesRecv, frecv)
	}

	// TODO: Record flag?
	cmd.Background = ccBackground

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	resp.Response = fmt.Sprintf("started command, id: %v", id)
	return resp
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
