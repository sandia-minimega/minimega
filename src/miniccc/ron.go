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

type ron struct {
	mode   int
	parent string
	port   int
	host   string
	rate   int
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
func (r *ron) Start(mode int, parent string, port int) error {
	r = &ron{
		mode:   mode,
		parent: parent,
		port:   port,
		rate:   HEARTBEAT_RATE,
		host:   fmt.Sprintf("http://%v:%v/heartbeat", parent, port),
	}

	switch mode {
	case MODE_MASTER:
		// a master node is simply a relay with no parent
		if parent != "" {
			return fmt.Errorf("master mode must have no parent")
		}

		err := r.newRelay()
		if err != nil {
			return err
		}
	case MODE_RELAY:
		if parent == "" {
			return fmt.Errorf("relay mode must have a parent")
		}

		err := r.newRelay()
		if err != nil {
			return err
		}
	case MODE_CLIENT:
		if parent == "" {
			return fmt.Errorf("client mode must have a parent")
		}

		err := r.newClient()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %v %v %v", mode, parent, port)
	return nil
}

func (r *ron) newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron", easter)
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/", handleRoot)

	host := fmt.Sprintf(":%v", r.port)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	go r.heartbeat()

	return nil
}

func (r *ron) newClient() error {
	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}
