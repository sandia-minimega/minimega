// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"errors"
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
			"cc <process,> <kill,> <pid or all>",

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
var ccCliSubHandlers = map[string]func(*minicli.Command, *minicli.Response) error{
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

func cliCC(c *minicli.Command, resp *minicli.Response) error {
	// Ensure that cc is running before proceeding
	if ccNode == nil {
		return errors.New("cc service not running")
	}

	if len(c.BoolArgs) > 0 {
		// Invoke a particular handler
		for k, fn := range ccCliSubHandlers {
			if c.BoolArgs[k] {
				log.Debug("cc handler %v", k)
				return fn(c, resp)
			}
		}

		return errors.New("unreachable")
	}

	// Getting status
	clients := ccNode.GetActiveClients()

	resp.Header = []string{"number of clients"}
	resp.Tabular = [][]string{
		[]string{
			fmt.Sprintf("%v", len(clients)),
		},
	}

	return nil
}

// prefix
func cliCCPrefix(c *minicli.Command, resp *minicli.Response) error {
	if prefix, ok := c.StringArgs["prefix"]; ok {
		ccPrefix = prefix
		return nil
	}

	resp.Response = ccPrefix
	return nil
}

// tunnel
func cliCCTunnel(c *minicli.Command, resp *minicli.Response) error {
	src, err := strconv.Atoi(c.StringArgs["src"])
	if err != nil {
		return fmt.Errorf("non-integer src: %v : %v", c.StringArgs["src"], err)
	}

	host := c.StringArgs["host"]

	dst, err := strconv.Atoi(c.StringArgs["dst"])
	if err != nil {
		return fmt.Errorf("non-integer dst: %v : %v", c.StringArgs["dst"], err)
	}

	if c.BoolArgs["rtunnel"] {
		return ccNode.Reverse(ccGetFilter(), src, host, dst)
	}

	return ccNode.Forward(c.StringArgs["uuid"], src, host, dst)
}

// responses
func cliCCResponses(c *minicli.Command, resp *minicli.Response) error {
	raw := c.BoolArgs["raw"]
	id := c.StringArgs["id"]

	var files []string

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Test if the file looks like a UUID. If it does, and a namespace is
		// active, check whether the VM is part of the active namespace. This
		// is a fairly naive way to filter the responses...
		if namespace != "" && isUUID(info.Name()) {
			if vm := vms.FindVM(info.Name()); vm == nil {
				log.Debug("skipping VM: %v", info.Name())
				return filepath.SkipDir
			}
		}

		if !info.IsDir() {
			log.Debug("add to response files: %v", path)
			files = append(files, path)
		}
		return nil
	}

	if id == Wildcard {
		// all responses
		return filepath.Walk(filepath.Join(*f_iomBase, ron.RESPONSE_PATH), walker)
	} else if _, err := strconv.Atoi(id); err == nil {
		p := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, id)
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("no such response dir %v", p)
		}

		return filepath.Walk(p, walker)
	}

	// try a prefix. First, do we even have anything with this prefix?
	ids := ccPrefixIDs(id)
	if len(ids) == 0 {
		return fmt.Errorf("no such prefix %v", id)
	}

	for _, i := range ids {
		p := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", i))
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("no such response dir %v", p)
		}
		if err := filepath.Walk(p, walker); err != nil {
			return err
		}
	}

	// now output files
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		if !raw {
			path, err := filepath.Rel(filepath.Join(*f_iomBase, ron.RESPONSE_PATH), file)
			if err != nil {
				return err
			}
			resp.Response += fmt.Sprintf("%v:\n", path)
		}
		resp.Response += fmt.Sprintf("%v\n", string(data))
	}

	return nil
}

