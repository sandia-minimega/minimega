// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"minicli"
	log "minilog"
	"net"
	"os"
	"ron"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type ccMount struct {
	// Name of the VM, kept to help with unmount
	Name string
	// Addr for UFS
	Addr string
	// Path where the filesystem is mounted
	Path string
}

var ccCLIHandlers = []minicli.Handler{
	{ // cc
		HelpShort: "command and control commands",
		HelpLong: `
Command and control for VMs running the miniccc client. Commands may include
regular commands, backgrounded commands, and any number of sent and/or received
files. Commands will be executed in command creation order. For example, to
send a file 'foo' and display the contents on a remote VM:

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

Users can also filter by any column in "vm info" using a similar syntax:

	cc filter name=server
	cc filter vlan=DMZ

"vm info" columns take precedance over tags when both define the same key.

"cc mount" allows direct access to a guest's filesystem over the command and
control connection. When given a VM uuid or name and a path, the VM's
filesystem is mounted to the local machine at the provided path. "cc mount"
without arguments displays the existing mounts. Users can use "clear cc mount"
to unmount the filesystem of one or all VMs. This should be done before killing
or stopping the VM ("clear namespace <name>" will handle this automatically).

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

			"cc <tunnel,> <vm name or uuid> <src port> <host> <dst port>",
			"cc <rtunnel,> <src port> <host> <dst port>",

			"cc <delete,> <command,> <id or prefix or all>",
			"cc <delete,> <response,> <id or prefix or all>",
		},
		Call: wrapBroadcastCLI(cliCC),
	},
	{ // cc mount
		HelpShort: "list mounted filesystems",
		Patterns: []string{
			"cc mount",
		},
		Call: cliCCMount,
	},
	{ // cc mount uuid
		HelpShort: "mount VM filesystem",
		Patterns: []string{
			"cc mount <uuid or name> [path]",
		},
		Call: cliCCMountUUID,
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
		Call: wrapSimpleCLI(cliCCClear),
	},
	{ // clear cc mount
		HelpShort: "unmount VM filesystem",
		Patterns: []string{
			"clear cc mount [uuid or name or path]",
		},
		Call: cliCCClearMount,
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

		return unreachable()
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

	v := c.StringArgs["vm"]

	// get the vm uuid
	vm := ns.FindVM(v)
	if vm == nil {
		return vmNotFound(v)
	}
	log.Debug("got vm: %v %v", vm.GetID(), vm.GetName())
	uuid := vm.GetUUID()

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

	for _, c := range ns.ccServer.GetCommands() {
		if c.Prefix == s {
			s, err := ns.ccServer.GetResponse(c.ID, raw)
			if err != nil {
				return err
			}

			resp.Response += s + "\n"

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
				// Implicit filter on a tag or `vm info` field
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

	resp.Data = ns.NewCommand(cmd)
	return nil
}

// recv
func cliCCFileRecv(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{}

	// Add new files to receive
	for _, file := range c.ListArgs["file"] {
		cmd.FilesRecv = append(cmd.FilesRecv, file)
	}

	resp.Data = ns.NewCommand(cmd)
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

	resp.Data = ns.NewCommand(cmd)
	return nil
}

func cliCCProcessKill(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// kill all processes
	if c.StringArgs["pid"] == Wildcard {
		cmd := &ron.Command{PID: -1}
		resp.Data = ns.NewCommand(cmd)

		return nil
	}

	// kill single process
	pid, err := strconv.Atoi(c.StringArgs["pid"])
	if err != nil {
		return fmt.Errorf("invalid PID: `%v`", c.StringArgs["pid"])
	}

	cmd := &ron.Command{PID: pid}
	resp.Data = ns.NewCommand(cmd)

	return nil
}

func cliCCProcessKillAll(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	cmd := &ron.Command{
		KillAll: c.StringArgs["name"],
	}

	resp.Data = ns.NewCommand(cmd)
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

	resp.Data = ns.NewCommand(cmd)
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

	resp.Data = ns.NewCommand(cmd)
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

	return unreachable()
}

func cliCCListen(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	port, err := strconv.Atoi(c.StringArgs["port"])
	if err != nil {
		return err
	}

	return ns.ccServer.Listen(port)
}

// cliCCMount needs to collect mounts from both the local ccMounts for the
// namespace and across the cluster.
func cliCCMount(c *minicli.Command, respChan chan<- minicli.Responses) {
	ns := GetNamespace()

	// makeResponse creates a response from the namespace's ccMounts
	makeResponse := func() *minicli.Response {
		resp := &minicli.Response{Host: hostname}

		resp.Header = []string{"name", "uuid", "addr", "path"}

		for uuid, mnt := range ns.ccMounts {
			resp.Tabular = append(resp.Tabular, []string{
				mnt.Name,
				uuid,
				mnt.Addr,
				mnt.Path,
			})
		}

		return resp
	}

	// local behavior, see cli.go
	if c.Source != "" {
		respChan <- minicli.Responses{makeResponse()}
		return
	}

	var res minicli.Responses

	// LOCK: this is a CLI handler so we already hold the cmdLock.
	for resps := range runCommands(namespaceCommands(ns, c)...) {
		for _, resp := range resps {
			res = append(res, resp)
		}
	}

	// if local node is not in namespace, append local response too
	if !ns.Hosts[hostname] {
		res = append(res, makeResponse())
	}

	respChan <- res

	return
}

func cliCCMountUUID(c *minicli.Command, respChan chan<- minicli.Responses) {
	ns := GetNamespace()

	resp := &minicli.Response{Host: hostname}

	id := c.StringArgs["uuid"]
	path := c.StringArgs["path"]

	if path == "" && c.Source == "" {
		// TODO: we could generate a sane default
		resp.Error = "must provide a mount path"

		respChan <- minicli.Responses{resp}
		return
	}

	if _, err := os.Stat(path); path != "" && os.IsNotExist(err) {
		resp.Error = "mount point does not exist"

		respChan <- minicli.Responses{resp}
		return
	}

	var vm VM

	// If we're doing the local behavior, only look at the local VMs.
	// Otherwise, look globally. See note in cli.go.
	if c.Source == "" {
		// LOCK: this is a CLI handler so we already hold the cmdLock.
		for _, vm2 := range globalVMs(ns) {
			if vm2.GetName() == id || vm2.GetUUID() == id {
				vm = vm2
				break
			}
		}
	} else {
		vm = ns.VMs.FindVM(id)
	}

	if vm == nil {
		resp.Error = vmNotFound(id).Error()

		respChan <- minicli.Responses{resp}
		return
	}

	// sanity check
	if c.Source != "" && vm.GetHost() != hostname {
		resp.Error = "holy heisenvm"

		respChan <- minicli.Responses{resp}
		return
	}

	if vm.GetHost() == hostname {
		// VM is running locally
		if mnt, ok := ns.ccMounts[vm.GetUUID()]; ok {
			resp.Error = fmt.Sprintf("already connected to %v", mnt.Addr)

			respChan <- minicli.Responses{resp}
			return
		}

		// Start UFS
		port, err := ns.ccServer.ListenUFS(vm.GetUUID())
		if err != nil {
			resp.Error = err.Error()

			respChan <- minicli.Responses{resp}
			return
		}

		log.Debug("ufs for %v started on %v", vm.GetUUID(), port)

		mnt := ccMount{
			Name: vm.GetName(),
			Addr: fmt.Sprintf("%v:%v", vm.GetHost(), port),
			Path: path,
		}

		if path == "" {
			ns.ccMounts[vm.GetUUID()] = mnt

			resp.Response = strconv.Itoa(port)

			respChan <- minicli.Responses{resp}
			return
		}

		log.Info("mount for %v from :%v to %v", vm.GetUUID(), port, path)

		// do the mount
		opts := fmt.Sprintf("trans=tcp,port=%v,version=9p2000", port)

		if err := syscall.Mount("127.0.0.1", path, "9p", 0, opts); err != nil {
			if err := ns.ccServer.DisconnectUFS(vm.GetUUID()); err != nil {
				// zombie UFS
				log.Error("unable to disconnect ufs for %v: %v", vm.GetUUID(), err)
			}
			resp.Error = err.Error()

			respChan <- minicli.Responses{resp}
			return
		}

		ns.ccMounts[vm.GetUUID()] = mnt

		respChan <- minicli.Responses{resp}
		return
	}

	if mnt, ok := ns.ccMounts[vm.GetUUID()]; ok {
		resp.Error = fmt.Sprintf("already connected to %v", mnt.Addr)

		respChan <- minicli.Responses{resp}
		return
	}

	// VM is running on a remote host
	cmd := minicli.MustCompilef("namespace %v cc mount %v", ns.Name, vm.GetUUID())
	cmd.SetSource(ns.Name)
	cmd.SetRecord(false)

	respChan2, err := meshageSend(cmd, vm.GetHost())
	if err != nil {
		resp.Error = err.Error()

		respChan <- minicli.Responses{resp}
		return
	}

	var port int

	for resps := range respChan2 {
		for _, resp := range resps {
			// error from previous response... there should only be one
			if err != nil {
				continue
			}

			if resp.Error != "" {
				err = errors.New(resp.Error)
			} else {
				port, err = strconv.Atoi(resp.Response)
			}
		}
	}

	if err != nil {
		resp.Error = err.Error()
	} else if port == 0 {
		resp.Error = "unable to find UFS port"
	} else {
		log.Info("remote mount for %v from %v:%v to %v", vm.GetUUID(), vm.GetHost(), port, path)

		addr, err := net.ResolveIPAddr("ip", vm.GetHost())
		if err != nil {
			resp.Error = err.Error()

			respChan <- minicli.Responses{resp}
			return
		}

		log.Info("resolved host %v to %v", vm.GetHost(), addr)

		// do the (remote) mount
		opts := fmt.Sprintf("trans=tcp,port=%v,version=9p2000", port)

		if err := syscall.Mount(addr.IP.String(), path, "9p", 0, opts); err != nil {
			resp.Error = err.Error()
		} else {
			ns.ccMounts[vm.GetUUID()] = ccMount{
				Name: vm.GetName(),
				Addr: fmt.Sprintf("%v:%v", vm.GetHost(), port),
				Path: path,
			}
		}
	}

	respChan <- minicli.Responses{resp}
	return
}

func cliCCClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	// local behavior, see cli.go
	if c.Source != "" {
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

		if len(c.BoolArgs) == 0 {
			// clear mounts too (not a sub handler)
			if err := ns.clearCCMount(""); err != nil {
				return err
			}
		}

		return nil
	}

	// local clean up
	if err := ns.clearCCMount(""); err != nil {
		return err
	}

	// fan out behavior
	// LOCK: this is a CLI handler so we already hold the cmdLock.
	return consume(runCommands(namespaceCommands(ns, c)...))
}

func cliCCClearMount(c *minicli.Command, respChan chan<- minicli.Responses) {
	ns := GetNamespace()

	resp := &minicli.Response{Host: hostname}

	id := c.StringArgs["uuid"]

	if err := ns.clearCCMount(id); err != nil {
		resp.Error = err.Error()

		respChan <- minicli.Responses{resp}
		return
	}

	if c.Source == "" {
		// LOCK: this is a CLI handler so we already hold the cmdLock.
		err := consume(runCommands(namespaceCommands(ns, c)...))
		if err != nil {
			resp.Error = err.Error()
		}
	}

	respChan <- minicli.Responses{resp}
	return
}
