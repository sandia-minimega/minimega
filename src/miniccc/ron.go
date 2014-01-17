// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"net/http"
)

const (
	MODE_MASTER = iota
	MODE_RELAY
	MODE_CLIENT
)

const (
	HEARTBEAT_RATE = 5
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
)

var (
	ronMode   int
	ronParent string
	ronPort   int
	ronHost   string
	ronRate   int
)

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
func ronStart() error {
	ronRate = HEARTBEAT_RATE
	ronHost = fmt.Sprintf("http://%v:%v/heartbeat", ronParent, ronPort)

	switch ronMode {
	case MODE_MASTER:
		// a master node is simply a relay with no parent
		if ronParent != "" {
			return fmt.Errorf("master mode must have no parent")
		}

		err := newRelay()
		if err != nil {
			return err
		}
	case MODE_RELAY:
		if ronParent == "" {
			return fmt.Errorf("relay mode must have a parent")
		}

		err := newRelay()
		if err != nil {
			return err
		}
	case MODE_CLIENT:
		if ronParent == "" {
			return fmt.Errorf("client mode must have a parent")
		}

		err := newClient()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid mode %v", ronMode)
	}

	log.Debug("registered new ron node: %v %v %v", ronMode, ronParent, ronPort)
	return nil
}

func newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron/", easter)
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/list/", handleList)
	http.HandleFunc("/list/raw", handleList)
	http.HandleFunc("/command/", handleCommands)
	http.HandleFunc("/command/new", handleNewCommand)
	http.HandleFunc("/command/delete", handleDeleteCommand)
	http.HandleFunc("/command/deletefiles", handleDeleteFiles)
	http.HandleFunc("/command/resubmit", handleResubmit)
	http.HandleFunc("/", handleRoot)

	host := fmt.Sprintf(":%v", ronPort)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	go heartbeat()

	return nil
}

func newClient() error {
	// start the periodic query to the parent
	go heartbeat()

	return nil
}
