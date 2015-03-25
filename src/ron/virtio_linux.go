// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build linux

package ron

import (
	log "minilog"
	"os"
)

func (c *Client) dialSerial(path string) error {
	log.Debug("ron dialSerial; %v", path)

	conn, err := os.OpenFile(path, os.O_RDWR, 0666)
	if err != nil {
		return err
	}

	c.conn = conn

	go c.handler()
	go c.mux()
	go c.periodic()
	go c.commandHandler()
	c.heartbeat()

	return nil
}
