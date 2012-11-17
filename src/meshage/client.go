package meshage

import (
	"encoding/gob"
	"net"
)

type client struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
}

func (c *client) send(m Message) error {
	err := c.enc.Encode(m)
	if err != nil {
		return err
	}
	return nil
}
