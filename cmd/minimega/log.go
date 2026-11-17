// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// mesh node we are currently logging to, guarded by meshLogLock
var logMeshNode string

// meshLogLock guards logMeshNode and meshLogWriter, because CLI commands can
// run at the same time.
var meshLogLock sync.Mutex

// Message is a log message that can be sent over the minimega mesh to another
// node in the mesh. It's primary use is to consolidate logs from all nodes in a
// mesh on a single node.
type meshageLogMessage struct {
	TID   int64     // unique ID for this message
	From  string    // mesh node that generated the log
	Level log.Level // log level
	Log   string    // log message
}

const (
	// meshageLogQueue is the number of log lines that wait to go to the mesh.
	// When the queue is full, new lines are dropped.
	meshageLogQueue = 4096

	// meshageLogSenders is the number of log lines that go to the mesh at the
	// same time.
	meshageLogSenders = 4
)

// meshLogWriter is the writer of the "meshage" logger, nil when mesh logging
// is disabled. It is guarded by meshLogLock.
var meshLogWriter *meshageLogWriter

type meshageLogWriter struct {
	dest    string
	queue   chan meshageLogMessage
	stop    chan struct{}
	dropped atomic.Uint64
}

func newMeshageLogWriter(dest string) *meshageLogWriter {
	w := &meshageLogWriter{
		dest:  dest,
		queue: make(chan meshageLogMessage, meshageLogQueue),
		stop:  make(chan struct{}),
	}

	for i := 0; i < meshageLogSenders; i++ {
		go w.sender()
	}

	return w
}

// Write implements the io.Writer interface for meshageLogWriter.
func (this *meshageLogWriter) Write(p []byte) (n int, err error) {
	this.log(p)
	return len(p), nil
}

// log queues a log message for the specified host on the minimega mesh. No
// response is expected, thus this is non-blocking. When the queue is full, the
// message is dropped. Any errors sending logs over the mesh are silently
// ignored.
func (this *meshageLogWriter) log(l []byte) {
	var (
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

	select {
	case <-this.stop:
		return
	default:
	}

	select {
	case this.queue <- m:
	default:
		this.dropped.Add(1)
	}
}

// sender sends the queued log messages. Before the next message, it sends the
// number of messages that were dropped.
func (this *meshageLogWriter) sender() {
	for {
		var m meshageLogMessage

		select {
		case <-this.stop:
			return
		case m = <-this.queue:
		}

		if n := this.dropped.Swap(0); n > 0 {
			this.send(meshageLogMessage{
				TID:   rand.Int63(),
				From:  hostname,
				Level: log.WARN,
				Log:   fmt.Sprintf("%s WARN log.go: dropped %d log lines because the mesh did not send them fast enough", time.Now().Format("2006/01/02 15:04:05"), n),
			})
		}

		this.send(m)
	}
}

func (this *meshageLogWriter) send(m meshageLogMessage) {
	if _, ok := meshageNode.Mesh()[this.dest]; !ok {
		log.Warn("log destination node not found: %s", this.dest)
		return
	}

	meshageNode.Set([]string{this.dest}, m)
}

// getLogMeshNode returns the mesh node we are currently logging to.
func getLogMeshNode() string {
	meshLogLock.Lock()
	defer meshLogLock.Unlock()

	return logMeshNode
}

// stopMeshageLogging disables mesh logging.
func stopMeshageLogging() {
	meshLogLock.Lock()
	defer meshLogLock.Unlock()

	stopMeshageLoggingLocked()
}

// stopMeshageLoggingLocked disables mesh logging. The caller holds
// meshLogLock.
func stopMeshageLoggingLocked() {
	log.DelLogger("meshage")

	if meshLogWriter != nil {
		close(meshLogWriter.stop)
		meshLogWriter = nil
	}

	logMeshNode = ""
}

func setupMeshageLogging(node string) error {
	if node == "" {
		// Do not error out here, since node will be an empty string if the -headnode
		// flag was not provided (which is the default).
		return nil
	}

	if node == hostname {
		return fmt.Errorf("cannot use self as destination node for mesh logging")
	}

	level := logLevel

	// Meshage events get included in debug logs... if we propagate those here we
	// end up in a memory-consuming loop.
	if level == log.DEBUG {
		level = log.INFO
	}

	// We do not verify the presence of the destination node in the mesh in this
	// function just in case the mesh hasn't been fully formed by the time this
	// function is called (for example, at startup when the destination node is
	// specified via the -headnode flag).

	meshLogLock.Lock()
	defer meshLogLock.Unlock()

	// stop the writer of an earlier setup, so that only one writer runs
	stopMeshageLoggingLocked()

	meshLogWriter = newMeshageLogWriter(node)

	log.AddLogger("meshage", meshLogWriter, level, false)
	// Add a filter for the log message that could be generated if the log
	// destination node isn't in the mesh (defined above). Without this filter, we
	// end up in a memory-consuming loop.
	log.AddFilter("meshage", "log destination node not found")

	logMeshNode = node
	return nil
}
