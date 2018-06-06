// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
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

Executed commands can have their stdio tied to pipes used by the plumb and pipe
APIs. To use named pipes, simply specify stdin, stdout, or stderr as a
key=value pair. For example:

	cc exec stderr=foo cat server.log
	cc background stdin=foo stdout=bar /usr/bin/program

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
			"cc <listen,> <port>",
			"cc <clients,>",
			"cc <filter,> [filter]...",
			"cc <commands,>",

			"cc <prefix,> [prefix]",

			"cc <send,> <file>...",
			"cc <recv,> <file>...",
			"cc <exec,> <command>...",
			"cc <background,> <command>...",

			"cc <process,> <list,> <vm name, uuid or all>",
			"cc <process,> <kill,> <pid or all>",
			"cc <process,> <killall,> <name>",

			"cc <log,> level <debug,info,warn,error,fatal>",

			"cc <responses,> <id or prefix or all> [raw,]",

			"cc <tunnel,> <uuid> <src port> <host> <dst port>",
			"cc <rtunnel,> <src port> <host> <dst port>",

			"cc <mount,>",
			"cc <mount,> <uuid or name> <path>",

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
var ccCliSubHandlers = map[string]wrappedCLIFunc{
	"background": cliCCBackground,
	"clients":    cliCCClients,
	"commands":   cliCCCommand,
	"delete":     cliCCDelete,
	"exec":       cliCCExec,
	"filter":     cliCCFilter,
	"log":        cliCCLog,
	"prefix":     cliCCPrefix,
	"process":    cliCCProcess,
	"recv":       cliCCFileRecv,
	"responses":  cliCCResponses,
	"rtunnel":    cliCCTunnel,
	"send":       cliCCFileSend,
	"tunnel":     cliCCTunnel,
	"listen":     cliCCListen,
	"mount":      cliCCMount,
}

func cliCC(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// Dispatcher for a sub handler
	if len(c.BoolArgs) > 0 {
		for k, fn := range ccCliSubHandlers {
			if c.BoolArgs[k] {
				log.Debug("cc handler %v", k)
				return fn(ns, c, resp)
			}
		}

		return errors.New("unreachable")
	}

	// If no sub handler, display the number of clients instead
	resp.Header = []string{"clients"}
	resp.Tabular = [][]string{
		[]string{
			strconv.Itoa(ns.ccServer.Clients()),
		},
	}

	return nil
}

// tunnel
func cliCCTunnel(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
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

	if c.BoolArgs["rtunnel"] {
		return ns.ccServer.Reverse(ns.ccFilter, src, host, dst)
	}

	return ns.ccServer.Forward(uuid, src, host, dst)
}

// responses
func cliCCResponses(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	s := c.StringArgs["id"]
	raw := c.BoolArgs["raw"]

	if s == Wildcard {
		r, err := ns.ccServer.GetResponses(raw)
		if err == nil {
			resp.Response = r
		}
		return err
	} else if v, err := strconv.Atoi(s); err == nil {
		r, err := ns.ccServer.GetResponse(v, raw)
		if err == nil {
			resp.Response = r
		}
		return err
	}

	// must be searching for a prefix
	var match bool
	var buf bytes.Buffer

	for _, c := range ns.ccServer.GetCommands() {
		if c.Prefix == s {
			s, err := ns.ccServer.GetResponse(c.ID, raw)
			if err != nil {
				return err
			}

			buf.WriteString(s)

			match = true
		}
	}

	if !match {
		return fmt.Errorf("no such prefix: `%v`", s)
	}

	return nil
}

// prefix
func cliCCPrefix(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if prefix, ok := c.StringArgs["prefix"]; ok {
		ns.ccPrefix = prefix
		return nil
	}

	resp.Response = ns.ccPrefix
	return nil
}

// filter
func cliCCFilter(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
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

		ns.ccFilter = filter
		return nil
	}

	// Summary of current filter
	if ns.ccFilter != nil {
		resp.Header = []string{"uuid", "hostname", "arch", "os", "ip", "mac", "tags"}
		row := []string{
			ns.ccFilter.UUID,
			ns.ccFilter.Hostname,
			ns.ccFilter.Arch,
			ns.ccFilter.OS,
			fmt.Sprintf("%v", ns.ccFilter.IP),
			fmt.Sprintf("%v", ns.ccFilter.MAC),
		}

		// encode the tags using JSON
		tags, err := json.Marshal(ns.ccFilter.Tags)
		if err != nil {
			log.Warn("Unable to json marshal tags: %v", err)
		} else if ns.ccFilter.Tags == nil {
			tags = []byte("{}")
		}
		row = append(row, string(tags))

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

// send
func cliCCFileSend(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	cmd, err := ns.ccServer.NewFilesSendCommand(c.ListArgs["file"])
	if err != nil {
		return err

	}

	ns.NewCommand(cmd)
	return nil
}

// recv
func cliCCFileRecv(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{}

	// Add new files to receive
	for _, file := range c.ListArgs["file"] {
		cmd.FilesRecv = append(cmd.FilesRecv, &ron.File{
			Name: file,
		})
	}

	ns.NewCommand(cmd)
	return nil
}

// background (just exec with background==true)
func cliCCBackground(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	stdin, stdout, stderr, command := ccCommandPreProcess(c.ListArgs["command"])

	cmd := &ron.Command{
		Background: true,
		Command:    command,
		Stdin:      stdin,
		Stdout:     stdout,
		Stderr:     stderr,
	}

	ns.NewCommand(cmd)
	return nil
}

func cliCCProcessKill(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// kill all processes
	if c.StringArgs["pid"] == Wildcard {
		cmd := &ron.Command{PID: -1}
		ns.NewCommand(cmd)

		return nil
	}

	// kill single process
	pid, err := strconv.Atoi(c.StringArgs["pid"])
	if err != nil {
		return fmt.Errorf("invalid PID: `%v`", c.StringArgs["pid"])
	}

	cmd := &ron.Command{PID: pid}
	ns.NewCommand(cmd)

	return nil
}

func cliCCProcessKillAll(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		KillAll: c.StringArgs["name"],
	}

	ns.NewCommand(cmd)
	return nil
}

