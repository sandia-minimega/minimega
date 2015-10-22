// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	log "minilog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"version"
)

// GetCommands returns a copy of the current command list
func (s *Server) GetCommands() map[int]*Command {
	// return a deep copy of the command list
	ret := make(map[int]*Command)
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	for k, v := range s.commands {
		ret[k] = &Command{
			ID:         v.ID,
			Background: v.Background,
			Command:    v.Command,
			FilesSend:  v.FilesSend,
			FilesRecv:  v.FilesRecv,
			CheckedIn:  v.CheckedIn,
			Filter:     v.Filter,
		}
	}

	log.Debug("ron GetCommands: %v", ret)

	return ret
}

// GetActiveClients returns a list of every active client
func (s *Server) GetActiveClients() map[string]*Client {
	var clients = make(map[string]*Client)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	// deep copy
	for u, c := range s.clients {
		clients[u] = &Client{
			UUID:     c.UUID,
			Hostname: c.Hostname,
			Arch:     c.Arch,
			OS:       c.OS,
		}
		for _, ip := range c.IP {
			clients[u].IP = append(clients[u].IP, ip)
		}
		for _, mac := range c.MAC {
			clients[u].MAC = append(clients[u].MAC, mac)
		}
	}

	log.Debug("ron GetActiveClients: %v", clients)

	return clients
}

// HasClient checks whether a client exists with the given identifier.
func (s *Server) HasClient(c string) bool {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	_, ok := s.clients[c]
	return ok
}

// Starts a Ron server on the specified port
func (s *Server) Start(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(s.path, RESPONSE_PATH), 0775)
	if err != nil {
		return err
	}

	go s.mux()
	go s.handler(ln)
	go s.responseHandler()
	go s.periodic()
	go s.clientReaper()

	return nil
}

// send the command list to all clients periodically, unless the list has been
// sent recently.
func (s *Server) periodic() {
	rate := time.Duration(HEARTBEAT_RATE * time.Second)
	for {
		log.Debugln("ron periodic")
		now := time.Now()
		if now.Sub(s.lastBroadcast) > rate {
			// issue a broadcast
			s.broadcastCommands()
		}
		sleep := rate - now.Sub(s.lastBroadcast)
		time.Sleep(sleep)
	}
}

// send the command list to all active clients
func (s *Server) broadcastCommands() {
	log.Debugln("ron broadcastCommands")
	commands := s.GetCommands()
	m := &Message{
		Type:     MESSAGE_COMMAND,
		Commands: commands,
	}
	s.in <- m
	s.lastBroadcast = time.Now()
}

// accept new tcp connections and start a client handler for each one
func (s *Server) handler(ln net.Listener) {
	log.Debugln("ron handler")
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Errorln(err)
			return
		}

		log.Debug("ron connection from: %v", conn.RemoteAddr())

		go s.clientHandler(conn)
	}
}

// client and transport handler for connections.
func (s *Server) clientHandler(conn io.ReadWriteCloser) {
	log.Debugln("ron clientHandler")

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	// get the first client struct as a handshake
	var handshake Message
	err := dec.Decode(&handshake)
	if err != nil {
		if err != io.EOF {
			log.Errorln(err)
		}
		conn.Close()
		return
	}
	c := handshake.Client

	if c.Version != version.Revision {
		log.Warn("mismatched miniccc version: %v", c.Version)
	}

	c.conn = conn
	c.Checkin = time.Now()

	err = s.addClient(c)
	if err != nil {
		log.Errorln(err)
		conn.Close()
		return
	}

	tunnelQuit := make(chan bool)
	defer func() { tunnelQuit <- true }()

	// create a tunnel connection
	go c.handleTunnel(true, tunnelQuit)

	// handle client i/o
	go func() {
		for {
			m := <-c.out
			if m == nil {
				return
			}
			err := enc.Encode(m)
			if err != nil {
				if err != io.EOF {
					log.Errorln(err)
				}
				s.removeClient(c.UUID)
				return
			}
		}
	}()

	for {
		var m Message
		err := dec.Decode(&m)
		if err != nil {
			if err != io.EOF {
				log.Errorln(err)
			}
			s.removeClient(c.UUID)
			return
		}
		s.in <- &m
	}
}

// add a client to the list of active clients
func (s *Server) addClient(c *Client) error {
	log.Debug("ron addClient: %v", c.UUID)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if _, ok := s.clients[c.UUID]; ok {
		return fmt.Errorf("client %v already exists!", c.UUID)
	}
	c.out = make(chan *Message, 1024)
	s.clients[c.UUID] = c

	return nil
}

// conditionally remove client from the client list, closing connections if
// possible
func (s *Server) removeClient(uuid string) {
	log.Debug("ron removeClient: %v", uuid)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if c, ok := s.clients[uuid]; ok {
		close(c.out)
		c.conn.Close()
		delete(s.clients, uuid)
	}
}

