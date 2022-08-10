// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"math/rand"

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
	dest  string
	level log.Level
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
	to := []string{this.dest}

	msg := meshageLogMessage{
		TID:   rand.Int63(),
		From:  hostname,
		Level: this.level,
		Log:   string(l),
	}

	go func() {
		meshageNode.Set(to, msg)
	}()
}

func setupMeshageLogging(node string) {
	if node == "" {
		return
	}

	level := logLevel

	// Meshage events get included in debug logs... if we propogate those here we
	// end up in a memory-consuming loop.
	if level == log.DEBUG {
		level = log.INFO
	}

	meshLogger := meshageLogWriter{dest: node, level: level}
	log.AddLogger("meshage", meshLogger, level, false)
	logMeshNode = node
}
