package meshage

import (
	"encoding/gob"
	"net"
	"time"
	"sync"
	"errors"
	log "minilog"
	"io"
)

type client struct {
	name string
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

func (c *client) sendHandshake(solicited bool, name string, mesh map[string][]string) (bool, error) {
	log.Debug("got conn: %v\n", c.conn.RemoteAddr())

	var command int
	if solicited {
		command = HANDSHAKE_SOLICITED
	} else {
		command = HANDSHAKE
	}

	// initial handshake
	hs := Message{
		Recipients:   []string{}, // recipient doesn't matter here as it's expecting this handshake
		Source:       name,
		CurrentRoute: []string{name},
		ID:           0, // special case
		Command:      command,
		Body:         mesh,
	}
	err := c.enc.Encode(hs)
	if err != nil {
		if err != io.EOF {
			return false, err
		}
		return false, nil
	}

	err = c.dec.Decode(&hs)
	if err != nil {
		if err != io.EOF {
			return false, err
		}
		return false, nil
	}

	c.name = hs.Source

	return true, nil
}

func (c *client) recvHandshake(solicited bool, clients map[string]*client, name string) (bool, map[string][]string, error) {
	var hs Message
	err := c.dec.Decode(&hs)
	if err != nil {
		return false, nil, err
	}
	log.Debug("recvHandshake got: %v\n", hs)

	c.name = hs.Source

	// am i connecting to myself?
	if c.name == name {
		c.conn.Close()
		return false, nil, errors.New("connecting to myself is not allowed")
	}

	if _, ok := clients[c.name]; ok {
		// we are already connected to you, no thanks.
		c.conn.Close()
		return false, nil, errors.New("already connected")
	}

	// were we solicited?
	if hs.Command == HANDSHAKE && solicited {
		c.conn.Close()
		return false, nil, nil
	}

	resp := Message{
		Source:       name,
		CurrentRoute: []string{name},
	}
	err = c.enc.Encode(resp)
	if err != nil {
		return false, nil, err
	}

	return true, hs.Body.(map[string][]string), nil
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
