// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const PART_SIZE = 1024 * 100

type Server struct {
	// UseVMs controls whether ron uses VM callbacks or not (see ron.VM)
	UseVMs bool

	// conns stores connected but not necessarily active connections. Includes
	// serial connections.
	conns map[string]net.Conn
	// connsLock synchronizes access to conns
	connsLock sync.Mutex

	// listeners stores listening sockets. Includes unix domain sockets and TCP
	listeners map[string]net.Listener
	// listenersLock synchronizes access to listeners
	listenersLock sync.Mutex

	// commands stores commands that are still active and will be pushed to
	// matching clients.
	commands map[int]*Command // map of active commands
	// commandCounter is the next available command ID
	commandCounter int
	// commandLock synchronizes access to commands and commandCounter
	commandLock sync.Mutex

	clients    map[string]*client // map of active clients, each of which have a running handler
	vms        map[string]VM      // map of uuid -> VM
	clientLock sync.Mutex         // lock for clients and vms

	path string // path for serving files

	// subpath is an optional path parameter that will always be used when
	// writing responses and receiving files. When reading files, we check for
	// the file with and without the subpath (with first).
	subpath string

	lastBroadcast time.Time // watchdog time of last command list broadcast

	responses chan *Client // queue of incoming responses, consumed by the response processor

	plumber *miniplumber.Plumber

	// set to non-zero value by Server.Destroy
	isdestroyed uint64
}

// NewServer creates a ron server. Must call Listen* to actually allow ron to
// start accepting connections from clients.
func NewServer(path, subpath string, plumber *miniplumber.Plumber) (*Server, error) {
	s := &Server{
		UseVMs:        true,
		conns:         make(map[string]net.Conn),
		listeners:     make(map[string]net.Listener),
		commands:      make(map[int]*Command),
		clients:       make(map[string]*client),
		vms:           make(map[string]VM),
		path:          path,
		subpath:       subpath,
		lastBroadcast: time.Now(),
		responses:     make(chan *Client, 1024),
		plumber:       plumber,
	}

	if err := os.MkdirAll(s.responsePath(nil), 0775); err != nil {
		return nil, err
	}

	go s.responseHandler()
	go s.clientReaper()

	log.Info("registered new ron server: %v", filepath.Join(path, subpath))

	return s, nil
}

func (s *Server) Destroy() {
	if s.destroyed() {
		// already been destroyed once before
		return
	}

	s.setDestroyed()

	// close all the serial connections
	s.connsLock.Lock()
	for _, c := range s.conns {
		c.Close()
	}
	s.connsLock.Unlock()

	// stop all listeners
	s.listenersLock.Lock()
	listeners := len(s.listeners)
	for _, ln := range s.listeners {
		ln.Close()
	}
	s.listenersLock.Unlock()

	// wait for all the listeners to shutdown. We do this to prevent a race
	// where a new client has been accepted but for which the handler has not
	// started. By waiting until all the listeners have called their defer func to
	// delete the listener, we can guarantee that there will be
	for listeners > 0 {
		log.Info("waiting on %v listeners to shutdown", listeners)
		time.Sleep(100 * time.Millisecond)

		s.listenersLock.Lock()
		listeners = len(s.listeners)
		s.listenersLock.Unlock()
	}

	// close channel for responses, killing responseHandler. Have to wait until
	// all the clients disconnect, otherwise we could try to send on a closed
	// channel.
	for v := s.Clients(); v > 0; v = s.Clients() {
		log.Info("waiting on %v clients to disconnect", v)
		time.Sleep(5 * time.Second)
	}
	close(s.responses)
}

// Listen starts accepting TCP connections on the specified port, accepting
// connections in a goroutine. Returns an error if the server is already
// listening on that port or if there was another error.
func (s *Server) Listen(port int) error {
	log.Info("listening on :%v", port)

	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	addr := ":" + strconv.Itoa(port)

	if _, ok := s.listeners[addr]; ok {
		return fmt.Errorf("already listening on %v", addr)
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	s.listeners[addr] = ln
	go s.serve(addr, ln)

	return nil
}

// ListenUnix creates a unix domain socket at the given path and listens for
// incoming connections. ListenUnix returns on the successful creation of the
// socket, and accepts connections in a goroutine. Returns an error if the
// server is already listening on that path or if there was another error.
func (s *Server) ListenUnix(path string) error {
	log.Info("listening on `%v`", path)

	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	if _, ok := s.listeners[path]; ok {
		return fmt.Errorf("already listening on %v", path)
	}

	u, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return err
	}

	ln, err := net.ListenUnix("unix", u)
	if err != nil {
		return err
	}

	s.listeners[path] = ln
	go s.serve(path, ln)

	return nil
}

