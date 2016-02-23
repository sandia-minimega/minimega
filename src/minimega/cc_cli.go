// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"ron"
	"sort"
	"strconv"
	"strings"
)

var (
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
Command and control for virtual machines running the miniccc client. Commands
may include regular commands, backgrounded commands, and any number of sent
and/or received files. Commands will be executed in command creation order. For
example, to send a file 'foo' and display the contents on a remote VM:

	cc send foo
	cc exec cat foo

Files to be sent must be in the filepath directory, as set by the -filepath
flag when launching minimega.

Responses are organized in a structure within <filepath>/miniccc_responses, and
include subdirectories for each client response named by the client's UUID.
Responses can also be displayed on the command line with the 'responses'
command.

Filters may be set to limit which clients may execute a posted command.  For
example, to filter on VMs that are running windows and have a specific IP.

	cc filter os=windows ip=10.0.0.1

When a namespace is active, there is an implicit filter for vms with the
provided namespace.

For more documentation, see the article "Command and Control API Tutorial".`,
		Patterns: []string{
			"cc",
			"cc <clients,>",

			"cc <prefix,> [prefix]",

			"cc <send,> <file>...",
			"cc <recv,> <file>...",
			"cc <exec,> <command>...",
			"cc <background,> <command>...",

			"cc <process,> <list,> <vm id, name, uuid or all>",
			"cc <process,> <kill,> <pid>",

			"cc <commands,>",

			"cc <filter,> [filter]...",

			"cc <responses,> <id or prefix or all> [raw,]",

			"cc <tunnel,> <uuid> <src port> <host> <dst port>",
			"cc <rtunnel,> <src port> <host> <dst port>",

			"cc <delete,> <command,> <id or prefix or all>",
			"cc <delete,> <response,> <id or prefix or all>",
		},
		Call: wrapBroadcastCLI(cliCC),
	},
	{ // clear cc
		HelpShort: "reset command and control state",
		HelpLong: `
Resets state for the command and control infrastructure provided by minimega.
See "help cc" for more information.`,
		Patterns: []string{
			"clear cc",
			"clear cc <commands,>",
			"clear cc <filter,>",
			"clear cc <prefix,>",
			"clear cc <responses,>",
		},
		Call: wrapBroadcastCLI(cliCCClear),
	},
}

// Functions pointers to the various handlers for the subcommands
var ccCliSubHandlers = map[string]func(*minicli.Command) *minicli.Response{
	"responses":  cliCCResponses,
	"commands":   cliCCCommand,
	"filter":     cliCCFilter,
	"send":       cliCCFileSend,
	"recv":       cliCCFileRecv,
	"exec":       cliCCExec,
	"background": cliCCBackground,
	"prefix":     cliCCPrefix,
	"delete":     cliCCDelete,
	"clients":    cliCCClients,
	"tunnel":     cliCCTunnel,
	"rtunnel":    cliCCTunnel,
	"process":    cliCCProcess,
}

func cliCC(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	// Ensure that cc is running before proceeding
	if ccNode == nil {
		resp.Error = "cc service not running"
		return resp
	}

	if len(c.BoolArgs) > 0 {
		// Invoke a particular handler
		for k, fn := range ccCliSubHandlers {
			if c.BoolArgs[k] {
				log.Debug("cc handler %v", k)
				return fn(c)
			}
		}
	} else {
		// Getting status
		clients := ccNode.GetActiveClients()

		resp.Header = []string{"number of clients"}
		resp.Tabular = [][]string{
			[]string{
				fmt.Sprintf("%v", len(clients)),
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

// tunnel
func cliCCTunnel(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	src, err := strconv.Atoi(c.StringArgs["src"])
	if err != nil {
		resp.Error = fmt.Sprintf("non-integer src: %v : %v", c.StringArgs["src"], err)
		return resp
	}

	host := c.StringArgs["host"]

	dst, err := strconv.Atoi(c.StringArgs["dst"])
	if err != nil {
		resp.Error = fmt.Sprintf("non-integer dst: %v : %v", c.StringArgs["dst"], err)
		return resp
	}

	if c.BoolArgs["rtunnel"] {
		err := ccNode.Reverse(ccFilter, src, host, dst)
		if err != nil {
			resp.Error = err.Error()
		}
	} else {
		err := ccNode.Forward(c.StringArgs["uuid"], src, host, dst)
		if err != nil {
			resp.Error = err.Error()
		}
	}

	return resp
}

// responses
func cliCCResponses(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	raw := c.BoolArgs["raw"]
	id := c.StringArgs["id"]

	var files []string

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
			log.Debug("add to response files: %v", path)
		}
		return nil
	}

	if id == Wildcard {
		// all responses
		err := filepath.Walk(filepath.Join(*f_iomBase, ron.RESPONSE_PATH), walker)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else if _, err := strconv.Atoi(id); err == nil {
		p := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, id)
		_, err := os.Stat(p)
		if err != nil {
			resp.Error = fmt.Sprintf("no such response dir %v", p)
			return resp
		}
		err = filepath.Walk(p, walker)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
	} else {
		// try a prefix. First, do we even have anything with this prefix?
		ids := ccPrefixIDs(id)
		if len(ids) == 0 {
			resp.Error = fmt.Sprintf("no such prefix %v", id)
			return resp
		}

		var totalFiles []string
		for _, i := range ids {
			p := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", i))
			_, err := os.Stat(p)
			if err != nil {
				resp.Error = fmt.Sprintf("no such response dir %v", p)
				return resp
			}
			err = filepath.Walk(p, walker)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			totalFiles = append(totalFiles, files...)
			files = []string{}
		}
		files = totalFiles
	}

	// now output files
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}
		if !raw {
			path, err := filepath.Rel(filepath.Join(*f_iomBase, ron.RESPONSE_PATH), file)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			resp.Response += fmt.Sprintf("%v:\n", path)
		}
		resp.Response += fmt.Sprintf("%v\n", string(data))
	}

	return resp
}

// filter
func cliCCFilter(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if len(c.ListArgs["filter"]) == 0 {
		// Summary of current filter
		if ccFilter != nil {
			resp.Header = []string{"UUID", "hostname", "arch", "OS", "IP", "MAC"}
			resp.Tabular = append(resp.Tabular, []string{
				ccFilter.UUID,
				ccFilter.Hostname,
				ccFilter.Arch,
				ccFilter.OS,
				fmt.Sprintf("%v", ccFilter.IP),
				fmt.Sprintf("%v", ccFilter.MAC),
			})
		}
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

	// Set implicit filter
	ccFilter.Namespace = namespace

	cmd := &ron.Command{
		Filter: ccFilter,
	}

	// Add new files to send, expand globs
	for _, fglob := range c.ListArgs["file"] {
		files, err := filepath.Glob(filepath.Join(*f_iomBase, fglob))
		if err != nil {
			resp.Error = fmt.Sprintf("non-existent files %v", fglob)
			return resp
		}

		if len(files) == 0 {
			resp.Error = fmt.Sprintf("no such file %v", fglob)
			return resp
		}

		for _, f := range files {
			file, err := filepath.Rel(*f_iomBase, f)
			if err != nil {
				resp.Error = fmt.Sprintf("parsing filesend: %v", err)
				return resp
			}
			fi, err := os.Stat(f)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}
			perm := fi.Mode() & os.ModePerm
			cmd.FilesSend = append(cmd.FilesSend, &ron.File{
				Name: file,
				Perm: perm,
			})
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

	// Set implicit filter
	ccFilter.Namespace = namespace

	cmd := &ron.Command{
		Filter: ccFilter,
	}

	// Add new files to receive
	for _, file := range c.ListArgs["file"] {
		cmd.FilesRecv = append(cmd.FilesRecv, &ron.File{
			Name: file,
		})
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// background (just exec with background==true)
func cliCCBackground(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Set implicit filter
	ccFilter.Namespace = namespace

	cmd := &ron.Command{
		Background: true,
		Command:    c.ListArgs["command"],
		Filter:     ccFilter,
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return resp
}

// process
func cliCCProcess(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if c.BoolArgs["kill"] {
		pid, err := strconv.Atoi(c.StringArgs["pid"])
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		cmd := &ron.Command{
			PID:    pid,
			Filter: ccFilter,
		}

		id := ccNode.NewCommand(cmd)
		log.Debug("generated command %v :%v", id, cmd)

		ccMapPrefix(id)
	} else {
		// list processes

		v := c.StringArgs["vm"]

		var activeVms []string

		if v == Wildcard {
			clients := ccNode.GetActiveClients()
			for _, client := range clients {
				activeVms = append(activeVms, client.UUID)
			}
		} else {
			// get the vm uuid
			vm := vms.findVm(v)
			if vm == nil {
				resp.Error = vmNotFound(v).Error()
				return resp
			}
			log.Debug("got vm: %v %v", vm.GetID(), vm.GetName())
			activeVms = []string{vm.GetUUID()}
		}

		resp.Header = []string{"name", "uuid", "pid", "command"}
		for _, uuid := range activeVms {
			vm := vms.findVm(uuid)
			if vm == nil {
				resp.Error = vmNotFound(v).Error()
				return resp
			}

			processes, err := ccNode.GetProcesses(uuid)
			if err != nil {
				resp.Error = err.Error()
				return resp
			}

			for _, p := range processes {
				resp.Tabular = append(resp.Tabular, []string{
					vm.GetName(),
					vm.GetUUID(),
					fmt.Sprintf("%v", p.PID),
					strings.Join(p.Command, " "),
				})
			}
		}
	}

	return resp
}

// exec
func cliCCExec(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	// Set implicit filter
	ccFilter.Namespace = namespace

	cmd := &ron.Command{
		Command: c.ListArgs["command"],
		Filter:  ccFilter,
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

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
			err := ccClear("commands")
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
	} else if c.BoolArgs["response"] {
		id := c.StringArgs["id"]

		if id == Wildcard {
			err := ccClear("responses")
			if err != nil {
				resp.Error = fmt.Sprintf("delete response %v: %v", Wildcard, err)
			}
			return resp
		}

		// attemp to delete by prefix
		ids := ccPrefixIDs(id)
		if len(ids) != 0 {
			for _, v := range ids {
				path := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", v))
				err := os.RemoveAll(path)
				if err != nil {
					resp.Error = fmt.Sprintf("cc delete response %v: %v", v, err)
					return resp
				}
			}
			return resp
		}

		_, err := strconv.Atoi(id)
		if err != nil {
			resp.Error = fmt.Sprintf("no such id or prefix %v", id)
			return resp
		}

		path := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", id))

		err = os.RemoveAll(path)
		if err != nil {
			resp.Error = fmt.Sprintf("cc delete response %v: %v", id, err)
			return resp
		}
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
