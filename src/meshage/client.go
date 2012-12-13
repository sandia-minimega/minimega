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

func (n *Node) clientSend(host string, m *Message, async bool) error {
	log.Debug("clientSend %s: %v\n", host, m)
	if c, ok := n.clients[host]; ok {
		c.lock.Lock()
		defer c.lock.Unlock()

		err := c.enc.Encode(m)
		if err != nil {
			c.conn.Close()
			if async {
				n.errors <- err
			}
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
				err = errors.New("timeout")
				if async {
					n.errors <- err
				}
				return err
			}
		}
	}
	err := fmt.Errorf("no such client %s", host)
	if async {
		n.errors <- err
	}
	return err
}

// clientHandler is called as a goroutine after a successful handshake. It begins
// by issuing an MSA, and starting the receiver for the client. When the receiver
// exits, another MSA is issued without the client.
func (n *Node) clientHandler(host string) {
	log.Debug("clientHandler: %v\n", host)
	c := n.clients[host]

	n.MSA()

	for {
		var m Message
		err := c.dec.Decode(&m)
		if err != nil {
			if err != io.EOF {
				n.errors <- err
			}
			break
		}

		log.Debug("decoded message: %v: %#v\n", host, m)

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
					n.errors <- err
				}
				break
			}
			n.messagePump <- &m
		}
	}
	log.Debug("client %v disconnected\n", host)

	// client has disconnected
	c.conn.Close()
	n.clientLock.Lock()
	delete(n.clients, c.name)
	n.clientLock.Unlock()

	n.MSA()
}

func (n *Node) Hangup(host string) error {
	log.Debug("hangup: %v\n", host)
	if c, ok := n.clients[host]; ok {
		c.conn.Close()
		return nil
	}
	return fmt.Errorf("no such client: %s", host)
}