// Dial a client serial port. The server will maintain this connection until a
// client connects and then disconnects.
func (s *Server) DialSerial(path, uuid string) error {
	dial := func(path string) (net.Conn, error) {
		log.Info("dial serial: %v", path)

		s.connsLock.Lock()
		defer s.connsLock.Unlock()

		// close any lingering connections to this client
		if conn, ok := s.conns[path]; ok {
			conn.Close()
		}

		// connect!
		conn, err := net.Dial("unix", path)
		if err != nil {
			return nil, err
		}

		s.conns[path] = conn

		return conn, nil
	}

	// Monitor this client connection, reconnecting when the client handler
	// completes if the VM still exists.
	go func() {
		for {
			s.clientLock.Lock()
			_, exists := s.vms[uuid]
			s.clientLock.Unlock()

			if !exists {
				log.Debug("vm %s (serial://%s) no longer exists -- closing serial connection", uuid, path)

				s.connsLock.Lock()
				if conn := s.conns[path]; conn != nil {
					conn.Close()
					delete(s.conns, path)
				}
				s.connsLock.Unlock()

				return
			}

			conn, err := dial(path)
			if err != nil {
				log.Error("dialing serial port %v failed: %v", path, err)
				return
			}

			cli, err := s.handshake(conn)
			if err != nil {
				log.Debug("handshake failed (due to %v) - retrying", err)

				time.Sleep(CLIENT_RECONNECT_RATE * time.Second)

				continue
			}

			// This blocks, but will return on a loss of connection to the client.
			s.clientHandler(cli)

			log.Info("client handler for %s completed", path)
		}
	}()

	return nil
}

// CloseUnix closes a unix domain socket created via ListenUnix.
func (s *Server) CloseUnix(path string) error {
	log.Info("close UNIX: %v", path)

	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	l, ok := s.listeners[path]
	if !ok {
		log.Info("tried to close unknown path: %v", path)
		return nil
	}

	if err := l.Close(); err != nil {
		return err
	}

	delete(s.listeners, path)
	return nil
}

// NewCommand posts a new command to the active command list. The command ID is
// returned.
func (s *Server) NewCommand(c *Command) int {
	log.Debug("NewCommand: %v", c)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	s.commandCounter++
	c.ID = s.commandCounter

	s.commands[c.ID] = c

	go s.sendCommands("")
	return c.ID
}

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

func (s *Server) GetExitCode(id int, client string) (int, error) {
	var cid string

	if _, ok := s.clients[client]; ok {
		cid = client
	} else {
		for _, c := range s.clients {
			if c.Hostname == client {
				cid = c.UUID
				break
			}
		}
	}

	if cid == "" {
		return 0, fmt.Errorf("no client %s", client)
	}

	path := filepath.Join(s.responsePath(&id), cid, "exitcode")

	if _, err := os.Stat(path); err != nil {
		return 0, err
	}

	body, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}

	code, err := strconv.Atoi(strings.TrimSpace(string(body)))
	if err != nil {
		return 0, err
	}

	return code, nil
}

func (s *Server) GetResponse(id int, raw bool) (string, error) {
	base := filepath.Join(s.responsePath(&id))
	res, err := s.getResponses(base, raw)

	if os.IsNotExist(err) {
		return res, fmt.Errorf("no responses for %v", id)
	}

	return res, err
}

func (s *Server) GetResponses(raw bool) (string, error) {
	res, err := s.getResponses(s.responsePath(nil), raw)

	if os.IsNotExist(err) {
		// if the responses directory doesn't exist, don't report an error,
		// just return an empty result
		return res, nil
	}

	return res, err
}

