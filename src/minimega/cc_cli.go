// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"path/filepath"
	"ron"
	"sort"
	"strconv"
	"strings"
)

var (
	ccSerial    bool
	ccFilter    *ron.Client
	ccPrefix    string
	ccPrefixMap map[int]string
)

func init() {
	ccPrefixMap = make(map[int]string)
}

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
			"cc <serial,>",
			"cc <clients,>",

			"cc <prefix,> [prefix]",

			"cc <send,> <file>...",
			"cc <recv,> <file>...",
			"cc <exec,> <command>...",
			"cc <background,> <command>...",

			"cc <command,>",

			"cc <filter,> [filter]...",

			//	"cc <response,> <id or prefix or all>",

			"cc <delete,> <command,> <id or prefix or all>",
			//	"cc <delete,> <response,> <id or prefix or all>",
		},
		Call: wrapSimpleCLI(cliCC),
	},
	{ // clear cc
		HelpShort: "reset command and control state",
		HelpLong: `
Resets state for the command and control infrastructure provided by minimega.
See "help cc" for more information.`,
		Patterns: []string{
			"clear cc",
			"clear cc <command,>",
			"clear cc <filter,>",
			"clear cc <prefix,>",
			//			"clear cc <response,>",
		},
		Call: wrapSimpleCLI(cliCCClear),
	},
}

// Functions pointers to the various handlers for the subcommands
var ccCliSubHandlers = map[string]func(*minicli.Command) *minicli.Response{
	//	"response":	cliCCResponse,
	"command":    cliCCCommand,
	"filter":     cliCCFilter,
	"send":       cliCCFileSend,
	"recv":       cliCCFileRecv,
	"exec":       cliCCExec,
	"background": cliCCBackground,
	"serial":     cliCCSerial,
	"prefix":     cliCCPrefix,
	"delete":     cliCCDelete,
	"clients":    cliCCClients,
}

func init() {
	registerHandlers("cc", ccCLIHandlers)
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

	if len(c.BoolArgs) > 0 {
		// Invoke a particular handler
		for k, fn := range ccCliSubHandlers {
			if c.BoolArgs[k] {
				return fn(c)
			}
		}
	} else {
		// Getting status
		port := ccNode.GetPort()
		clients := ccNode.GetActiveClients()

		resp.Header = []string{"port", "number of clients", "serial active"}
		resp.Tabular = [][]string{
			[]string{
				strconv.Itoa(port),
				fmt.Sprintf("%v", len(clients)),
				strconv.FormatBool(ccSerial),
			},
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

// prefix
func cliCCPrefix(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	prefix, ok := c.StringArgs["prefix"]

	if !ok {
		resp.Response = ccPrefix
		return resp
	} else {
		ccPrefix = prefix
	}

	return resp
}

// filter
func cliCCFilter(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.ListArgs["filter"]) == 0 {
		// Summary of current filter

		resp.Header = []string{"UUID", "hostname", "arch", "OS", "IP", "MAC"}
		resp.Tabular = append(resp.Tabular, []string{
			ccFilter.UUID,
			ccFilter.Hostname,
			ccFilter.Arch,
			ccFilter.OS,
			fmt.Sprintf("%v", ccFilter.IP),
			fmt.Sprintf("%v", ccFilter.MAC),
		})
	} else {
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

		ccFilter = filter
	}

	return resp
}

// send
func cliCCFileSend(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	cmd := &ron.Command{
		Record: true,
		Filter: []*ron.Client{ccFilter},
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
			cmd.FilesSend = append(cmd.FilesSend, file)
		}
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// recv
func cliCCFileRecv(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	cmd := &ron.Command{
		Record: true,
		Filter: []*ron.Client{ccFilter},
	}

	// Add new files to receive
	for _, file := range c.ListArgs["file"] {
		cmd.FilesRecv = append(cmd.FilesRecv, file)
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// background (just exec with background==true)
func cliCCBackground(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	cmd := &ron.Command{
		Record:     true,
		Filter:     []*ron.Client{ccFilter},
		Background: true,
		Command:    c.ListArgs["command"],
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// exec
func cliCCExec(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	cmd := &ron.Command{
		Record:  true,
		Filter:  []*ron.Client{ccFilter},
		Command: c.ListArgs["command"],
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// serial
func cliCCSerial(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if ccSerial {
		resp.Error = "cc serial service already running"
		return resp
	}

	ccSerial = true
	go ccSerialWatcher()

	return resp
}

// clients
func cliCCClients(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	resp.Header = []string{
		"UUID", "hostname", "arch", "OS",
		"IP", "MAC",
	}
	resp.Tabular = [][]string{}

	clients := ccNode.GetActiveClients()

	var uuids []string
	for k, _ := range clients {
		uuids = append(uuids, k)
	}
	sort.Strings(uuids)

	for _, i := range uuids {
		v := clients[i]
		row := []string{
			v.UUID,
			v.Hostname,
			v.Arch,
			v.OS,
			fmt.Sprintf("%v", v.IP),
			fmt.Sprintf("%v", v.MAC),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return resp
}

// command
func cliCCCommand(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	resp.Header = []string{
		"ID", "prefix", "command", "responses", "background",
		"send files", "receive files", "filter",
	}
	resp.Tabular = [][]string{}

	var commandIDs []int
	commands := ccNode.GetCommands()
	for k, _ := range commands {
		commandIDs = append(commandIDs, k)
	}
	sort.Ints(commandIDs)

	for _, i := range commandIDs {
		v := commands[i]
		row := []string{
			strconv.Itoa(v.ID),
			ccPrefixMap[i],
			fmt.Sprintf("%v", v.Command),
			strconv.Itoa(len(v.CheckedIn)),
			strconv.FormatBool(v.Background),
			fmt.Sprintf("%v", v.FilesSend),
			fmt.Sprintf("%v", v.FilesRecv),
			filterString(v.Filter),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return resp
}

func cliCCDelete(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["command"] {
		id := c.StringArgs["id"]

		if id == Wildcard {
			// delete all commands, same as 'clear cc command'
			err := ccClear("command")
			if err != nil {
				resp.Error = fmt.Sprintf("delete command %v: %v", Wildcard, err)
			}
			return resp
		}

		// attempt to delete by prefix
		ids := ccPrefixIDs(id)
		if len(ids) != 0 {
			for _, v := range ids {
				err := ccNode.DeleteCommand(v)
				if err != nil {
					resp.Error = fmt.Sprintf("cc delete command %v : %v", v, err)
					return resp
				}
				ccUnmapPrefix(v)
			}
			return resp
		}

		val, err := strconv.Atoi(id)
		if err != nil {
			resp.Error = fmt.Sprintf("no such id or prefix %v", id)
			return resp
		}

		err = ccNode.DeleteCommand(val)
		if err != nil {
			resp.Error = fmt.Sprintf("cc delete command %v : %v", val, err)
			return resp
		}
		ccUnmapPrefix(val)
	}

	return resp
}

func cliCCClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	// Ensure that cc is running before proceeding
	if ccNode == nil {
		resp.Error = "cc service not running"
		return resp
	}

	for k := range ccCliSubHandlers {
		// We only want to clear something if it was specified on the
		// command line or if we're clearing everything (nothing was
		// specified).
		if c.BoolArgs[k] || len(c.BoolArgs) == 0 {
			err = ccClear(k)
			if err != nil {
				break
			}
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
