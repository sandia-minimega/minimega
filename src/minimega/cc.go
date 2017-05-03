// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"ron"
)

// This file provides helper functions for muxing between the global
// ccServer/ccFilter/ccPrefix and the ones contained in the active namespace.
// If a namespace is active, those will be used over the global versions. If we
// move towards a default namespace, this file will be much simpler.

// ccStart starts ron and calls log.Fatal if there is a problem
func ccStart(path, subpath string) *ron.Server {
	s, err := ron.NewServer(path, subpath, plumber)
	if err != nil {
		log.Fatal("creating cc node %v", err)
	}

	return s
}

// ccNewCommand takes a command, adds the current filter, and then sends the
// command to the correct ron server. If a filter or prefix are specified, then
// they will be used instead of the current values.
func ccNewCommand(ns *Namespace, c *ron.Command, f *ron.Filter, p *string) int {
	if f == nil {
		c.Filter = ns.ccFilter
	} else {
		c.Filter = f
	}
	if p == nil {
		c.Prefix = ns.ccPrefix
	} else {
		c.Prefix = *p
	}

	id := ns.ccServer.NewCommand(c)
	log.Debug("generated command %v: %v", id, c)

	return id
}
