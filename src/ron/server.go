// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	log "minilog"
	"io"
	"time"
)

func (s *Server) GetCommands() map[int]*Command {
	// return a deep copy of the command list
	ret := make(map[int]*Command)
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	for k, v := range s.commands {
		ret[k] = &Command{
			ID:             v.ID,
			Background:     v.Background,
			Command:        v.Command,
			FilesSend:      v.FilesSend,
			FilesRecv:      v.FilesRecv,
			CheckedIn:      v.CheckedIn,
			Filter: v.Filter,
		}
	}
	return ret
}

// return a copy of each active client
func (s *Server) GetActiveClients() map[string]*Client {
	var clients = make(map[string]*Client)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	// deep copy
	for u, c := range r.clients {
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

	log.Debug("active clients: %v", clients)
	return clients
}

// Starts a Ron server on the specified port
func (s *Server) Start(port int) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%v", port))
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(r.path, RESPONSE_PATH), 0775)
	if err != nil {
		return err
	}

	go s.mux()
	go s.handler(ln)
	go s.responseHandler()
	go s.periodic()
}

// send the command list to all clients periodically, unless the list has been
// sent recently.
func (s *Server) periodic() {
	rate := time.Duration(HEARTBEAT_RATE * time.Second)
	for {
		now := time.Now()
		if s.lastBroadcast.Sub(now) > rate {
			// issue a broadcast
			s.broadcastCommands()
		}
		sleep := rate - s.lastBroadcast.Sub(now)
		time.Sleep(sleep)
	}
}

func (s *Server) broadcastCommands() {
	commands := s.GetCommands()
	m := &Message{
		Type: MESSSAGE_COMMAND,
		Commands: commands,
	}
	s.in <- m
	s.lastBroadcast = time.Now()
}

func (s *Server) handler(ln net.Listener) {
	log.Debugln("handler")
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
	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	// get the first client struct as a handshake
	var c Client
	dec.Decode(&c)

	c.conn = conn

	err := s.addClient(c)
	if err != nil {
		log.Errorln(err)
		conn.Close()
		return
	}

	// handle client i/o
	go func() {
		for {
			m := <-c.out
			err := enc.Encode(m)
			if err != nil {
				log.Errorln(err)
				s.removeClient(c.UUID)
				return
			}
		}
	}()

	for {
		var m Message
		err := dec.Decode(&m)
		if err != nil {
			log.Errorln(err)
			s.removeClient(c.UUID)
			return
		}
		s.in <- &m
	}
}

func (s *Server) addClient(c *Client) error {
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
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	if c, ok := s.clients[uuid]; ok {
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
			s.responses <- m.Client
		case MESSAGE_TUNNEL:
			// handle a tunnel message
		case MESSAGE_COMMAND:
			// route a command to one or all clients
			s.route(m)
		default:
			log.Error("unknown message type: %v", m.Type)
			return
		}
	}
}

func (s *Server) route(m *Message) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if m.UUID == "" {
		// all clients
		for _, c := range s.clients
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

			path := filepath.Join(r.path, RESPONSE_PATH, strconv.Itoa(v.ID), cin.UUID)
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

	r.commandLock.Lock()
	defer r.commandLock.Unlock()

	if c, ok := r.commands[id]; ok {
		c.CheckedIn = append(c.CheckedIn, uuid)
	} else {
		log.Error("ron command checkin: command %v does not exist", id)
	}
}

func (s *Server) DeleteCommand(id int) error {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()
	if _, ok := s.commands[id]; ok {
		delete(r.commands, id)
		return nil
	} else {
		return fmt.Errorf("command %v not found", id)
	}
}

func (s *Server) NewCommand(c *Command) int {
	c.ID = <-s.commandID
	r.commandLock.Lock()
	r.commands[c.ID] = c
	r.commandLock.Unlock()
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
		for k, v := range r.clients {
			if t.Sub(v.Checkin) > time.Duration(CLIENT_EXPIRED) * time.Second) {
				log.Debug("client %v expired", k)
				go s.removeClient(k) // hack: put this in a goroutine to simplify locking
			}
		}
		r.clientLock.Unlock()
	}
}

func (s *Server) GetActiveSerialPorts() []string {
	r.serialLock.Lock()
	defer r.serialLock.Unlock()

	var ret []string
	for k, _ := range r.serialConns {
		ret = append(ret, k)
	}

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
