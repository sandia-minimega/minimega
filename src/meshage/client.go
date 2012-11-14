package meshage

import (
	"encoding/gob"
	"net"
)

type client struct {
	name string
	conn net.Conn
	enc  gob.Encoder
	dec  gob.Decoder
}

func (c *client) receiveHandler(pump chan Message) {
	for {
		var m Message
		err := c.dec.Decode(m)
		if err != nil {
			// TODO: error handling
		} else {
			pump <- m
		}
	}
}

func (c *client) send(m Message) {
	err := c.enc.Encode(m)
	if err != nil {
		// TODO: error handling
	}
}
