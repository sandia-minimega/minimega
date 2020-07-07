// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package meshage

import (
	"encoding/gob"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	deadlineMultiplier = 2
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
	if log.WillLog(log.DEBUG) {
		log.Debug("clientSend %s: %v", host, m)
	}

	c, err := n.getClient(host)
	if err != nil {
		return err
	}

	c.lock.Lock()
	defer c.lock.Unlock()

	err = c.enc.Encode(m)
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

// clientHandler is called as a goroutine after a successful handshake. It
// begins by issuing an MSA. When the receiver exits, another MSA is issued
// without the client.
func (n *Node) clientHandler(host string) {
	log.Debug("clientHandler: %v", host)

	c, err := n.getClient(host)
	if err != nil {
		log.Error("client %v vanished -- %v", host, err)
		return
	}

	n.MSA()

	for {
		var m Message
		c.conn.SetReadDeadline(time.Now().Add(deadlineMultiplier * n.msaTimeout))
		err := c.dec.Decode(&m)
		if err != nil {
			if err != io.EOF && !strings.Contains(err.Error(), "connection reset by peer") {
				log.Error("client %v decode: %v", host, err)
			}
			break
		}
		if log.WillLog(log.DEBUG) {
			log.Debug("decoded message: %v: %v", c.name, &m)
		}
		if m.Command == ACK {
			c.ack <- m.ID
		} else {
			// send an ack
			a := Message{
				Command: ACK,
				ID:      m.ID,
			}
			c.conn.SetWriteDeadline(time.Now().Add(deadlineMultiplier * n.msaTimeout))
			err := c.enc.Encode(a)
			if err != nil {
				if err != io.EOF {
					log.Error("client %v encode ACK: %v", host, err)
				}
				break
			}
			n.messagePump <- &m
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

	c, err := n.getClient(host)
	if err != nil {
		return err
	}

	c.conn.Close()
	return nil
}
