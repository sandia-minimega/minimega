// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	log "minilog"
	"ron"
	"strconv"
)

// This file provides helper functions for muxing between the global
// ccServer/ccFilter/ccPrefix and the ones contained in the active namespace.
// If a namespace is active, those will be used over the global versions. If we
// move towards a default namespace, this file will be much simpler.

var (
	ccServer *ron.Server
	ccFilter *ron.Filter
	ccPrefix string
)

// ccStart starts ron and calls log.Fatal if there is a problem
func ccStart(path, subpath string) *ron.Server {
	s, err := ron.NewServer(path, subpath)
	if err != nil {
		log.Fatal("creating cc node %v", err)
	}

	return s
}

// ccGetFilter returns a filter for cc clients
func ccGetFilter() *ron.Filter {
	if ns := GetNamespace(); ns != nil {
		return ns.ccFilter
	}

	return ccFilter
}

// ccSetFilter updates the filter
func ccSetFilter(f *ron.Filter) {
	if ns := GetNamespace(); ns != nil {
		ns.ccFilter = f
		return
	}

	ccFilter = f
}

// ccGetPrefix returns the current prefix
func ccGetPrefix() string {
	if ns := GetNamespace(); ns != nil {
		return ns.ccPrefix
	}

	return ccPrefix
}

// ccSetPrefix updates the prefix
func ccSetPrefix(s string) {
	if ns := GetNamespace(); ns != nil {
		ns.ccPrefix = s
		return
	}

	ccPrefix = s
}

// ccDialSerial wraps ron.DialSerial for the specified namespace
func ccDialSerial(namespace, path string) error {
	ccServer := ccServer
	if namespace != "" {
		ns := GetOrCreateNamespace(namespace)
		ccServer = ns.ccServer
	}

	return ccServer.DialSerial(path)
}

// ccListenUnix wraps ron.ListenUnix for the specified namespace
func ccListenUnix(namespace, path string) error {
	ccServer := ccServer
	if namespace != "" {
		ns := GetOrCreateNamespace(namespace)
		ccServer = ns.ccServer
	}

	return ccServer.ListenUnix(path)
}

// ccCloseUnix wraps ron.CloseUnix for the specified namespace
func ccCloseUnix(namespace, path string) {
	ccServer := ccServer
	if namespace != "" {
		ns := GetOrCreateNamespace(namespace)
		ccServer = ns.ccServer
	}

	ccServer.CloseUnix(path)
}

// ccRegisterVM wraps ron.RegisterVM for the specified namespace
func ccRegisterVM(vm VM) {
	namespace := vm.GetNamespace()

	ccServer := ccServer
	if namespace != "" {
		ns := GetOrCreateNamespace(namespace)
		ccServer = ns.ccServer
	}

	ccServer.RegisterVM(vm)
}

// ccUnregisterVM wraps ron.RegisterVM for the specified namespace
func ccUnregisterVM(vm VM) {
	namespace := vm.GetNamespace()

	ccServer := ccServer
	if namespace != "" {
		ns := GetOrCreateNamespace(namespace)
		ccServer = ns.ccServer
	}

	ccServer.UnregisterVM(vm)
}

// ccNewCommand takes a command, adds the current filter, and then sends the
// command to the correct ron server. If a filter or prefix are specified, then
// they will be used instead of the current values.
func ccNewCommand(c *ron.Command, f *ron.Filter, p *string) int {
	ccServer := ccServer
	ccFilter := ccFilter
	ccPrefix := ccPrefix

	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
		ccFilter = ns.ccFilter
		ccPrefix = ns.ccPrefix
	}

	if f == nil {
		c.Filter = ccFilter
	} else {
		c.Filter = f
	}
	if p == nil {
		c.Prefix = ccPrefix
	} else {
		c.Prefix = *p
	}

	id := ccServer.NewCommand(c)
	log.Debug("generated command %v: %v", id, c)

	return id
}

// ccTunnel creates a forward or reverse tunnel. UUID is only used for forward
// tunnels.
func ccTunnel(host, uuid string, src, dst int, reverse bool) error {
	ccServer := ccServer
	ccFilter := ccFilter

	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
		ccFilter = ns.ccFilter
	}

	if reverse {
		return ccServer.Reverse(ccFilter, src, host, dst)
	}

	return ccServer.Forward(uuid, src, host, dst)
}

func ccGetClients() map[string]*ron.Client {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	return ccServer.GetClients()
}

func ccClients() int {
	ccServer := ccServer

	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	return ccServer.Clients()
}

func ccCommands() map[int]*ron.Command {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	return ccServer.GetCommands()
}

// ccResponses returns the responses matching s. s may be:
//  * Wildcard
//  * Integer ID
//  * Prefix
func ccResponses(s string, raw bool) (string, error) {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	if s == Wildcard {
		return ccServer.GetResponses(raw)
	} else if v, err := strconv.Atoi(s); err == nil {
		return ccServer.GetResponse(v, raw)
	}

	// must be searching for a prefix
	var match bool
	var buf bytes.Buffer

	for _, c := range ccServer.GetCommands() {
		if c.Prefix == s {
			s, err := ccServer.GetResponse(c.ID, raw)
			if err != nil {
				return "", err
			}

			buf.WriteString(s)

			match = true
		}
	}

	if !match {
		return "", fmt.Errorf("no such prefix: `%v`", s)
	}

	return buf.String(), nil
}

// ccDeleteCommands deletes commands matching s. s may be:
//  * Wildcard
//  * Integer ID
//  * Prefix
func ccDeleteCommands(s string) error {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	if s == Wildcard {
		ccServer.ClearCommands()
	} else if v, err := strconv.Atoi(s); err == nil {
		return ccServer.DeleteCommand(v)
	}

	return ccServer.DeleteCommands(s)
}

// ccDeleteResponses deletes commands matching s. s may be:
//  * Wildcard
//  * Integer ID
//  * Prefix
func ccDeleteResponses(s string) error {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	if s == Wildcard {
		ccServer.ClearResponses()
	} else if v, err := strconv.Atoi(s); err == nil {
		return ccServer.DeleteResponse(v)
	}

	return ccServer.DeleteResponses(s)
}

// ccGetProcesses returns the processes running on a given client
func ccGetProcesses(uuid string) ([]*ron.Process, error) {
	ccServer := ccServer
	if ns := GetNamespace(); ns != nil {
		ccServer = ns.ccServer
	}

	return ccServer.GetProcesses(uuid)
}

// ccProcessKill kills a process by PID
func ccProcessKill(pid int) {
	cmd := &ron.Command{PID: pid}

	ccNewCommand(cmd, nil, nil)
}

func ccClear(what string) error {
	namespace := ""
	ccServer := ccServer
	ccFilter := &ccFilter
	ccPrefix := &ccPrefix

	if ns := GetNamespace(); ns != nil {
		namespace = ns.Name
		ccServer = ns.ccServer
		ccFilter = &ns.ccFilter
		ccPrefix = &ns.ccPrefix
	}

	log.Info("clearing %v in namespace `%v`", what, namespace)

	switch what {
	case "filter":
		*ccFilter = nil
	case "commands":
		ccServer.ClearCommands()
	case "responses":
		ccServer.ClearResponses()
	case "prefix":
		*ccPrefix = ""
	}

	return nil
}
