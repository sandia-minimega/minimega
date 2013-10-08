// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package meshage

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	log "minilog"
	"net"
	"sync"
	"time"
)

type client struct {
	name string // name of client
	conn net.Conn
	enc  *gob.Encoder
	dec  *gob.Decoder
	ack  chan uint64
	lock sync.Mutex
}

func (n *Node) clientSend(host string, m *Message) error {
	log.Debug("clientSend %s: %v", host, m)
	if c, ok := n.clients[host]; ok {
		c.lock.Lock()
		defer c.lock.Unlock()

		err := c.enc.Encode(m)
		if err != nil {
			c.conn.Close()
			return err
		}

		// wait for a response
		for {
			select {
			case ID := <-c.ack:
				if ID == m.ID {
					return nil
				}
			case <-time.After(n.timeout):
				c.conn.Close()
				return errors.New("timeout")
			}
		}
	}
	return fmt.Errorf("no such client %s", host)
}

func (c *client) clientMessagePump(p chan *Message) {
	for {
		var m Message
		err := c.dec.Decode(&m)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			p <- nil
			return
		}
		log.Debug("decoded message: %v: %#v", c.name, m)
		p <- &m
	}
}

// clientHandler is called as a goroutine after a successful handshake. It begins
// by issuing an MSA, and starting the receiver for the client. When the receiver
// exits, another MSA is issued without the client.
func (n *Node) clientHandler(host string) {
	log.Debug("clientHandler: %v", host)
	c := n.clients[host]

	n.MSA()

	clientMessages := make(chan *Message)
	go c.clientMessagePump(clientMessages)

CLIENT_HANDLER_LOOP:
	for {
		select {
		case m := <-clientMessages:
			if m == nil {
				break CLIENT_HANDLER_LOOP
			}
			if m.Command == ACK {
				c.ack <- m.ID
			} else {
				// send an ack
				a := Message{
					Command: ACK,
					ID:      m.ID,
				}
				err := c.enc.Encode(a)
				if err != nil {
					if err != io.EOF {
						log.Errorln(err)
					}
					break CLIENT_HANDLER_LOOP
				}
				n.messagePump <- m
			}
		case <-time.After(2 * time.Duration(n.msaTimeout) * time.Second):
			log.Error("client %v MSA timeout", host)
			break CLIENT_HANDLER_LOOP
		}
	}
	log.Info("client %v disconnected", host)

	// client has disconnected
	c.conn.Close()
	n.clientLock.Lock()
	delete(n.clients, c.name)
	n.clientLock.Unlock()
	go n.checkDegree()

	n.MSA()
}

// Dicconnect from the specified host.
func (n *Node) Hangup(host string) error {
	log.Debug("hangup: %v", host)
	if c, ok := n.clients[host]; ok {
		c.conn.Close()
		return nil
	}
	return fmt.Errorf("no such client: %s", host)
}
