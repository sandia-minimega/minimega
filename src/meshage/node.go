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
	"encoding/gob"
	"fmt"
	"net"
	"sort"
	"sync"
	log "minilog"
	"io"
	"strings"
	"time"
	"math/rand"
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
	HANDSHAKE_SOLICITED
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
	name               string              // node name. Must be unique on a network.
	degree             uint                // degree for this node, set to 0 to force node to not broadcast
	mesh               map[string][]string // adjacency list for the known topology for this node
	setSequences       map[string]uint64   // set sequence IDs for each node, including this node
	broadcastSequences map[string]uint64   // broadcast sequence IDs for each node, including this node
	routes             map[string]string   // one-hop routes for every node on the network, including this node
	receive            chan Message        // channel of incoming messages. A program will read this channel for incoming messages to this node

	clients      map[string]client // list of connections to this node
	clientLock   sync.Mutex
	sequenceLock sync.Mutex
	meshLock     sync.Mutex
	degreeLock	sync.Mutex
	messagePump  chan Message

	errors chan error
}

// A Message is the payload for all message passing, and contains the user 
// specified message in the Body field.
type Message struct {
	MessageType  int         // set or broadcast
	Recipients   []string    // list of recipients if MessageType = MESSAGE_SET, undefined if broadcast
	Source       string      // name of source node
	CurrentRoute []string    // list of hops for an in-flight message
	ID           uint64      // sequence id
	Command      int         // union, intersection, message, ack
	Body         interface{} // message body
}

func init() {
	gob.Register(map[string][]string{})
}

// NewNode returns a new node and receiver channel with a given name and 
// degree. If degree is non-zero, the node will automatically begin 
// broadcasting for connections.
func NewNode(name string, degree uint) (Node, chan Message, chan error) {
	n := Node{
		name:               name,
		degree:             degree,
		mesh:               make(map[string][]string),
		setSequences:       make(map[string]uint64),
		broadcastSequences: make(map[string]uint64),
		routes:             make(map[string]string),
		receive:            make(chan Message, RECEIVE_BUFFER),
		clients:            make(map[string]client),
		messagePump:        make(chan Message, RECEIVE_BUFFER),
		errors:             make(chan error),
	}
	n.setSequences[name] = 1
	n.broadcastSequences[name] = 1
	go n.connectionListener()
	go n.broadcastListener()
	go n.messageHandler()
	go n.checkDegree()
	return n, n.receive, n.errors
}

// check degree emits connection requests when our number of connected clients is below the degree threshold
func (n *Node) checkDegree() {
	var backoff uint = 1
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	for uint(len(n.clients)) < n.degree {
		log.Debugln("soliciting connections")
		b := net.IPv4(255,255,255,255)
		addr := net.UDPAddr{
			IP: b,
			Port: PORT,
		}
		socket, err := net.DialUDP("udp4", nil, &addr)
		if err != nil {
			log.Errorln(err)
			n.errors <- err
			break
		}
		_, err = socket.Write([]byte("meshage"))
		if err != nil {
			log.Errorln(err)
			n.errors <- err
			break
		}
		wait := r.Intn(1<<backoff)
		time.Sleep(time.Duration(wait) * time.Second)
		if (backoff < 7) { // maximum wait won't exceed 128 seconds
			backoff++
		}
	}
}

// broadcastListener listens for broadcast connection requests and attempts to connect to that node
func (n *Node) broadcastListener() {
	listenAddr := net.UDPAddr{
		IP: net.IPv4(0,0,0,0),
		Port: PORT,
	}
	ln, err := net.ListenUDP("udp4", &listenAddr)
	if err != nil {
		log.Errorln(err)
		n.errors <- err
		return
	}
	for {
		data := make([]byte, 7)
		_, remoteAddr, err := ln.ReadFromUDP(data)
		if string(data) != "meshage" {
			err = fmt.Errorf("got malformed udp data: %v\n", string(data))
			log.Errorln(err)
			n.errors <- err
			continue
		}
		addr := remoteAddr.String()
		f := strings.Split(addr, ":")
		if len(f) != 2 {
			err = fmt.Errorf("malformed host: %v\n", remoteAddr)
			n.errors <- err
		}
		host := f[0]
		if host == n.name {
			continue
		}
		log.Debug("got solicitation from %v\n", host)
		go n.dial(host, true)
	}
}