// GetClients returns a list of every active client
func (s *Server) GetClients() map[string]*Client {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	res := make(map[string]*Client)

	// deep copy
	for u, c := range s.clients {
		res[u] = &Client{
			UUID:          c.UUID,
			Arch:          c.Arch,
			OS:            c.OS,
			Version:       c.Version,
			Hostname:      c.Hostname,
			LastCommandID: c.LastCommandID,
			Processes:     make(map[int]*Process),
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

// Clients returns the number of clients connected to the server.
func (s *Server) Clients() int {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	return len(s.clients)
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

// DeleteCommands removes all commands with the specified prefix.
func (s *Server) DeleteCommands(prefix string) error {
	log.Debug("delete commands: `%v`", prefix)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	var matched bool

	for id, c := range s.commands {
		if c.Prefix == prefix {
			matched = true
			delete(s.commands, id)
		}
	}

	if !matched {
		return fmt.Errorf("no such prefix: `%v`", s)
	}

	return nil
}

// ClearCommands deletes all commands and sets the command ID counter back to
// zero. As with DeleteCommand, any in-flight responses may still be returned.
func (s *Server) ClearCommands() {
	log.Debug("clearing commands")

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	s.commandCounter = 0
	s.commands = make(map[int]*Command)

	for _, c := range s.clients {
		c.maxCommandID = 0
	}
}

// DeleteResponse removes all responses for the given ID. Any in-flight
// responses will not be deleted.
func (s *Server) DeleteResponse(id int) error {
	log.Debug("delete response: %v", id)

	// grab the client lock so that no more responses can be processed until
	// we're finished.
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	path := filepath.Join(s.responsePath(&id))

	return os.RemoveAll(path)
}

// DeleteResponses removes all commands with the specified prefix.
func (s *Server) DeleteResponses(prefix string) error {
	log.Debug("delete responses: `%v`", prefix)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	// grab the client lock so that no more responses can be processed until
	// we're finished.
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	var matched bool

	for id, c := range s.commands {
		if c.Prefix == prefix {
			if err := os.RemoveAll(s.responsePath(&id)); err != nil {
				return err
			}

			matched = true
		}
	}

	if !matched {
		return fmt.Errorf("no such prefix: `%v`", s)
	}

	return nil
}

// ClearResponses deletes all responses received so far. It may not affect
// responses that are still in-flight.
func (s *Server) ClearResponses() error {
	log.Info("clearing responses")

	// grab the client lock so that no more responses can be processed until
	// we're finished.
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	log.Info("cleared responses")

	return os.RemoveAll(s.responsePath(nil))
}

func (s *Server) RegisterVM(vm VM) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	s.vms[vm.GetUUID()] = vm
}

func (s *Server) UnregisterVM(vm VM) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	delete(s.vms, vm.GetUUID())
}

func (s *Server) setDestroyed() {
	atomic.StoreUint64(&s.isdestroyed, 1)
}

func (s *Server) destroyed() bool {
	return atomic.LoadUint64(&s.isdestroyed) > 0
}

// responsePath returns the directory for responses. If an ID is specified, it
// is added to the path.
func (s *Server) responsePath(id *int) string {
	if id == nil {
		return filepath.Join(s.path, s.subpath, RESPONSE_PATH)
	}

	return filepath.Join(s.path, s.subpath, RESPONSE_PATH, strconv.Itoa(*id))
}

// serve
func (s *Server) serve(addr string, ln net.Listener) {
	defer func() {
		s.listenersLock.Lock()
		defer s.listenersLock.Unlock()

		delete(s.listeners, addr)
		log.Info("closed listener: %v", addr)
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			// filter out errors caused by closed network connection --
			// probably means the server is being destroyed or CloseUnix
			// was called.
			if !strings.Contains(err.Error(), "use of closed network connection") {
				log.Error("serving %v: %v", addr, err)
			}

			return
		}

		remote := conn.RemoteAddr()

		log.Info("client connected: %v -> %v", remote, addr)
		c, err := s.handshake(conn)
		if err != nil {
			if err != io.EOF {
				// supress these, VM was probably never started
				log.Error("handshake failed: %v", err)
			}
			conn.Close()
			continue
		}

		go func() {
			s.clientHandler(c)
			log.Debug("client disconnected: %v -> %v", remote, addr)
		}()
	}
}

// handshake performs a handshake with the client, returning the new client if
// there were no errors.
func (s *Server) handshake(conn net.Conn) (*client, error) {
	// read until we see the magic bytes
	var buf [3]byte
	for string(buf[:]) != "RON" {
		// shift the buffer
		buf[0] = buf[1]
		buf[1] = buf[2]
		// read the next byte
		_, err := conn.Read(buf[2:])
		if err != nil {
			return nil, err
		}
	}

	// write magic bytes back
	if _, err := io.WriteString(conn, "RON"); err != nil {
		return nil, err
	}

	c := &client{
		conn:            conn,
		enc:             gob.NewEncoder(conn),
		dec:             gob.NewDecoder(conn),
		pipeReaders:     make(map[string]*miniplumber.Reader),
		pipeWriters:     make(map[string]chan<- string),
		cancelHeartbeat: make(chan struct{}),
	}

	// get the first client struct as a handshake
	var m Message
	if err := c.dec.Decode(&m); err != nil {
		return nil, err
	}

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if _, ok := s.clients[m.Client.UUID]; ok {
		return nil, fmt.Errorf("client %v already exists!", m.Client.UUID)
	}

	var namespace string

	if s.UseVMs {
		vm, ok := s.vms[m.Client.UUID]
		if !ok {
			// try again after unmangling the uuid, which qemu does in certain
			// versions
			vm, ok = s.vms[unmangle(m.Client.UUID)]
			c.mangled = true
		}
		if !ok {
			return nil, fmt.Errorf("unregistered client %v", m.Client.UUID)
		}

		namespace = vm.GetNamespace()
	}

	c.Namespace = namespace

	if m.Client.Version != version.Revision {
		log.Warn("mismatched miniccc version: %v", m.Client.Version)
	}

	if majorVersion(m.Version) > 0 {
		log.Info("starting heartbeat to client %s", m.Client.UUID)

		// Only send heartbeats to client if message version is present to prevent
		// older clients from failing against this version of the server (sending a
		// message to a client that doesn't recognize the message type will cause
		// the client to fail).
		go func() {
			t := time.NewTicker(HEARTBEAT_RATE * time.Second)

			for {
				select {
				case <-c.cancelHeartbeat:
					log.Debug("stopping heartbeats to client %s", m.Client.UUID)
					t.Stop()
					return
				case <-t.C:
					log.Debug("sending HEARTBEAT to client %s", m.Client.UUID)
					m := Message{Type: MESSAGE_HEARTBEAT, Version: "v1"}
					c.enc.Encode(&m) // no need to worry about errors here
				}
			}
		}()
	} else {
		log.Warn("client %s is missing message version -- not starting heartbeat", m.Client.UUID)
	}

	// TODO: if the client blocks, ron will hang... probably not good
	if err := c.enc.Encode(&m); err != nil {
		// client disconnected before it read the full handshake
		if err != io.EOF {
			log.Errorln(err)
		}
		return nil, err
	}

	c.Client = m.Client
	if c.mangled {
		c.UUID = unmangle(m.Client.UUID)
	}
	log.Debug("new client: %v", m.Client)

	c.checkin = time.Now()

	s.clients[c.UUID] = c

	return c, nil
}

// client and transport handler for connections.
func (s *Server) clientHandler(c *client) {
	defer c.conn.Close()
	defer s.removeClient(c.UUID)

	// Set up minitunnel, dialing the server that should be running on the
	// client's side. Data is Trunk'd via Messages.
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	go Trunk(remote, c.UUID, c.sendMessage)

	go func() {
		tunnel, err := minitunnel.Dial(local)
		if err != nil {
			log.Error("dial: %v", err)
			return
		}

		s.clientLock.Lock()
		defer s.clientLock.Unlock()

		log.Debug("minitunnel created for %v", c.UUID)
		c.tunnel = tunnel
	}()

	// send the commands to our new client
	go s.sendCommands(c.UUID)

	var err error

	for err == nil {
		if s.destroyed() {
			return
		}

		var m Message
		if err = c.dec.Decode(&m); err == nil {
			log.Debug("new message: %v", m.Type)

			// unmangle the client uuid if necessary
			if c.mangled {
				m.UUID = unmangle(m.UUID)
			}

			switch m.Type {
			case MESSAGE_TUNNEL:
				_, err = remote.Write(m.Tunnel)
			case MESSAGE_FILE:
				if m.Error != "" {
					log.Error("file error from %v: %v", c.UUID, m.Error)
					continue
				}
				if m.File.Data == nil && m.File.Offset == 0 {
					// client requested file
					err = s.sendFile(c, m.File.Name)
				} else {
					// client sent file
					fpath := filepath.Join(s.responsePath(&m.File.ID), c.UUID, m.File.Name)
					err = m.File.Recv(fpath)
				}
			case MESSAGE_CLIENT:
				if c.mangled {
					m.Client.UUID = unmangle(m.Client.UUID)
				}
				s.responses <- m.Client
			case MESSAGE_COMMAND:
				// this shouldn't be sent via the client...
			case MESSAGE_PIPE:
				c.pipeHandler(s.plumber, &m)
			case MESSAGE_UFS:
				c.ufsMessage(&m)
			default:
				err = fmt.Errorf("unknown message type: %v", m.Type)
			}
		}
	}

	// This is an OK error - likely just means the TCP connection was closed.
	if err == io.EOF {
		return
	}

	// This is an OK error - likely just means the client closed the connection.
	if strings.Contains(err.Error(), "connection reset by peer") {
		log.Debugln(err)
		return
	}

	// This is an OK error - likely just means the unix socket was closed.
	if strings.Contains(err.Error(), "use of closed network connection") {
		log.Debugln(err)
		return
	}

	log.Errorln(err)
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

		// with the client conn closed, close any lingering plumbing
		c.pipeLock.Lock()
		defer c.pipeLock.Unlock()
		for _, p := range c.pipeReaders {
			p.Close()
		}
		for _, p := range c.pipeWriters {
			close(p)
		}

		// stop Goroutine sending heartbeats to this client
		close(c.cancelHeartbeat)

		delete(s.clients, uuid)
	}
}

