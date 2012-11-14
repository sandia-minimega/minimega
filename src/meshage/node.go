// Meshage is a fully distributed, mesh based, message passing protocol. It 
// supports completely decentralized message passing, both to a set of nodes 
// as well as broadcast. Meshage is design for resiliency, and automatically 
// updates routes and topologies when nodes in the mesh fail. Meshage 
// automatically maintains density health - as nodes leave the mesh, adjacent 
// nodes will connect to others in the mesh to maintain a minimum degree for 
// resiliency. 
// 
// Meshage is decentralized - Any node in the mesh is capable of initiating and
// receiving messages of any type. This also means that any node is capable of 
// issuing control messages that affect the topology of the mesh.
// 
// Meshage is secure and resilient - All messages are signed and encrypted by 
// the sender to guarantee authenticity and integrity. Nodes on the network 
// store public keys of trusted agents, who may send messages signed and 
// encrypted with a corresponding private key. This is generally done by the 
// end user. Compromised nodes on the mesh that attempt denial of service 
// through discarding messages routed through them are automatically removed 
// from the network by neighbor nodes.  
package meshage

import (
	"fmt"
	"net"
	"sync"
)

const (
	RECEIVE_BUFFER = 1024
	PORT           = 8966
)

const (
	SET = iota
	BROADCAST
	UNION
	INTERSECTION
	MESSAGE
	ACK
	NACK
	HANDSHAKE
)

// Errors
const (
	UNROUTABLE        = iota // host is/was on the network, but is now unroutable
	RETRY_LIMIT              // attempt to send failed too many times
	INVALID_RECIPIENT        // recipient node is not on the network
)

// A Node object contains the network information for a given node. Creating a 
// Node object with a non-zero degree will cause it to begin broadcasting for 
// connections automatically.
type Node struct {
	name                string              // node name. Must be unique on a network.
	degree              uint                // degree for this node, set to 0 to force node to not broadcast
	mesh                map[string][]string // adjacency list for the known topology for this node
	set_sequences       map[string]uint64   // set sequence IDs for each node, including this node
	broadcast_sequences map[string]uint64   // broadcast sequence IDs for each node, including this node
	routes              map[string]string   // one-hop routes for every node on the network, including this node
	receive             chan interface{}    // channel of incoming message bodies. A program will read this channel for incoming messages to this node

	clients     map[string]client // list of connections to this node
	clientLock  sync.Mutex
	messagePump chan Message
}

// A Message is the payload for all message passing, and contains the user 
// specified message in the Body field.
type Message struct {
	Message_type int         // set or broadcast
	Recipients   []string    // list of recipients if message_type = MESSAGE_SET, undefined if broadcast
	Source       string      // name of source node
	CurrentRoute []string    // list of hops for an in-flight message
	ID           uint64      // sequence id
	Command      int         // union, intersection, message, ack
	Body         interface{} // message body
}

// NewNode returns a new node and receiver channel with a given name and 
// degree. If degree is non-zero, the node will automatically begin 
// broadcasting for connections.
func NewNode(name string, degree uint) (Node, chan interface{}) {
	n := Node{
		name:                name,
		degree:              degree,
		mesh:                make(map[string][]string),
		set_sequences:       make(map[string]uint64),
		broadcast_sequences: make(map[string]uint64),
		routes:              make(map[string]string),
		receive:             make(chan interface{}, RECEIVE_BUFFER),
		clients:             make(map[string]client),
		messagePump:         make(chan Message, RECEIVE_BUFFER),
	}
	n.set_sequences[name] = 0
	n.broadcast_sequences[name] = 0
	go n.connectionListener()
	//go n.messageHandler()
	//go n.checkDegree()
	return n, n.receive
}

// connectionListener accepts incoming connections and hands new connections to a connection handler
func (n *Node) connectionListener() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		// TODO: how does meshage handle errors?
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			// TODO: again, how do we deal with this?
		}
		go n.handleConnection(conn)
	}
}

// handleConnection creates a new client and issues a handshake. It adds the client to the list
// of clients only after a successful handshake
func (n *Node) handleConnection(conn net.Conn) {
	c := client{
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}

	// initial handshake
	hs := Message{
		Message_type: SET,
		Recipients:   []string{}, // recipient doesn't matter here as it's expecting this handshake
		Source:       n.Name,
		CurrentRoute: []string{n.Name},
		ID:           0, // special case
		Command:      HANDSHAKE,
		Body:         n.mesh,
	}
	err := c.enc.Encode(hs)
	if err != nil {
		return
	}

	err := c.dec.Decode(hs)
	if err != nil {
		return
	}
	if hs.Command == NACK {
		c.conn.Close()
		return
	}
	// valid connection, add it to the client roster
	n.clientLock.Lock()
	clients[hs.Name] = c
	n.clientLock.Unlock()

	c.decode(n.messagePump)
}

// Degree sets the degree for a given node. Setting degree == 0 will cause the 
// node to stop broadcasting for connections.
func (n *Node) Degree(d uint) {
	n.degree = d
}

// Dial connects a node to another, regardless of degree. Returned error is nil 
// if successful.
func (n *Node) Dial(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		// TODO: error handling
	}
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	var hs Message
	err = dec.Decode(hs)
	if err != nil {
		// TODO: error handling
	}

	if _, ok := n.mesh[hs.Name]; ok {
		// we are already connected to you, no thanks.
		conn.Close()
		return fmt.Errof("already connected")
	}

	resp := Message{
		MessageType: SET,
		Recipients: []string{},
		Source: n.Name,
		CurrentRoute: []string{n.Name},
		ID: 0,
		Command: ACK,
	}
	err = enc.Encode(resp)
	if err != nil {
		// TODO: error handling
	}

	// the network we're connecting to
	mesh := hs.Body.(map[string][]string)
}

// Send a message according to the parameters set in the message. Error is nil 
// if successful. Set messages will block until the message is acknowledged, or 
// receives an error. Broadcast messages will return immediately. 
// Users will generally use the Set and Broadcast methods instead of Send.
func (n *Node) Send(m Message) error {}

// Send a set message heartbeat to all nodes and block until all ACKs have been 
// received. 
func (n *Node) Heartbeat() error {}

// Set sends a set message to a list of recipients. Set blocks until all 
// recipients have acknowledged the message, or returns a non-nil error.
func (n *Node) Set(recipients []string, body interface{}) error {}

// Broadcast sends a broadcast message to all connected nodes. Broadcast does 
// not block.
func (n *Node) Broadcast(body interface{}) {}
