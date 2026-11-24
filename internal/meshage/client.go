// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package meshage

import (
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	deadlineMultiplier = 2

	// maxRedialWait is the longest wait before a node dials a bulk or log
	// link again.
	maxRedialWait = 30 * time.Second
)

// client is a peer. The control link carries the mesh state and decides if
// the peer is connected. The other links carry their class of traffic, and
// the control link carries it when they are down.
type client struct {
	name    string
	addr    string // address that this node dialed, empty if the peer dialed
	session uint64 // the same on all links of the peer

	lock  sync.Mutex
	links [numClasses]*link
	done  chan struct{} // closed when the control link closes
}

func newClient(name, addr string, session uint64, control *link) *client {
	c := &client{
		name:    name,
		addr:    addr,
		session: session,
		done:    make(chan struct{}),
	}
	c.links[Control] = control

	return c
}

// link returns the link for class, or the control link when that link is
// down.
func (c *client) link(class Class) *link {
	c.lock.Lock()
	defer c.lock.Unlock()

	if class.valid() {
		if l := c.links[class]; l != nil && !l.closed() {
			return l
		}
	}

	return c.links[Control]
}

// setLink replaces the link for class and closes the old link.
func (c *client) setLink(class Class, l *link) {
	c.lock.Lock()
	old := c.links[class]
	c.links[class] = l
	c.lock.Unlock()

	if old != nil && old != l {
		old.close(errors.New("replaced"))
	}
}

// clearLink removes l, if it is still the link for its class.
func (c *client) clearLink(l *link) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if c.links[l.class] == l {
		c.links[l.class] = nil
	}
}

// close closes all the links of the client.
func (c *client) close(err error) {
	c.lock.Lock()
	links := c.links
	c.lock.Unlock()

	for _, l := range links {
		if l != nil {
			l.close(err)
		}
	}
}

// connected returns the classes that have a link that is up.
func (c *client) connected() []Class {
	c.lock.Lock()
	defer c.lock.Unlock()

	var res []Class
	for class, l := range c.links {
		if l != nil && !l.closed() {
			res = append(res, Class(class))
		}
	}

	return res
}

func (n *Node) clientSend(host string, m *Message) error {
	if log.WillLog(log.DEBUG) {
		log.Debug("clientSend %s: %v", host, m)
	}

	c, err := n.getClient(host)
	if err != nil {
		return err
	}

	return c.link(m.Class).send(m)
}

// readLink reads the messages of l until the link closes. It returns the
// error that closed the link.
func (n *Node) readLink(c *client, l *link) error {
	for {
		var m Message

		// The progressReader of the link sets the read deadline.
		if err := l.dec.Decode(&m); err != nil {
			l.close(err)
			return err
		}
		if log.WillLog(log.DEBUG) {
			log.Debug("decoded message: %v %v: %v", c.name, l.class, &m)
		}

		if m.Command == ACK {
			// sequence number 0 is a keepalive
			if m.Seq != 0 {
				l.acked(m.Seq)
			}
			continue
		}

		if err := l.ack(m.Seq); err != nil {
			return err
		}

		select {
		case n.messagePump[l.class] <- &m:
		case <-l.done:
			return l.closeErr()
		}
	}
}

// readTimeout returns the read deadline of a link, and how long a message can
// wait for its ACK while the link receives nothing. The peer sends a keepalive
// on an idle link each MSA period, so a link closes after two missed
// keepalives.
func (n *Node) readTimeout() time.Duration {
	return deadlineMultiplier * n.GetMSATimeout()
}

// clientHandler is called as a goroutine after a successful handshake. It
// begins by issuing an MSA. When the control link closes, it closes the other
// links and issues another MSA without the client.
func (n *Node) clientHandler(c *client) {
	log.Debug("clientHandler: %v", c.name)

	n.MSA()

	if c.addr != "" {
		for class := Control + 1; class < numClasses; class++ {
			go n.keepLink(c, class)
		}
	}

	err := n.readLink(c, c.links[Control])
	if err != io.EOF && !strings.Contains(err.Error(), "connection reset by peer") {
		log.Error("client %v decode: %v", c.name, err)
	}
	log.Info("client %v disconnected", c.name)

	close(c.done)
	c.close(errLinkClosed)

	n.clientLock.Lock()
	if n.clients[c.name] == c {
		delete(n.clients, c.name)
	}
	n.clientLock.Unlock()
	go n.checkDegree()

	n.MSA()
}

// keepLink dials the link for class to the peer, and dials it again each time
// it closes, until the control link closes.
func (n *Node) keepLink(c *client, class Class) {
	wait := time.Second

	for {
		l, err := n.dialLink(c, class)
		if err == nil {
			start := time.Now()

			err = n.readLink(c, l)
			c.clearLink(l)

			// the peer can close a link at once, for example for a wrong session
			if time.Since(start) > maxRedialWait {
				wait = time.Second
			}
		}

		select {
		case <-c.done:
			return
		default:
		}

		log.Warn("%v link to %v: %v, dialing again in %v", class, c.name, err, wait)

		select {
		case <-c.done:
			return
		case <-time.After(wait):
		}

		if wait *= 2; wait > maxRedialWait {
			wait = maxRedialWait
		}
	}
}

// serveLink reads a bulk or log link that the peer dialed.
func (n *Node) serveLink(c *client, l *link) {
	go func() {
		select {
		case <-c.done:
			l.close(errLinkClosed)
		case <-l.done:
		}
	}()

	err := n.readLink(c, l)
	c.clearLink(l)
	log.Debug("%v link from %v closed: %v", l.class, c.name, err)
}

// Dicconnect from the specified host.
func (n *Node) Hangup(host string) error {
	log.Debug("hangup: %v", host)

	c, err := n.getClient(host)
	if err != nil {
		return err
	}

	// Close the control link first, so that the other links see that the
	// client is gone and do not dial again.
	c.lock.Lock()
	l := c.links[Control]
	c.lock.Unlock()

	l.close(errLinkClosed)
	return nil
}
