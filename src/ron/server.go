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
	"minitunnel"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"version"
)

// GetCommand returns copy of a command by ID or nil if it doesn't exist
func (s *Server) GetCommand(id int) *Command {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	if v, ok := s.commands[id]; ok {
		return v.Copy()
	}

	return nil
}

// GetCommands returns a deep copy of the current command list
func (s *Server) GetCommands() map[int]*Command {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	res := make(map[int]*Command)

	for k, v := range s.commands {
		res[k] = v.Copy()
	}

	return res
}

func (s *Server) GetProcesses(uuid string) ([]*Process, error) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	var res []*Process

	c, ok := s.clients[uuid]
	if !ok {
		return nil, fmt.Errorf("no client with uuid: %v", uuid)
	}

	// ordered list of pids
	var pids []int
	for k, _ := range c.Processes {
		pids = append(pids, k)
	}
	sort.Ints(pids)

	for _, v := range pids {
		res = append(res, c.Processes[v])
	}
	return res, nil
}

// GetActiveClients returns a list of every active client
func (s *Server) GetActiveClients() map[string]*Client {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	res := make(map[string]*Client)

	// deep copy
	for u, c := range s.clients {
		res[u] = &Client{
			UUID:           c.UUID,
			Arch:           c.Arch,
			OS:             c.OS,
			Version:        c.Version,
			Hostname:       c.Hostname,
			CommandCounter: c.CommandCounter,
			Processes:      make(map[int]*Process),
		}
		for _, ip := range c.IPs {
			res[u].IPs = append(res[u].IPs, ip)
		}
		for _, mac := range c.MACs {
			res[u].MACs = append(res[u].MACs, mac)
		}
		for k, v := range c.Processes {
			res[u].Processes[k] = v
		}
	}

	return res
}

// HasClient checks whether a client exists with the given identifier.
func (s *Server) HasClient(c string) bool {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	_, ok := s.clients[c]
	return ok
}

// Starts a Ron server on the specified port
func (s *Server) Listen(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		return err
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Errorln(err)
				return
			}

			log.Debug("new connection from: %v", conn.RemoteAddr())

			go s.clientHandler(conn)
		}
	}()

	return nil
}

// ListenUnix creates a unix domain socket at the given path and listens for
// incoming connections. ListenUnix returns on the successful creation of the
// socket, and accepts connections in a goroutine. If the domain socket file is
// deleted, the goroutine for ListenUnix exits silently.
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
		defer func() {
			s.udsLock.Lock()
			defer s.udsLock.Unlock()

			if l, ok := s.udsConns[path]; ok {
				l.Close()
			}
		}()

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

		// client disconnected
		s.serialLock.Lock()
		defer s.serialLock.Unlock()

		delete(s.serialConns, path)
		log.Debug("disconnected from serial client: %v", path)
	}()

	return nil
}

func (s *Server) RegisterVM(uuid string, f VM) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	s.vms[uuid] = f
}

func (s *Server) UnregisterVM(uuid string) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	delete(s.vms, uuid)
}