// connectionListener accepts incoming connections and hands new connections to a connection handler
func (n *Node) connectionListener() {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", PORT))
	if err != nil {
		n.errors <- err
		return
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorln(err)
			n.errors <- err
			continue
		}
		n.handleConnection(conn)
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

	log.Debug("got conn: %v\n", conn)

	var command int
	if uint(len(n.clients)) < n.degree {
		command = HANDSHAKE_SOLICITED
	} else {
		command = HANDSHAKE
	}

	// initial handshake
	hs := Message{
		MessageType:  SET,
		Recipients:   []string{}, // recipient doesn't matter here as it's expecting this handshake
		Source:       n.name,
		CurrentRoute: []string{n.name},
		ID:           0, // special case
		Command:      command,
		Body:         n.mesh,
	}
	err := c.enc.Encode(hs)
	if err != nil {
		if err != io.EOF {
			log.Errorln(err)
			n.errors <- err
		}
		return
	}

	err = c.dec.Decode(&hs)
	if err != nil {
		if err != io.EOF {
			log.Errorln(err)
			n.errors <- err
		}
		return
	}

	// valid connection, add it to the client roster
	n.clientLock.Lock()
	n.clients[hs.Source] = c
	n.clientLock.Unlock()

	go n.receiveHandler(hs.Source)
}

func (n *Node) receiveHandler(client string) {
	c := n.clients[client]

	for {
		var m Message
		err := c.dec.Decode(&m)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
				n.errors <- err
			}
			break
		} else {
			log.Debug("receiveHandler got: %v\n", m)
			n.messagePump <- m
		}
	}
	n.clientLock.Lock()
	delete(n.clients, client)
	n.clientLock.Unlock()
}

// SetDegree sets the degree for a given node. Setting degree == 0 will cause the 
// node to stop broadcasting for connections.
func (n *Node) SetDegree(d uint) {
	n.degree = d
}

// Degree returns the current degree
func (n *Node) Degree() uint {
	return n.degree
}

// Dial connects a node to another, regardless of degree. Returned error is nil 
// if successful.
func (n *Node) Dial(addr string) error {
	return n.dial(addr,false)
}

func (n *Node) dial(addr string, solicited bool) error {
	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", addr, PORT))
	if err != nil {
		return err
	}
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	var hs Message
	err = dec.Decode(&hs)
	if err != nil {
		return err
	}
	log.Debug("Dial got: %v\n", hs)

	if _, ok := n.clients[hs.Source]; ok {
		// we are already connected to you, no thanks.
		conn.Close()
		log.Errorln("already connected")
		return fmt.Errorf("already connected")
	}

	// were we solicited?
	if hs.Command == HANDSHAKE && solicited {
		conn.Close()
		return nil
	}

	resp := Message{
		MessageType:  SET,
		Recipients:   []string{},
		Source:       n.name,
		CurrentRoute: []string{n.name},
		ID:           0,
		Command:      ACK,
	}
	err = enc.Encode(resp)
	if err != nil {
		return err
	}

	// add this client to our client list
	c := client{
		conn: conn,
		enc:  enc,
		dec:  dec,
	}

	n.clientLock.Lock()
	n.clients[hs.Source] = c
	n.clientLock.Unlock()
	go n.receiveHandler(hs.Source)

	// the network we're connecting to
	mesh := hs.Body.(map[string][]string)

	// add this new connection to the mesh and union with our mesh
	mesh[n.name] = append(mesh[n.name], hs.Source)
	mesh[hs.Source] = append(mesh[hs.Source], n.name)
	n.union(mesh)

	// let everyone know about the new topology
	u := Message{
		MessageType:  BROADCAST,
		Source:       n.name,
		CurrentRoute: []string{n.name},
		ID:           n.broadcastID(),
		Command:      UNION,
		Body:         n.mesh,
	}
	log.Debug("Dial broadcasting topology: %v\n", u)
	n.Send(u)
	return nil
}

// union merges a mesh with the local one and eliminates redundant connections
func (n *Node) union(m map[string][]string) {
	log.Debug("union mesh: %v\n", m)
	n.meshLock.Lock()
	defer n.meshLock.Unlock()

	// merge everything, sort each bin, and eliminate duplicate entries
	for k, v := range m {
		n.mesh[k] = append(n.mesh[k], v...)
		sort.Strings(n.mesh[k])
		var nl []string
		for _, j := range n.mesh[k] {
			if len(nl) == 0 {
				nl = append(nl, j)
				continue
			}
			if nl[len(nl)-1] != j {
				nl = append(nl, j)
			}
		}
		n.mesh[k] = nl
	}
	log.Debug("new mesh is: %v\n", n.mesh)
}