// exec
func cliCCExec(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	stdin, stdout, stderr, command := ccCommandPreProcess(c.ListArgs["command"])

	cmd := &ron.Command{
		Command: command,
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
	}

	ns.NewCommand(cmd)
	return nil
}

// process
func cliCCProcess(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	if c.BoolArgs["kill"] {
		return cliCCProcessKill(ns, c, resp)
	} else if c.BoolArgs["killall"] {
		return cliCCProcessKillAll(ns, c, resp)
	}

	// list processes
	v := c.StringArgs["vm"]

	var activeVms []string

	if v == Wildcard {
		clients := ns.ccServer.GetClients()
		for _, client := range clients {
			activeVms = append(activeVms, client.UUID)
		}
	} else {
		// get the vm uuid
		vm := ns.FindVM(v)
		if vm == nil {
			return vmNotFound(v)
		}
		log.Debug("got vm: %v %v", vm.GetID(), vm.GetName())
		activeVms = []string{vm.GetUUID()}
	}

	resp.Header = []string{"name", "uuid", "pid", "command"}
	for _, uuid := range activeVms {
		vm := ns.FindVM(uuid)
		if vm == nil {
			return vmNotFound(v)
		}

		processes, err := ns.ccServer.GetProcesses(uuid)
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

// parse out key/value pairs from the command list for stdio
func ccCommandPreProcess(c []string) (stdin, stdout, stderr string, command []string) {
	// pop key/value pairs (up to three) for stdio plumber redirection
	for i := 0; i < 3 && i < len(c); i++ {
		f := strings.Split(c[i], "=")
		if len(f) == 1 {
			command = c[i:]
			return
		}
		switch f[0] {
		case "stdin":
			stdin = f[1]
		case "stdout":
			stdout = f[1]
		case "stderr":
			stderr = f[1]
		default:
			// perhaps some goofy filename with an = in it
			command = c[i:]
			return
		}
	}
	command = c[3:]
	return
}

func cliCCLog(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// search for level in BoolArgs, we know that one of the BoolArgs will
	// parse without error thanks to minicli.
	var level log.Level
	for k := range c.BoolArgs {
		v, err := log.ParseLevel(k)
		if err == nil {
			level = v
			break
		}
	}

	cmd := &ron.Command{
		Level: &level,
	}

	ns.NewCommand(cmd)
	return nil
}

// clients
func cliCCClients(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"uuid", "hostname", "arch", "os", "ip", "mac",
	}

	for _, c := range ns.ccServer.GetClients() {
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
func cliCCCommand(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{
		"id", "prefix", "command", "responses", "background",
		"sent", "received", "level", "filter",
	}
	resp.Tabular = [][]string{}

	commands := ns.ccServer.GetCommands()

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
		}

		if v.Level != nil {
			row = append(row, v.Level.String())
		} else {
			row = append(row, "")
		}

		row = append(row, fmt.Sprintf("%v", v.Filter))

		resp.Tabular = append(resp.Tabular, row)
	}

	return nil
}

func cliCCDelete(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	s := c.StringArgs["id"]

	if c.BoolArgs["command"] {
		if s == Wildcard {
			ns.ccServer.ClearCommands()
			return nil
		} else if v, err := strconv.Atoi(s); err == nil {
			return ns.ccServer.DeleteCommand(v)
		}

		return ns.ccServer.DeleteCommands(s)
	} else if c.BoolArgs["response"] {
		if s == Wildcard {
			ns.ccServer.ClearResponses()
			return nil
		} else if v, err := strconv.Atoi(s); err == nil {
			return ns.ccServer.DeleteResponse(v)
		}

		return ns.ccServer.DeleteResponses(s)
	}

	return errors.New("unreachable")
}

func cliCCListen(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	port, err := strconv.Atoi(c.StringArgs["port"])
	if err != nil {
		return err
	}

	return ns.ccServer.Listen(port)
}

func cliCCMount(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	id := c.StringArgs["uuid"]
	path := c.StringArgs["path"]

	if id != "" {
		// id can be UUID or VM's name
		vm := ns.VMs.FindVM(id)
		if vm == nil {
			return vmNotFound(id)
		}

		return ns.ccServer.Mount(vm.GetUUID(), path)
	}

	// TODO: display existing mounts
	/*
		resp.Header = []string{"client", "path"}
		resp.Tabular = [][]string{
			[]string{
				strconv.Itoa(ns.ccServer.Clients()),
			},
		}
	*/

	return nil
}

func cliCCClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	for what := range ccCliSubHandlers {
		// We only want to clear something if it was specified on the
		// command line or if we're clearing everything (nothing was
		// specified).
		if c.BoolArgs[what] || len(c.BoolArgs) == 0 {
			log.Info("clearing %v in namespace `%v`", what, ns.Name)

			switch what {
			case "filter":
				ns.ccFilter = nil
			case "commands":
				ns.ccServer.ClearCommands()
			case "responses":
				ns.ccServer.ClearResponses()
			case "prefix":
				ns.ccPrefix = ""
			}
		}
	}

	return nil
}
