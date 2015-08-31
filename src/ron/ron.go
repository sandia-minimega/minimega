// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"io"
	log "minilog"
	"minitunnel"
	"net"
	"sync"
	"time"
)

// Ron message types to inform the mux on either end how to route the message
const (
	MESSAGE_COMMAND = iota
	MESSAGE_CLIENT
	MESSAGE_TUNNEL
	MESSAGE_FILE
)

const (
	HEARTBEAT_RATE = 5
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
	RESPONSE_PATH  = "miniccc_responses"
)

type Server struct {
	serialConns        map[string]net.Conn // map of connected, but not necessarily active serial connections
	serialLock         sync.Mutex
	udsConns           map[string]net.Listener
	udsLock            sync.Mutex
	commands           map[int]*Command // map of active commands
	commandLock        sync.Mutex
	commandCounter     int
	commandCounterLock sync.Mutex
	clients            map[string]*Client // map of active clients, each of which have a running handler
	clientLock         sync.Mutex
	in                 chan *Message // incoming message queue, consumed by the mux
	path               string        // path for serving files
	lastBroadcast      time.Time     // watchdog time of last command list broadcast
	responses          chan *Client  // queue of incoming responses, consumed by the response processor
}

type Client struct {
	// server client data
	out            chan *Message // outgoing message queue
	in             chan *Message // incoming message queue, consumed by the mux
	path           string        // path for storing files, pid, etc.
	CommandCounter int
	conn           io.ReadWriteCloser
	Checkin        time.Time   // last client checkin time, used by the server
	tunnelData     chan []byte // tunnel data queue, consumed by the tunnel handler
	tunnel         *minitunnel.Tunnel

	// client parameters
	UUID     string
	Hostname string
	Arch     string
	OS       string
	IP       []string
	MAC      []string

	Responses     []*Response   // response queue, consumed and cleared by the heartbeat
	Commands      chan *Command // ordered list of commands to be processed by the client
	responseLock  sync.Mutex
	commands      chan map[int]*Command // unordered, unfiltered list of incoming commands from the server
	lastHeartbeat time.Time             // last heartbeat watchdog time
	files         chan *Message         // incoming files sent by the server and requested by GetFile()
	hold          sync.Mutex            // held while attempting to redial to prevent heartbeats, otherwise they get stacked
}

type Message struct {
	Type     int
	UUID     string
	Commands map[int]*Command
	Client   *Client
	File     []byte
	Filename string
	Error    string
	Tunnel   []byte
}

// NewServer creates a ron server listening on on tcp.
func NewServer(port int, path string) (*Server, error) {
	s := &Server{
		serialConns:   make(map[string]net.Conn),
		udsConns:      make(map[string]net.Listener),
		commands:      make(map[int]*Command),
		clients:       make(map[string]*Client),
		path:          path,
		in:            make(chan *Message, 1024),
		lastBroadcast: time.Now(),
		responses:     make(chan *Client, 1024),
	}
	err := s.Start(port)
	if err != nil {
		return nil, err
	}

	log.Debug("registered new ron server: %v", port)

	return s, nil
}

// NewClient attempts to connect to a ron server over tcp, or serial if the
// serial argument is supplied.
func NewClient(port int, parent, serial, path string) (*Client, error) {
	uuid, err := getUUID()
	if err != nil {
		return nil, err
	}

	c := &Client{
		UUID:          uuid,
		path:          path,
		in:            make(chan *Message, 1024),
		out:           make(chan *Message, 1024),
		Commands:      make(chan *Command, 1024),
		commands:      make(chan map[int]*Command, 1024),
		lastHeartbeat: time.Now(),
		files:         make(chan *Message, 1024),
	}

	if serial != "" {
		err = c.dialSerial(serial)
	} else {
		c.dial(parent, port)
	}
	if err != nil {
		return nil, err
	}

	log.Debug("registered new ron client: %v, %v, %v, %v", port, parent, serial, path)

	return c, nil
}