// sendCommands send a commands message to the specified UUID. If UUID is not
// specified, the message is sent to all active clients.
func (s *Server) sendCommands(uuid string) {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	m := &Message{
		Type:     MESSAGE_COMMAND,
		Commands: make(map[int]*Command),
		UUID:     uuid,
	}
	for k, v := range s.commands {
		m.Commands[k] = v.Copy()
	}

	s.route(m)
}

// NewFilesSendCommand creates a command to send to clients to read the listed
// files, expanding globs.
func (s *Server) NewFilesSendCommand(files []string) (*Command, error) {
	cmd := &Command{}

	for _, f := range files {
		f = filepath.Clean(f)

		if filepath.IsAbs(f) && !strings.HasPrefix(f, s.path) {
			return nil, fmt.Errorf("can only send files from %v", s.path)
		}

		var send []string
		var err error

		if filepath.IsAbs(f) {
			// if the file is absolute, glob it
			send, err = filepath.Glob(f)
		} else {
			// if the file is relative, look in the subpath first and then in
			// the global directory
			dir := filepath.Join(s.path, s.subpath)

			send, err = filepath.Glob(filepath.Join(dir, f))
			if err != nil || len(send) == 0 {
				dir = s.path
				send, err = filepath.Glob(filepath.Join(dir, f))
			}

			// make all files relative, won't do anything if there was an error
			for i, v := range send {
				v2, err := filepath.Rel(dir, v)
				if err != nil {
					return nil, err
				}

				log.Info("send %v, rel: %v", v, v2)

				send[i] = v2
			}
		}

		if err != nil || len(send) == 0 {
			return nil, fmt.Errorf("no such file: %v", f)
		}

		cmd.FilesSend = send
	}

	return cmd, nil
}

