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
	"time"
)

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

// return a copy of each active client
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

	c.conn = conn

	err = s.addClient(c)
	if err != nil {
		log.Errorln(err)
		conn.Close()
		return
	}

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

// conditionally remove client from the client list
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

func (s *Server) responseHandler() {
	for {
		cin := <-s.responses

		log.Debug("ron responseHandler: %v", cin.UUID)

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
			for f, d := range v.Files {
				fpath := filepath.Join(path, f)
				log.Debug("writing file %v", fpath)
				dir := filepath.Dir(fpath)
				err := os.MkdirAll(dir, os.FileMode(0770))
				if err != nil {
					log.Errorln(err)
					continue
				}
				err = ioutil.WriteFile(fpath, d, os.FileMode(0660))
				if err != nil {
					log.Errorln(err)
					continue
				}
			}
		}
	}
}

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

func (s *Server) NewCommand(c *Command) int {
	log.Debug("ron NewCommand: %v", c)

	c.ID = <-s.commandID
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

// Dial a client serial port. Used by a master ron node only.
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
