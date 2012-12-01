package meshage

import (
	"encoding/gob"
	"net"
	"time"
	"sync"
	"errors"
	log "minilog"
)

type client struct {
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
	timeout time.Duration
	lock sync.Mutex
	ack chan uint64
}

func newClient(conn net.Conn, timeout time.Duration) *client {
	return &client{
		conn: conn,
		enc: gob.NewEncoder(conn),
		dec: gob.NewDecoder(conn),
		timeout: timeout,
		ack: make(chan uint64, RECEIVE_BUFFER),
	}
}

func (c *client) send(m Message) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	log.Debug("encoding message: %v\n", m)
	err := c.enc.Encode(m)
	if err != nil {
		return err
	}

	// wait for an ack or a timeout
ACKLOOP:
	for {
		select {
		case a := <-c.ack:
			if a == m.ID {
				break ACKLOOP
			}
		case <-time.After(c.timeout):
			c.hangup()
			return errors.New("timeout")
		}
	}
	return nil
}

func (c *client) receive() (Message, error) {
	var m Message
	for {
		err := c.dec.Decode(&m)
		if err != nil {
			return Message{}, err
		}
		log.Debug("decoded message: %#v\n", m)
		if m.Command == ACK {
			c.ack <- m.Body.(uint64)
			m = Message{}
		} else {
			// send an ack
			a := Message{
				Command: ACK,
				Body: m.ID,
			}
			c.enc.Encode(a)
			break
		}
	}
	return m, nil
}

func (c *client) hangup() {
	c.conn.Close()
}