// filter
func cliCCFilter(c *minicli.Command, resp *minicli.Response) error {
	if len(c.ListArgs["filter"]) > 0 {
		filter := &ron.Client{}

		// Process the id=value pairs
		for _, v := range c.ListArgs["filter"] {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("malformed id=value pair: %v", v)
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
			case "tag":
				// Explicit filter on tag
				parts = parts[1:]
				fallthrough
			default:
				// Implicit filter on a tag
				if filter.Tags == nil {
					filter.Tags = make(map[string]string)
				}

				// Split on `=` or `:` -- who cares if they did `tag=foo=bar`,
				// `tag=foo:bar` or `foo=bar`. `=` takes precedence.
				if strings.Contains(parts[0], "=") {
					parts = strings.SplitN(parts[0], "=", 2)
				} else if strings.Contains(parts[0], ":") {
					parts = strings.SplitN(parts[0], ":", 2)
				}

				if len(parts) == 1 {
					filter.Tags[parts[0]] = ""
				} else if len(parts) == 2 {
					filter.Tags[parts[0]] = parts[1]
				}
			}
		}

		ccFilter = filter
		return nil
	}

	// Summary of current filter
	if ccFilter != nil {
		resp.Header = []string{"UUID", "hostname", "arch", "OS", "IP", "MAC", "Tags"}
		row := []string{
			ccFilter.UUID,
			ccFilter.Hostname,
			ccFilter.Arch,
			ccFilter.OS,
			fmt.Sprintf("%v", ccFilter.IP),
			fmt.Sprintf("%v", ccFilter.MAC),
		}

		// encode the tags using JSON
		tags, err := json.Marshal(ccFilter.Tags)
		if err != nil {
			log.Warn("Unable to json marshal tags: %v", err)
		} else if ccFilter.Tags == nil {
			tags = []byte("{}")
		}
		row = append(row, string(tags))

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

// send
func cliCCFileSend(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Filter: ccGetFilter(),
	}

	// Add new files to send, expand globs
	for _, fglob := range c.ListArgs["file"] {
		files, err := filepath.Glob(filepath.Join(*f_iomBase, fglob))
		if err != nil {
			return fmt.Errorf("non-existent files %v", fglob)
		}

		if len(files) == 0 {
			return fmt.Errorf("no such file %v", fglob)
		}

		for _, f := range files {
			file, err := filepath.Rel(*f_iomBase, f)
			if err != nil {
				return fmt.Errorf("parsing filesend: %v", err)
			}
			fi, err := os.Stat(f)
			if err != nil {
				return err
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

	return nil
}

// recv
func cliCCFileRecv(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Filter: ccGetFilter(),
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

	return nil
}

// background (just exec with background==true)
func cliCCBackground(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Background: true,
		Command:    c.ListArgs["command"],
		Filter:     ccGetFilter(),
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return nil
}

// ccProcessKill kills a process by PID for VMs that aren't filtered.
func ccProcessKill(pid int) {
	cmd := &ron.Command{
		PID:    pid,
		Filter: ccGetFilter(),
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v :%v", id, cmd)

	ccMapPrefix(id)
}

func cliCCProcessKill(c *minicli.Command, resp *minicli.Response) error {
	// kill all processes
	if c.StringArgs["pid"] == Wildcard {
		ccProcessKill(-1)

		return nil
	}

	// kill single process
	pid, err := strconv.Atoi(c.StringArgs["pid"])
	if err != nil {
		return err
	}

	ccProcessKill(pid)

	return nil
}

// process
func cliCCProcess(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["kill"] {
		return cliCCProcessKill(c, resp)
	}

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
		vm := vms.FindVM(v)
		if vm == nil {
			return vmNotFound(v)
		}
		log.Debug("got vm: %v %v", vm.GetID(), vm.GetName())
		activeVms = []string{vm.GetUUID()}
	}

	resp.Header = []string{"name", "uuid", "pid", "command"}
	for _, uuid := range activeVms {
		vm := vms.FindVM(uuid)
		if vm == nil {
			return vmNotFound(v)
		}

		processes, err := ccNode.GetProcesses(uuid)
		if err != nil {
			return err
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

	return nil
}

// exec
func cliCCExec(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Command: c.ListArgs["command"],
		Filter:  ccGetFilter(),
	}

	id := ccNode.NewCommand(cmd)
	log.Debug("generated command %v : %v", id, cmd)

	ccMapPrefix(id)

	return nil
}

// clients
func cliCCClients(c *minicli.Command, resp *minicli.Response) error {
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

	return nil
}

// command
func cliCCCommand(c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"ID", "prefix", "command", "responses", "background",
		"send files", "receive files", "filter",
	}
	resp.Tabular = [][]string{}

	var commandIDs []int
	commands := ccNode.GetCommands()
	for k, v := range commands {
		// only show commands for the active namespace
		if !ccMatchNamespace(v) {
			continue
		}

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

	return nil
}

func cliCCDelete(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["command"] {
		id := c.StringArgs["id"]

		if id == Wildcard {
			// delete all commands, same as 'clear cc command'
			return ccClear("commands")
		}

		// attempt to delete by prefix
		ids := ccPrefixIDs(id)
		if len(ids) != 0 {
			for _, v := range ids {
				c := ccNode.GetCommand(v)
				if c == nil {
					return fmt.Errorf("cc delete unknown command %v", v)
				}

				if !ccMatchNamespace(c) {
					// skip without warning
					continue
				}

				err := ccNode.DeleteCommand(v)
				if err != nil {
					return fmt.Errorf("cc delete command %v : %v", v, err)
				}
				ccUnmapPrefix(v)
			}

			return nil
		}

		val, err := strconv.Atoi(id)
		if err != nil {
			return fmt.Errorf("no such id or prefix %v", id)
		}

		c := ccNode.GetCommand(val)
		if c == nil {
			return fmt.Errorf("cc delete unknown command %v", val)
		}

		if !ccMatchNamespace(c) {
			return fmt.Errorf("cc command not part of active namespace")
		}

		if err := ccNode.DeleteCommand(val); err != nil {
			return fmt.Errorf("cc delete command %v: %v", val, err)
		}
		ccUnmapPrefix(val)
	} else if c.BoolArgs["response"] {
		id := c.StringArgs["id"]

		if id == Wildcard {
			return ccClear("responses")
		}

		// attemp to delete by prefix
		ids := ccPrefixIDs(id)
		if len(ids) != 0 {
			for _, v := range ids {
				path := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", v))
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf("cc delete response %v: %v", v, err)
				}
			}

			return nil
		}

		if _, err := strconv.Atoi(id); err != nil {
			return fmt.Errorf("no such id or prefix %v", id)
		}

		path := filepath.Join(*f_iomBase, ron.RESPONSE_PATH, fmt.Sprintf("%v", id))

		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("cc delete response %v: %v", id, err)
		}
	}

	return nil
}

func cliCCClear(c *minicli.Command, resp *minicli.Response) error {
	// Ensure that cc is running before proceeding
	if ccNode == nil {
		return errors.New("cc service not running")
	}

	for k := range ccCliSubHandlers {
		// We only want to clear something if it was specified on the
		// command line or if we're clearing everything (nothing was
		// specified).
		if c.BoolArgs[k] || len(c.BoolArgs) == 0 {
			if err := ccClear(k); err != nil {
				return err
			}
		}
	}

	return nil
}
