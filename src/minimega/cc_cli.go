// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"os"
	"path/filepath"
	"ron"
	"sort"
	"strconv"
	"strings"
)

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

Users can also filter by VM tags. For example, to filter on VMs that have the
tag with key foo and value bar set:

	cc filter tag=foo:bar

If users wish, they may drop the tag= prefix and key=value pairs will be
treated as tags:

	cc filter foo=bar

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

			"cc <process,> <list,> <vm name, uuid or all>",
			"cc <process,> <kill,> <pid or all>",
			"cc <process,> <killall,> <name>",

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
	// Dispatcher for a sub handler
	if len(c.BoolArgs) > 0 {
		for k, fn := range ccCliSubHandlers {
			if c.BoolArgs[k] {
				log.Debug("cc handler %v", k)
				return fn(c, resp)
			}
		}

		return errors.New("unreachable")
	}

	// If no sub handler, display the number of clients instead
	resp.Header = []string{"clients"}
	resp.Tabular = [][]string{
		[]string{
			strconv.Itoa(ccClients()),
		},
	}

	return nil
}

// tunnel
func cliCCTunnel(c *minicli.Command, resp *minicli.Response) error {
	src, err := strconv.Atoi(c.StringArgs["src"])
	if err != nil {
		return fmt.Errorf("non-integer src: %v : %v", c.StringArgs["src"], err)
	}

	dst, err := strconv.Atoi(c.StringArgs["dst"])
	if err != nil {
		return fmt.Errorf("non-integer dst: %v : %v", c.StringArgs["dst"], err)
	}

	host := c.StringArgs["host"]
	uuid := c.StringArgs["uuid"]
	reverse := c.BoolArgs["rtunnel"]

	return ccTunnel(host, uuid, src, dst, reverse)
}

// responses
func cliCCResponses(c *minicli.Command, resp *minicli.Response) error {
	id := c.StringArgs["id"]
	raw := c.BoolArgs["raw"]

	res, err := ccResponses(id, raw)
	if err == nil {
		resp.Response = res
	}

	return err
}

// prefix
func cliCCPrefix(c *minicli.Command, resp *minicli.Response) error {
	if prefix, ok := c.StringArgs["prefix"]; ok {
		ccSetPrefix(prefix)
		return nil
	}

	resp.Response = ccGetPrefix()
	return nil
}

// filter
func cliCCFilter(c *minicli.Command, resp *minicli.Response) error {
	if len(c.ListArgs["filter"]) > 0 {
		filter := &ron.Filter{}

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
				filter.IP = parts[1]
			case "mac":
				filter.MAC = parts[1]
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
		resp.Header = []string{"uuid", "hostname", "arch", "os", "ip", "mac", "tags"}
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
	cmd := &ron.Command{}

	// Add new files to send, expand globs
	for _, arg := range c.ListArgs["file"] {
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(*f_iomBase, arg)
		}
		arg = filepath.Clean(arg)

		if !strings.HasPrefix(arg, *f_iomBase) {
			return fmt.Errorf("can only send files from %v", *f_iomBase)
		}

		files, err := filepath.Glob(arg)
		if err != nil {
			return fmt.Errorf("non-existent files %v", arg)
		}

		if len(files) == 0 {
			return fmt.Errorf("no such file %v", arg)
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

	ccNewCommand(cmd, nil, nil)
	return nil
}

// recv
func cliCCFileRecv(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{}

	// Add new files to receive
	for _, file := range c.ListArgs["file"] {
		cmd.FilesRecv = append(cmd.FilesRecv, &ron.File{
			Name: file,
		})
	}

	ccNewCommand(cmd, nil, nil)
	return nil
}

// background (just exec with background==true)
func cliCCBackground(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Background: true,
		Command:    c.ListArgs["command"],
	}

	ccNewCommand(cmd, nil, nil)
	return nil
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
		return fmt.Errorf("invalid PID: `%v`", c.StringArgs["pid"])
	}

	ccProcessKill(pid)

	return nil
}

func cliCCProcessKillAll(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		KillAll: c.StringArgs["name"],
	}

	ccNewCommand(cmd, nil, nil)
	return nil
}

// exec
func cliCCExec(c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		Command: c.ListArgs["command"],
		Filter:  ccGetFilter(),
	}

	ccNewCommand(cmd, nil, nil)
	return nil
}

// process
func cliCCProcess(c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["kill"] {
		return cliCCProcessKill(c, resp)
	} else if c.BoolArgs["killall"] {
		return cliCCProcessKillAll(c, resp)
	}

	// list processes
	v := c.StringArgs["vm"]

	var activeVms []string

	if v == Wildcard {
		clients := ccGetClients()
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

		processes, err := ccGetProcesses(uuid)
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

// clients
func cliCCClients(c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"uuid", "hostname", "arch", "os", "ip", "mac",
	}

	for _, c := range ccGetClients() {
		row := []string{
			c.UUID,
			c.Hostname,
			c.Arch,
			c.OS,
			fmt.Sprintf("%v", c.IPs),
			fmt.Sprintf("%v", c.MACs),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

// command
func cliCCCommand(c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"id", "prefix", "command", "responses", "background",
		"send files", "receive files", "filter",
	}
	resp.Tabular = [][]string{}

	commands := ccCommands()

	// create sorted list of IDs
	var ids []int
	for id := range commands {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	for _, id := range ids {
		v := commands[id]
		row := []string{
			strconv.Itoa(v.ID),
			v.Prefix,
			fmt.Sprintf("%v", v.Command),
			strconv.Itoa(len(v.CheckedIn)),
			strconv.FormatBool(v.Background),
			fmt.Sprintf("%v", v.FilesSend),
			fmt.Sprintf("%v", v.FilesRecv),
			fmt.Sprintf("%v", v.Filter),
		}

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliCCDelete(c *minicli.Command, resp *minicli.Response) error {
	id := c.StringArgs["id"]

	if c.BoolArgs["command"] {
		return ccDeleteCommands(id)
	} else if c.BoolArgs["response"] {
		return ccDeleteResponses(id)
	}

	return errors.New("unreachable")
}

func cliCCClear(c *minicli.Command, resp *minicli.Response) error {
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