// Send a message according to the parameters set in the message. Error is nil 
// if successful. Set messages will block until the message is acknowledged, or 
// receives an error. Broadcast messages will return immediately. 
// Users will generally use the Set and Broadcast methods instead of Send.
func (n *Node) Send(m Message) {
	log.Debug("Send: %v\n", m)
	switch m.MessageType {
	case SET:
		n.setSend(m)
	case BROADCAST:
		n.broadcastSend(m)
	default:
		log.Errorln("Send: invalid message type")
		n.errors <- fmt.Errorf("Send: invalid message type")
	}
}

// setSend sends a set type message according to known routes
func (n *Node) setSend(m Message) {
}

// broadcastSend sends a broadcast message to all connected clients
func (n *Node) broadcastSend(m Message) {
	for k, c := range n.clients {
		log.Debug("broadcasting to: %v : %v\n", k, m)
		err := c.send(m)
		if err != nil {
			log.Errorln(err)
			n.errors <- err
		}
	}
}

// Send a set message heartbeat to all nodes and block until all ACKs have been 
// received. 
func (n *Node) Heartbeat() error {
	return nil
}

// Set sends a set message to a list of recipients. Set blocks until all 
// recipients have acknowledged the message, or returns a non-nil error.
func (n *Node) Set(recipients []string, body interface{}) error {
	return nil
}

// Broadcast sends a broadcast message to all connected nodes. Broadcast does 
// not block.
func (n *Node) Broadcast(body interface{}) {}

// Return a broadcast ID for this node and automatically increment the ID
func (n *Node) broadcastID() uint64 {
	n.sequenceLock.Lock()
	id := n.broadcastSequences[n.name]
	n.broadcastSequences[n.name]++
	log.Debug("broadcast id: %v", n.broadcastSequences[n.name])
	n.sequenceLock.Unlock()
	return id
}

// Return a set ID for this node and automatically increment the ID
func (n *Node) setID() uint64 {
	n.sequenceLock.Lock()
	id := n.setSequences[n.name]
	n.setSequences[n.name]++
	log.Debug("set id: %v", n.setSequences[n.name])
	n.sequenceLock.Unlock()
	return id
}

// messageHandler receives messages on a channel from any clients and processes them.
// Some messages are rebroadcast, or sent along other routes. Messages intended for this
// node are sent along the receive channel to the user.
func (n *Node) messageHandler() {
	for {
		m := <-n.messagePump
		log.Debug("messageHandler: %v\n", m)
		switch m.MessageType {
		case SET:
			// shoudl we handle this or drop it?
			if n.setSequences[m.Source] < m.ID {
				// it's a new message to us
				n.sequenceLock.Lock()
				n.setSequences[m.Source] = m.ID
				n.sequenceLock.Unlock()
				m.CurrentRoute = append(m.CurrentRoute, n.name)

				go n.setSend(m)

				// do we also handle it?
				for _, i := range m.Recipients {
					if i == n.name {
						n.handleMessage(m)
						break
					}
				}
			}
		case BROADCAST:
			// should we handle this or drop it?
			if n.broadcastSequences[m.Source] < m.ID {
				// it's a new message to us
				n.sequenceLock.Lock()
				n.broadcastSequences[m.Source] = m.ID
				n.sequenceLock.Unlock()
				// update the route information
				m.CurrentRoute = append(m.CurrentRoute, n.name)
				go n.broadcastSend(m)
				n.handleMessage(m)
			}
		}
	}
}

// handleMessage parses a message intended for this node.
// If the message is a control message, we process it here, if it's
// a regular message, we put it on the receive channel.
func (n *Node) handleMessage(m Message) {
	log.Debug("handleMessage: %v\n", m)
	switch m.Command {
	case UNION:
		n.union(m.Body.(map[string][]string))
	case INTERSECTION:
	case MESSAGE:
		n.receive <- m
	case ACK:
	case NACK:
	default:
		err := fmt.Errorf("handleMessage: invalid message type")
		log.Errorln(err)
		n.errors <- err 
	}
}
