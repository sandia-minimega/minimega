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