// sendFile reads the file and sends it in multiple chunks to the client.
func (s *Server) sendFile(c *client, filename string) error {
	log.Debug("sendFile: %v to %v", filename, c.UUID)

	// try to send version from subpath first
	dir := filepath.Join(s.path, s.subpath)
	fpath := filepath.Join(dir, filename)
	if _, err := os.Stat(fpath); err == nil {
		// found file in subpath
		return SendFile(dir, fpath, 0, PART_SIZE, c.sendMessage)
	}

	dir = s.path
	fpath = filepath.Join(dir, filename)
	return SendFile(dir, fpath, 0, PART_SIZE, c.sendMessage)
}

// route an outgoing message to one or all clients, according to UUID
func (s *Server) route(m *Message) {
	var maxCommandID int
	for i := range m.Commands {
		if i > maxCommandID {
			maxCommandID = i
		}
	}

	handleUUID := func(uuid string) {
		// create locally scoped pointer to message
		m := m

		c, ok := s.clients[uuid]
		if !ok {
			log.Error("no such client %v", uuid)
			return
		}

		if c.maxCommandID == maxCommandID {
			log.Info("no commands for %v", uuid)
			return
		}

		if m.Type == MESSAGE_COMMAND {
			if s.UseVMs {
				vm, ok := s.vms[uuid]
				if !ok {
					// odd, someone must have unregistered the client...
					log.Error("unregistered client %v", uuid)
					return
				}
				// update client's tags in case we're matching based on them
				c.Tags = vm.GetTags()

				// load the relevant info fields, overriding any tag values
				for _, cmd := range m.Commands {
					if cmd.Filter != nil && cmd.Filter.Tags != nil {
						for k := range cmd.Filter.Tags {
							// only replace non-zero fields
							v, _ := vm.Info(k)
							if v != "" {
								c.Tags[k] = v
							}
						}
					}
				}
			}

			// create a copy of the Message
			m2 := *m
			m2.Commands = map[int]*Command{}

			// filter the commands to relevant ones
			for i, cmd := range m.Commands {
				if c.Matches(cmd.Filter) && i > c.maxCommandID {
					m2.Commands[i] = cmd
				}
			}

			c.maxCommandID = maxCommandID

			if len(m2.Commands) == 0 {
				log.Info("no commands for %v", uuid)
				return
			}

			m = &m2
		}

		if err := c.sendMessage(m); err != nil {
			if strings.Contains(err.Error(), "broken pipe") {
				log.Debug("client disconnected: %v", uuid)
			} else {
				log.Info("unable to send message to %v: %v", uuid, err)
			}
		}
	}

	// handleUUID doesn't modify the clients map so we can allow parallel reads
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
	for cin := range s.responses {
		log.Debug("responseHandler: %v", cin.UUID)

		// update all the client fields
		s.updateClient(cin)

		// ingest responses from this client
		for _, v := range cin.Responses {
			log.Debug("got response %v : %v", cin.UUID, v.ID)
			s.commandCheckIn(v.ID, cin.UUID)

			path := filepath.Join(s.responsePath(&v.ID), cin.UUID)

			if err := os.MkdirAll(path, os.FileMode(0770)); err != nil {
				log.Error("could not record response %v for %v: %v", v.ID, cin.UUID, err)
				continue
			}

			// generate exitcode file
			if v.RecordExitCode {
				err := ioutil.WriteFile(filepath.Join(path, "exitcode"), []byte(strconv.Itoa(v.ExitCode)), os.FileMode(0660))
				if err != nil {
					log.Error("could not record exit code %v for %v: %v", v.ID, cin.UUID, err)
				}
			}

			// generate stdout and stderr if they exist
			if v.Stdout != "" {
				err := ioutil.WriteFile(filepath.Join(path, "stdout"), []byte(v.Stdout), os.FileMode(0660))
				if err != nil {
					log.Error("could not record stdout %v for %v: %v", v.ID, cin.UUID, err)
				}
			}

			if v.Stderr != "" {
				err := ioutil.WriteFile(filepath.Join(path, "stderr"), []byte(v.Stderr), os.FileMode(0660))
				if err != nil {
					log.Error("could not record stderr %v for %v: %v", v.ID, cin.UUID, err)
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
		// the client probably disconnected between sending the heartbeat and
		// us processing it. We'll still process any command responses but
		// shouldn't try to update the client itself.
		log.Info("unknown client %v", cin.UUID)
		return
	}

	c.Client = cin
	c.checkin = time.Now()

	if !s.UseVMs {
		return
	}

	vm, ok := s.vms[cin.UUID]
	if !ok {
		// see above but for the VM.
		log.Info("unregistered client %v", cin.UUID)
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

// clientReaper periodically flushes old entries from the client list
func (s *Server) clientReaper() {
	for {
		if s.destroyed() {
			log.Debug("reaping client reaper")
			return
		}

		time.Sleep(time.Duration(REAPER_RATE) * time.Second)

		s.clientLock.Lock()
		for k, v := range s.clients {
			// checked in more recently than expiration time
			active := time.Now().Sub(v.checkin) < CLIENT_EXPIRED*time.Second

			if !active {
				log.Debug("client %v expired", k)
				// the same as removeClient except we already hold clientLock
				v.conn.Close()
				close(v.cancelHeartbeat)
				delete(s.clients, k)
			}

			if vm, ok := s.vms[k]; ok {
				vm.SetCCActive(active)
			}
		}
		s.clientLock.Unlock()
	}
}

func (s *Server) getResponses(base string, raw bool) (string, error) {
	var res string
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			if strings.HasSuffix(path, "exitcode") {
				return nil
			}

			log.Debug("add to response files: %v", path)

			data, err := ioutil.ReadFile(path)
			if err != nil {
				return err
			}

			if !raw {
				relPath, err := filepath.Rel(s.responsePath(nil), path)
				if err != nil {
					return err
				}
				res += fmt.Sprintf("%v:\n", relPath)
			}

			res += fmt.Sprintf("%v\n", string(data))
		}

		return nil
	}

	err := filepath.Walk(base, walker)
	return res, err
}