// send the command list to all clients periodically, unless the list has been
// sent recently.
func (s *Server) periodic() {
	rate := time.Duration(HEARTBEAT_RATE * time.Second)
	for {
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
	commands := s.GetCommands()
	m := &Message{
		Type:     MESSAGE_COMMAND,
		Commands: commands,
	}
	s.route(m)
	s.lastBroadcast = time.Now()
}

// client and transport handler for connections.
func (s *Server) clientHandler(conn net.Conn) {
	log.Debug("clientHandler to %v", conn.RemoteAddr())

	defer conn.Close()

	c := &client{
		conn: conn,
		enc:  gob.NewEncoder(conn),
		dec:  gob.NewDecoder(conn),
	}

	// get the first client struct as a handshake
	var handshake Message
	if err := c.dec.Decode(&handshake); err != nil {
		//if err != io.EOF {
		log.Errorln(err)
		//}
		return
	}
	c.Client = handshake.Client
	log.Debug("new client: %v", handshake.Client)

	if c.Version != version.Revision {
		log.Warn("mismatched miniccc version: %v", c.Version)
	}

	// Set up minitunnel, dialing the server that should be running on the
	// client's side. Data is Trunk'd via Messages.
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	go func() {
		go Trunk(remote, c.UUID, c.sendMessage)

		tunnel, err := minitunnel.Dial(local)
		if err != nil {
			log.Error("dial: %v", err)
			return
		}

		s.clientLock.Lock()
		defer s.clientLock.Unlock()

		log.Info("minitunnel created for %v", c.UUID)
		c.tunnel = tunnel
	}()

	c.Checkin = time.Now()

	if err := s.addClient(c); err != nil {
		log.Errorln(err)
		return
	}
	defer s.removeClient(c.UUID)

	var err error

	for err == nil {
		var m Message
		if err = c.dec.Decode(&m); err == nil {
			log.Debug("new message: %v", m.Type)

			switch m.Type {
			case MESSAGE_TUNNEL:
				_, err = remote.Write(m.Tunnel)
			case MESSAGE_FILE:
				m2 := s.readFile(m.Filename)
				m2.UUID = m.UUID
				err = c.sendMessage(m2)
			case MESSAGE_CLIENT:
				s.responses <- m.Client
			case MESSAGE_COMMAND:
				// this shouldn't be sent via the client...
			default:
				err = fmt.Errorf("unknown message type: %v", m.Type)
			}
		}
	}

	if err != io.EOF && !strings.Contains(err.Error(), "connection reset by peer") {
		log.Errorln(err)
	}

	log.Info("clientHandler exit: %v", conn.RemoteAddr())
}

// addClient adds a client to the list of active clients
func (s *Server) addClient(c *client) error {
	log.Debug("addClient: %v", c.UUID)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if _, ok := s.clients[c.UUID]; ok {
		return fmt.Errorf("client %v already exists!", c.UUID)
	}
	s.clients[c.UUID] = c

	return nil
}

// removeClient conditionally remove client from the client list, closing
// connections if possible.
func (s *Server) removeClient(uuid string) {
	log.Debug("removeClient: %v", uuid)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if c, ok := s.clients[uuid]; ok {
		c.conn.Close()
		delete(s.clients, uuid)
	}
}

// readFile reads the file by name and returns a message that can be sent back
// to the client.
func (s *Server) readFile(f string) *Message {
	log.Debug("readFile: %v", f)

	filename := filepath.Join(s.path, f)
	m := &Message{
		Type:     MESSAGE_FILE,
		Filename: f,
	}

	info, err := os.Stat(filename)
	if err != nil {
		m.Error = fmt.Sprintf("file %v does not exist: %v", filename, err)
	} else if info.IsDir() {
		m.Error = fmt.Sprintf("file %v is a directory", filename)
	} else {
		// read the file
		m.File, err = ioutil.ReadFile(filename)
		if err != nil {
			m.Error = fmt.Sprintf("file %v: %v", filename, err)
		}
	}

	if m.Error != "" {
		log.Errorln(m.Error)
	}

	return m
}

// route an outgoing message to one or all clients, according to UUID
func (s *Server) route(m *Message) {
	handleUUID := func(uuid string) {
		c, ok := s.clients[uuid]
		if !ok {
			log.Error("no such client %v", uuid)
			return
		}

		vm, ok := s.vms[uuid]
		if !ok {
			// The client is connected but not registered:
			//  * client connected before it was registered
			//  * client was unregistered before it disconnected
			// Either way, we have to skip it since we don't know what
			// namespace it belongs to.
			log.Error("unregistered client %v", uuid)
			return
		}

		// Create a copy of the Message
		m2 := *m
		m2.Tags = vm.GetTags()
		m2.Namespace = vm.GetNamespace()

		if err := c.sendMessage(&m2); err != nil {
			log.Error("unable to send message to %v: %v", uuid, err)
		}
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if m.UUID != "" {
		handleUUID(m.UUID)
		return
	}

	var wg sync.WaitGroup

	// send commands to all clients, in parallel
	for uuid := range s.clients {
		wg.Add(1)

		go func(uuid string) {
			defer wg.Done()

			handleUUID(uuid)
		}(uuid)
	}

	wg.Wait()
}

// process responses, writing files when necessary
func (s *Server) responseHandler() {
	for {
		cin := <-s.responses

		log.Debug("responseHandler: %v", cin.UUID)

		// update maximum command id if there's a higher one in the wild
		if cin.CommandCounter > s.commandCounter {
			s.commandCounterLock.Lock()
			s.commandCounter = cin.CommandCounter
			s.commandCounterLock.Unlock()
		}

		// update all the client fields
		s.updateClient(cin)

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

// updateClient updates the client fields and pushes the VM tags state
func (s *Server) updateClient(cin *Client) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	c, ok := s.clients[cin.UUID]
	if !ok {
		log.Error("unknown client %v", cin.UUID)
		return
	}

	c.Client = cin
	c.Checkin = time.Now()

	vm, ok := s.vms[cin.UUID]
	if !ok {
		log.Error("unregistered client %v", cin.UUID)
		return
	}

	for k, v := range cin.Tags {
		vm.SetTag(k, v)
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
		log.Error("command checkin: command %v does not exist", id)
	}
}

// DeleteCommand removes a command from the active command list. Any in-flight
// messages held by any clients may still return a response to the deleted
// command.
func (s *Server) DeleteCommand(id int) error {
	log.Debug("delete command: %v", id)

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
	log.Debug("NewCommand: %v", c)

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

		s.clientLock.Lock()
		for k, v := range s.clients {
			// checked in more recently than expiration time
			active := time.Now().Sub(v.Checkin) < CLIENT_EXPIRED*time.Second

			if !active {
				log.Debug("client %v expired", k)
				go s.removeClient(k) // hack: put this in a goroutine to simplify locking
			}

			if vm, ok := s.vms[k]; ok {
				vm.SetCCActive(active)
			}
		}
		s.clientLock.Unlock()
	}
}