// incoming message mux. Routes messages to the correct handlers based on
// message type
func (s *Server) mux() {
	for {
		m := <-s.in
		switch m.Type {
		case MESSAGE_CLIENT:
			// handle a client response
			log.Debugln("ron MESSAGE_CLIENT")
			s.responses <- m.Client
		case MESSAGE_TUNNEL:
			// handle a tunnel message
			log.Debugln("ron MESSAGE_TUNNEL")
			s.routeTunnel(m)
		case MESSAGE_COMMAND:
			// route a command to one or all clients
			log.Debugln("ron MESSAGE_COMMAND")
			s.route(m)
		case MESSAGE_FILE:
			// send a file if it exists
			s.sendFile(m)
		default:
			log.Error("unknown message type: %v", m.Type)
			return
		}
	}
}

// unwrap ron messages and forward tunnel data to the tunnel handler
func (s *Server) routeTunnel(m *Message) {
	log.Debug("routeTunnel: %v", m.UUID)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	for _, c := range s.clients {
		if c.UUID == m.UUID {
			c.tunnelData <- m.Tunnel
			return
		}
	}
	log.Error("routeTunnel invalid UUID: %v", m.UUID)
}

// return a file to a client requesting it via the clients GetFile() call
func (s *Server) sendFile(m *Message) {
	log.Debug("ron sendFile: %v", m.Filename)

	filename := filepath.Join(s.path, m.Filename)
	info, err := os.Stat(filename)
	if err != nil {
		e := fmt.Errorf("file %v does not exist: %v", filename, err)
		m.Error = e.Error()
		log.Errorln(e)
	} else if info.IsDir() {
		e := fmt.Errorf("file %v is a directory", filename)
		m.Error = e.Error()
		log.Errorln(e)
	} else {
		// read the file
		m.File, err = ioutil.ReadFile(filename)
		if err != nil {
			e := fmt.Errorf("file %v: %v", filename, err)
			m.Error = e.Error()
			log.Errorln(e)
		}
	}

	// route this message ourselves instead of using the mux, because we
	// want the type to still be FILE
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if c, ok := s.clients[m.UUID]; ok {
		c.out <- m
	} else {
		log.Error("no such client %v", m.UUID)
	}
}

// route an outgoing message to one or all clients, according to UUID
func (s *Server) route(m *Message) {
	log.Debug("ron route: %v", m.UUID)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if m.UUID == "" {
		// all clients
		for _, c := range s.clients {
			c.out <- m
		}
	} else {
		if c, ok := s.clients[m.UUID]; ok {
			c.out <- m
		} else {
			log.Error("no such client %v", m.UUID)
		}
	}
}

// process responses, writing files when necessary
func (s *Server) responseHandler() {
	for {
		cin := <-s.responses

		log.Debug("ron responseHandler: %v", cin.UUID)

		// update maximum command id if there's a higher one in the wild
		if cin.CommandCounter > s.commandCounter {
			s.commandCounterLock.Lock()
			s.commandCounter = cin.CommandCounter
			s.commandCounterLock.Unlock()
		}

		// update client fields
		s.clientLock.Lock()
		if c, ok := s.clients[cin.UUID]; ok {
			c.Hostname = cin.Hostname
			c.Arch = cin.Arch
			c.OS = cin.OS
			c.IP = cin.IP
			c.MAC = cin.MAC
			c.Checkin = time.Now()
		} else {
			log.Error("unknown client %v", cin.UUID)
			s.clientLock.Unlock()
			continue
		}
		s.clientLock.Unlock()

		// ingest responses from this client
		for _, v := range cin.Responses {
			log.Debug("got response %v : %v", cin.UUID, v.ID)
			s.commandCheckIn(v.ID, cin.UUID)

			path := filepath.Join(s.path, RESPONSE_PATH, strconv.Itoa(v.ID), cin.UUID)
			err := os.MkdirAll(path, os.FileMode(0770))
			if err != nil {
				log.Errorln(err)
				log.Error("could not record response %v : %v", cin.UUID, v.ID)
				continue
			}
			// generate stdout and stderr if they exist
			if v.Stdout != "" {
				err := ioutil.WriteFile(filepath.Join(path, "stdout"), []byte(v.Stdout), os.FileMode(0660))
				if err != nil {
					log.Errorln(err)
					log.Error("could not record stdout %v : %v", cin.UUID, v.ID)
				}
			}
			if v.Stderr != "" {
				err := ioutil.WriteFile(filepath.Join(path, "stderr"), []byte(v.Stderr), os.FileMode(0660))
				if err != nil {
					log.Errorln(err)
					log.Error("could not record stderr %v : %v", cin.UUID, v.ID)
				}
			}

			// write out files if they exist
			for _, f := range v.Files {
				fpath := filepath.Join(path, f.Name)
				log.Debug("writing file %v", fpath)
				dir := filepath.Dir(fpath)
				err := os.MkdirAll(dir, os.FileMode(0770))
				if err != nil {
					log.Errorln(err)
					continue
				}
				err = ioutil.WriteFile(fpath, f.Data, f.Perm)
				if err != nil {
					log.Errorln(err)
					continue
				}
			}
		}
	}
}

