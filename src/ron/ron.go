// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"io"
	log "minilog"
	"net"
	"sync"
	"time"
)

const (
	MESSAGE_COMMAND = iota
	MESSAGE_CLIENT
	MESSAGE_TUNNEL
)

const (
	HEARTBEAT_RATE = 10
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
	RESPONSE_PATH  = "miniccc_responses"
)

type Server struct {
	serialConns    map[string]net.Conn
	serialLock     sync.Mutex
	commands       map[int]*Command
	commandLock    sync.Mutex
	commandCounter int
	clients        map[string]*Client
	clientLock     sync.Mutex
	in             chan *Message
	path           string
	lastBroadcast  time.Time
	commandID      chan int
	responses      chan *Client
}

type Client struct {
	// server client data
	out            chan *Message
	in             chan *Message
	path           string
	commandCounter int
	conn           io.ReadWriteCloser
	Checkin        time.Time

	// client parameters
	UUID     string
	Hostname string
	Arch     string
	OS       string
	IP       []string
	MAC      []string

	Responses     []*Response
	Commands      chan *Command
	responseLock  sync.Mutex
	commands      chan map[int]*Command
	lastHeartbeat time.Time
}

type Message struct {
	Type     int
	UUID     string
	Commands map[int]*Command
	Client   *Client
	// Tunnel []byte
}

func NewServer(port int, path string) (*Server, error) {
	s := &Server{
		serialConns:   make(map[string]net.Conn),
		commands:      make(map[int]*Command),
		clients:       make(map[string]*Client),
		path:          path,
		in:            make(chan *Message, 1024),
		lastBroadcast: time.Now(),
		commandID:     make(chan int),
		responses:     make(chan *Client, 1024),
	}
	err := s.Start(port)
	if err != nil {
		return nil, err
	}

	go func() {
		id := 1
		for {
			s.commandID <- id
			id++
		}
	}()

	log.Debug("registered new ron server: %v", port)

	return s, nil
}

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
	}

	if serial != "" {
		err = c.dialSerial(serial)
	} else {
		err = c.dial(parent, port)
	}
	if err != nil {
		return nil, err
	}

	log.Debug("registered new ron client: %v, %v, %v, %v", port, parent, serial, path)

	return c, nil
}
