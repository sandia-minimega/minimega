// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package meshage

import (
	"bytes"
	"errors"
	"fmt"
	"text/tabwriter"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const (
	ACK = iota
	MSA
	MESSAGE
)

const (
	LOLLIPOP_LENGTH = 16
)

// A message is the payload for all message passing, and contains the user
// specified message in the body field.
type Message struct {
	Recipients   []string    // list of client recipients, unused if broadcasting
	Source       string      // source node name
	Instance     uint64      // ID for the instance, new on restart
	CurrentRoute []string    // list of hops for an in-flight message
	ID           uint64      // sequence ID, uses lollipop sequence numbering
	Command      int         // mesh state announcement, message
	Body         interface{} // message body
}

func (m *Message) String() string {
	// create output
	var o bytes.Buffer
	w := new(tabwriter.Writer)
	w.Init(&o, 5, 0, 1, ' ', 0)
	fmt.Fprintf(&o, "\n")
	fmt.Fprintf(w, "\tSource:\t%v\n", m.Source)
	fmt.Fprintf(w, "\tRecipients:\t%v\n", m.Recipients)
	fmt.Fprintf(w, "\tCurrent Route:\t%v\n", m.CurrentRoute)
	fmt.Fprintf(w, "\tID:\t%v\n", m.ID)
	switch m.Command {
	case ACK:
		fmt.Fprintf(w, "\tCommand:\tACK\n")
	case MSA:
		fmt.Fprintf(w, "\tCommand:\tMSA\n")
	case MESSAGE:
		fmt.Fprintf(w, "\tCommand:\tmessage\n")
	}
	//fmt.Fprintf(w, "\tBody:\t%v", m.Body)
	w.Flush()
	return o.String()
}

// Send a message according to the parameters set in the message.
// Users will generally use the Set and Broadcast functions instead of Send.
// The returned error is always nil if the message type is broadcast.
// If an error is encountered, Send returns immediately.
func (n *Node) Send(m *Message) ([]string, error) {
	if log.WillLog(log.DEBUG) {
		log.Debug("Send: %v", m)
	}

	// force updating the network if needed on Send()
	n.checkUpdateNetwork()

	routeSlices, err := n.getRoutes(m)
	if err != nil {
		return nil, err
	}

	if log.WillLog(log.DEBUG) {
		log.Debug("routeSlices: %v", routeSlices)
	}

	errChan := make(chan error)
	for k, v := range routeSlices {
		go func(client string, recipients []string) {
			mOne := &Message{
				Recipients:   recipients,
				Source:       m.Source,
				Instance:     m.Instance,
				CurrentRoute: m.CurrentRoute,
				Command:      m.Command,
				Body:         m.Body,
			}
			err := n.clientSend(client, mOne)
			if err != nil {
				errChan <- err
			} else {
				errChan <- nil
			}
		}(k, v)
	}

	// wait on all of the client sends to complete
	var ret string
	for i := 0; i < len(routeSlices); i++ {
		r := <-errChan
		if r != nil {
			ret += r.Error() + "\n"
		}
	}

	// Rebuild the recipients from the routeSlices so that the caller can know
	// which recipients were actually valid.
	recipients := []string{}
	for _, r := range routeSlices {
		recipients = append(recipients, r...)
	}

	if ret == "" {
		return recipients, nil
	}

	return recipients, errors.New(ret)
}

func (n *Node) getRoutes(m *Message) (map[string][]string, error) {
	routeSlices := make(map[string][]string)
	n.meshLock.Lock()
	defer n.meshLock.Unlock()

	for _, v := range m.Recipients {
		if v == n.name {
			if len(m.Recipients) == 1 {
				return nil, fmt.Errorf("cannot mesh send yourself")
			}
			continue
		}

		var route string
		var ok bool
		if route, ok = n.routes[v]; !ok {
			log.Warn("no route to host: %v, skipping", v)
			continue
		}
		routeSlices[route] = append(routeSlices[route], v)
	}

	return routeSlices, nil
}

// Set sends a message to a set of nodes. Set blocks until an ACK is received
// from all recipient nodes, or until the timeout is reached.
func (n *Node) Set(recipients []string, body interface{}) ([]string, error) {
	m := &Message{
		Recipients:   recipients,
		Source:       n.name,
		Instance:     n.instance,
		CurrentRoute: []string{n.name},
		Command:      MESSAGE,
		Body:         body,
	}
	return n.Send(m)
}

// Broadcast sends a message to all nodes on the mesh.
func (n *Node) Broadcast(body interface{}) ([]string, error) {
	// force updating the network if needed on Broadcast before looking at
	// the effective network
	m := &Message{
		Recipients:   n.BroadcastRecipients(),
		Source:       n.name,
		Instance:     n.instance,
		CurrentRoute: []string{n.name},
		Command:      MESSAGE,
		Body:         body,
	}
	return n.Send(m)
}

// Determine all the nodes on the mesh that would receive a broadcast message
// from this node. This excludes the node itself as nodes cannot send messages
// to themselves.
func (n *Node) BroadcastRecipients() []string {
	n.checkUpdateNetwork()

	n.meshLock.Lock()
	defer n.meshLock.Unlock()

	var recipients []string
	for k, _ := range n.effectiveNetwork {
		if k != n.name {
			recipients = append(recipients, k)
		}
	}

	return recipients
}

// messageHandler accepts messages from all connected clients and forwards them to the
// appropriate handlers, and to the receiver channel should the message be intended for this
// node.
func (n *Node) messageHandler() {
	log.Debugln("messageHandler")
	for {
		m := <-n.messagePump
		if log.WillLog(log.DEBUG) {
			log.Debug("messageHandler: %v", m)
		}
		m.CurrentRoute = append(m.CurrentRoute, n.name)

		switch m.Command {
		case MSA:
			n.sequenceLock.Lock()
			if m.ID == 1 && n.sequences[m.Instance] > LOLLIPOP_LENGTH {
				n.sequences[m.Instance] = 0
			}
			if m.ID > n.sequences[m.Instance] {
				n.sequences[m.Instance] = m.ID

				go n.handleMSA(m)
				go n.flood(m)
			} else {
				log.Debug("dropping broadcast: %v:%v", m.Source, m.ID)
			}
			n.sequenceLock.Unlock()
		case MESSAGE:
			var newRecipients []string
			runLocal := false
			for _, i := range m.Recipients {
				if i == n.name {
					runLocal = true
				} else {
					newRecipients = append(newRecipients, i)
				}
			}
			m.Recipients = newRecipients

			go n.Send(m)
			if runLocal {
				go n.handleMessage(m)
			} else {
				if n.Snoop != nil {
					go n.Snoop(m)
				}
			}
		default:
			log.Errorln("invalid message command: ", m.Command)
		}
	}
}

func (n *Node) flood(m *Message) {
	if log.WillLog(log.DEBUG) {
		log.Debug("flood: %v", m)
	}

	n.clientLock.Lock()
	defer n.clientLock.Unlock()

floodLoop:
	for k, _ := range n.clients {
		for _, j := range m.CurrentRoute {
			if k == j {
				continue floodLoop
			}
		}
		go func(j string, m *Message) {
			err := n.clientSend(j, m)
			if err != nil {
				// is j still a client?
				if n.hasClient(j) {
					log.Error("flood to client %v: %v", j, err)
				}
			}
		}(k, m)
	}
}

func (n *Node) handleMessage(m *Message) {
	n.receive <- m
}