// mark which commands have been responsed to by which client
func (s *Server) commandCheckIn(id int, uuid string) {
	log.Debug("commandCheckIn %v %v", id, uuid)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	if c, ok := s.commands[id]; ok {
		c.CheckedIn = append(c.CheckedIn, uuid)
	} else {
		log.Error("ron command checkin: command %v does not exist", id)
	}
}

// DeleteCommand removes a command from the active command list. Any in-flight
// messages held by any clients may still return a response to the deleted
// command.
func (s *Server) DeleteCommand(id int) error {
	log.Debug("ron DeleteCommand: %v", id)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()
	if _, ok := s.commands[id]; ok {
		delete(s.commands, id)
		return nil
	} else {
		return fmt.Errorf("command %v not found", id)
	}
}

// Post a new command to the active command list. The command ID is returned.
func (s *Server) NewCommand(c *Command) int {
	log.Debug("ron NewCommand: %v", c)

	s.commandCounterLock.Lock()
	s.commandCounter++
	c.ID = s.commandCounter
	s.commandCounterLock.Unlock()

	s.commandLock.Lock()
	s.commands[c.ID] = c
	s.commandLock.Unlock()
	go s.broadcastCommands()
	return c.ID
}

// clientReaper periodically flushes old entries from the client list
func (s *Server) clientReaper() {
	for {
		time.Sleep(time.Duration(REAPER_RATE) * time.Second)
		log.Debugln("clientReaper")
		t := time.Now()
		s.clientLock.Lock()
		for k, v := range s.clients {
			if t.Sub(v.Checkin) > time.Duration(CLIENT_EXPIRED*time.Second) {
				log.Debug("client %v expired", k)
				go s.removeClient(k) // hack: put this in a goroutine to simplify locking
			}
		}
		s.clientLock.Unlock()
	}
}

// Return the list of currently connected serial ports. This does not indicate
// which serial connections have active clients, simply which serial
// connections the server is attached to.
func (s *Server) GetActiveSerialPorts() []string {
	s.serialLock.Lock()
	defer s.serialLock.Unlock()

	var ret []string
	for k, _ := range s.serialConns {
		ret = append(ret, k)
	}

	log.Debug("ron GetActiveSerialPorts: %v", ret)

	return ret
}

// Return the list of currently listening UDS ports. This does not indicate
// which connections have active clients, simply which connections the server
// is attached to.
func (s *Server) GetActiveUDSPorts() []string {
	s.udsLock.Lock()
	defer s.udsLock.Unlock()

	var ret []string
	for k, _ := range s.udsConns {
		ret = append(ret, k)
	}

	log.Debug("ron GetActiveUDSPorts: %v", ret)

	return ret
}

func (s *Server) CloseUDS(path string) error {
	s.udsLock.Lock()
	defer s.udsLock.Unlock()

	if l, ok := s.udsConns[path]; ok {
		return l.Close()
	} else {
		return fmt.Errorf("no such path: %v", path)
	}
}

// ListenUnix creates a unix domain socket at the given path and listens for
// incoming connections. ListenUnix returns on the successful creation of the
// socket, and accepts connections in a goroutine. If the domain socket file is
// deleted, the goroutine for ListenUnix exists silently.
func (s *Server) ListenUnix(path string) error {
	log.Debug("ListenUnix: %v", path)

	s.udsLock.Lock()
	defer s.udsLock.Unlock()

	u, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return err
	}

	l, err := net.ListenUnix("unix", u)
	if err != nil {
		return err
	}
	s.udsConns[path] = l

	go func() {
		defer s.CloseUDS(path)
		for {
			l.SetDeadline(time.Now().Add(time.Second))
			conn, err := l.Accept()
			if err != nil {
				if strings.Contains(err.Error(), "timeout") {
					_, err := os.Stat(path)
					if err != nil {
						return
					} else {
						continue
					}
				} else {
					if !strings.Contains(err.Error(), "use of closed network connection") {
						log.Error("ListenUnix: accept: %v", err)
					}
					return
				}
			}
			log.Info("client connected on %v", path)
			s.clientHandler(conn)
			log.Info("client disconnected on %v", path)
		}
	}()
	return nil
}

// Dial a client serial port. The server will maintain this connection until a
// client connects and then disconnects.
func (s *Server) DialSerial(path string) error {
	log.Debug("DialSerial: %v", path)

	s.serialLock.Lock()
	defer s.serialLock.Unlock()

	// are we already connected to this client?
	if _, ok := s.serialConns[path]; ok {
		return fmt.Errorf("already connected to serial client %v", path)
	}

	// connect!
	serial, err := net.Dial("unix", path)
	if err != nil {
		return err
	}

	s.serialConns[path] = serial
	go func() {
		s.clientHandler(serial)
		s.serialLock.Lock()
		delete(s.serialConns, path)
		s.serialLock.Unlock()
		log.Debug("disconnected from serial client: %v", path)
	}()

	return nil
}
