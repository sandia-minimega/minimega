// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// mesh node we are currently logging to
var logMeshNode string

// Message is a log message that can be sent over the minimega mesh to another
// node in the mesh. It's primary use is to consolidate logs from all nodes in a
// mesh on a single node.
type meshageLogMessage struct {
	TID   int64     // unique ID for this message
	From  string    // mesh node that generated the log
	Level log.Level // log level
	Log   string    // log message
}

type meshageLogWriter struct {
	dest string
}

// Write implements the io.Writer interface for meshageLogWriter.
func (this meshageLogWriter) Write(p []byte) (n int, err error) {
	this.log(p)
	return len(p), nil
}

// log sends a log message to the specified host over the minimega mesh. No
// response is expected, thus this is non-blocking. Any errors sending logs over
// the mesh are silently ignored.
func (this meshageLogWriter) log(l []byte) {
	var (
		to     = []string{this.dest}
		msg    = string(l)
		fields = strings.Fields(msg)
	)

	// Should never happen... but let's do our best to avoid a panic anyway.
	if len(fields) < 3 {
		return
	}

	// The incoming log will have already been formatted with the standard log
	// flags, to include the log level as the 3rd word in the log. This should
	// *always* be the case. If for some reason it's not, the level variable will
	// be set to -1 which means the receiving node won't end up doing anything
	// with it.
	level, _ := log.ParseLevel(strings.ToLower(fields[2]))

	m := meshageLogMessage{
		TID:   rand.Int63(),
		From:  hostname,
		Level: level,
		Log:   msg,
	}

	go func() {
		if _, ok := meshageNode.Mesh()[this.dest]; !ok {
			log.Warn("log destination node not found: %s", this.dest)
			return
		}

		meshageNode.Set(to, m)
	}()
}

func setupMeshageLogging(node string) error {
	if node == "" {
		// Do not error out here, since node will be an empty string if the -lognode
		// flag was not provided (which is the default).
		return nil
	}

	if node == hostname {
		return fmt.Errorf("cannot use self as destination node for mesh logging")
	}

	level := logLevel

	// Meshage events get included in debug logs... if we propogate those here we
	// end up in a memory-consuming loop.
	if level == log.DEBUG {
		level = log.INFO
	}

	// We do not verify the presence of the destination node in the mesh in this
	// function just in case the mesh hasn't been fully formed by the time this
	// function is called (for example, at startup when the destination node is
	// specified via the -lognode flag).

	meshLogger := meshageLogWriter{dest: node}

	log.AddLogger("meshage", meshLogger, level, false)
	// Add a filter for the log message that could be generated if the log
	// destination node isn't in the mesh (defined above). Without this filter, we
	// end up in a memory-consuming loop.
	log.AddFilter("meshage", "log destination node not found")

	logMeshNode = node
	return nil
}
