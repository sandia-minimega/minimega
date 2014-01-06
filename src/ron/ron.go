// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// package ron - reconciled often network
//
// ron is a http based file transfer and command and control
// protocol for massive networks of devices. ron trades the
// guarantee of delivery of messages for scalability. ron supports a
// multi-level tree of clients and relays (relays are simply
// intermediate nodes that mirror the master node to allow greater
// distribution).
package ron

import (
	"fmt"
	log "minilog"
)

const (
	MODE_MASTER = iota
	MODE_RELAY
	MODE_CLIENT
)

type Ron struct {
	mode   int
	parent string
	port   int
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
func New(mode int, parent string, port int) (*Ron, error) {
	r := &Ron{
		mode:   mode,
		parent: parent,
		port:   port,
	}

	switch mode {
	case MODE_MASTER:
		// a master node is simply a relay with no parent

		if parent != "" {
			return nil, fmt.Errorf("master mode must have no parent")
		}

		err := r.newRelay()
		if err != nil {
			return nil, err
		}
	case MODE_RELAY:
		if parent == "" {
			return nil, fmt.Errorf("relay mode must have a parent")
		}

		err := r.newRelay()
		if err != nil {
			return nil, err
		}
	case MODE_CLIENT:
		if parent == "" {
			return nil, fmt.Errorf("client mode must have a parent")
		}

		err := r.newClient()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %v %v %v", mode, parent, port)
	return r, nil
}
