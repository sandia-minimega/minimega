// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"math/rand"
	"sync"
	"time"
)

// meshageStatusMessage is a message that is sent over the minimega mesh to
// another node in the mesh to update it on the status of a long running
// command.
type meshageStatusMessage struct {
	TID    int64  // unique ID for this message
	From   string // mesh node that generated the status
	Status string // status message
}

var (
	// guards messageStatusChans and meshageStatusPeriod
	meshageStatusLock  sync.RWMutex
	meshageStatusChans = make(map[string]chan string)
	// default amount of time to wait between status update publishes
	meshageStatusPeriod = 3 * time.Second
)

func addStatusMessageChannel(name string, c chan string) {
	meshageStatusLock.Lock()
	defer meshageStatusLock.Unlock()

	meshageStatusChans[name] = c
}

func delStatusMessageChannel(name string) {
	meshageStatusLock.Lock()
	defer meshageStatusLock.Unlock()

	delete(meshageStatusChans, name)
}

func sendStatusMessage(status string, to ...string) {
	m := meshageStatusMessage{
		TID:    rand.Int63(),
		From:   hostname,
		Status: status,
	}

	go func() {
		meshageNode.Set(to, m)
	}()
}
