// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package ron

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
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

var errClientTooOld = fmt.Errorf("miniccc client too old -- please update")

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

	// accepted stores the open connections accepted on each listener
	accepted   map[string]map[net.Conn]struct{}
	acceptLock sync.Mutex
	// acceptWG counts the goroutines that serve accepted connections
	acceptWG sync.WaitGroup

	// uploads counts the bytes that each client wrote in the current hour
	uploads    map[string]*uploadWindow
	uploadLock sync.Mutex

	// onMessage, when set, is called with each message from a client before
	// it is handled. Used by tests.
	onMessage func(*Message)

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
	autoUpdate bool

	// updateTimeout limits how long the server holds commands for a client
	// after it sends an update. Zero means UPDATE_TIMEOUT.
	updateTimeout time.Duration

	// hashes caches the SHA-256 hashes of staged update binaries
	hashes   map[string]stagedHash
	hashLock sync.Mutex

	path string // path for serving files

	// subpath is an optional path parameter that will always be used when
	// writing responses and receiving files. When reading files, we check for
	// the file with and without the subpath (with first).
	subpath string

	lastBroadcast time.Time // watchdog time of last command list broadcast

	responses chan queuedClient // queue of incoming responses, consumed by the response processor

	// queued is the encoded size of the client updates in responses, at most
	// MaxQueuedResponses
	queued    int64
	queueLock sync.Mutex
	queueCond *sync.Cond

	handlerDone chan struct{} // closed when responseHandler returns

	stopReaper chan struct{} // closed by Destroy to stop clientReaper

	reaperRate   time.Duration // how often clientReaper checks for expired clients
	clientExpiry time.Duration // how long a client can go without a check-in

	reconnectRate time.Duration // how long DialSerial waits before it dials again

	plumber *miniplumber.Plumber

	// set to non-zero value by Server.Destroy
	isdestroyed uint64
}

// NewServer creates a ron server. Must call Listen* to actually allow ron to
// start accepting connections from clients.
func NewServer(path, subpath string, plumber *miniplumber.Plumber) (*Server, error) {
	return newServer(path, subpath, plumber, REAPER_RATE*time.Second, CLIENT_EXPIRED*time.Second)
}

// newServer creates a ron server that checks for expired clients every
// reaperRate and expires clients that do not check in within clientExpiry.
func newServer(path, subpath string, plumber *miniplumber.Plumber, reaperRate, clientExpiry time.Duration) (*Server, error) {
	s := &Server{
		UseVMs:        true,
		conns:         make(map[string]net.Conn),
		listeners:     make(map[string]net.Listener),
		accepted:      make(map[string]map[net.Conn]struct{}),
		uploads:       make(map[string]*uploadWindow),
		commands:      make(map[int]*Command),
		clients:       make(map[string]*client),
		vms:           make(map[string]VM),
		path:          path,
		subpath:       subpath,
		lastBroadcast: time.Now(),
		responses:     make(chan queuedClient, 1024),
		handlerDone:   make(chan struct{}),
		stopReaper:    make(chan struct{}),
		reaperRate:    reaperRate,
		clientExpiry:  clientExpiry,
		reconnectRate: CLIENT_RECONNECT_RATE * time.Second,
		plumber:       plumber,
	}
	s.queueCond = sync.NewCond(&s.queueLock)

	// the response folder is created when the first response or file arrives
	if err := os.MkdirAll(s.path, 0775); err != nil {
		return nil, err
	}

	go s.responseHandler()
	go s.clientReaper()

	log.Info("registered new ron server: %v", filepath.Join(path, subpath))

	return s, nil
}

// SetAutoUpdate enables or disables miniccc binary updates on client connect.
func (s *Server) SetAutoUpdate(enabled bool) {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	s.autoUpdate = enabled
}

// AutoUpdate reports whether miniccc binary updates on client connect are
// enabled.
func (s *Server) AutoUpdate() bool {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()
	return s.autoUpdate
}

func (s *Server) Destroy() {
	if !atomic.CompareAndSwapUint64(&s.isdestroyed, 0, 1) {
		// already been destroyed once before
		return
	}

	// keep reaping while Destroy waits, so that silent clients expire
	if s.stopReaper != nil {
		defer close(s.stopReaper)
	}

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

	// close the accepted connections and wait for their handlers, so that no
	// handshake can add a client after the wait below
	s.closeAccepted("")
	s.acceptWG.Wait()

	// close channel for responses, killing responseHandler. Have to wait until
	// all the clients disconnect, otherwise we could try to send on a closed
	// channel.
	for v := s.Clients(); v > 0; v = s.Clients() {
		log.Info("waiting on %v clients to disconnect", v)
		time.Sleep(5 * time.Second)
	}
	close(s.responses)

	// wait for queued responses to be written before removing empty folders
	if s.handlerDone != nil {
		<-s.handlerDone
	}

	s.removeEmptyDirs()
}

// removeEmptyDirs removes the response folder and then the subpath folder if
// they are empty. It removes only folders below s.path, never s.path itself
// or a folder that is not empty.
func (s *Server) removeEmptyDirs() {
	for _, dir := range []string{s.responsePath(nil), filepath.Join(s.path, s.subpath)} {
		if !isSubdir(s.path, dir) {
			continue
		}

		if err := removeDirIfEmpty(dir); err != nil {
			log.Warn("unable to remove empty folder %v: %v", dir, err)
		}
	}
}

// isSubdir returns true if dir is below base and is not base itself.
func isSubdir(base, dir string) bool {
	rel, err := filepath.Rel(base, dir)
	if err != nil {
		return false
	}

	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// removeDirIfEmpty removes dir if it exists and is empty.
func removeDirIfEmpty(dir string) error {
	f, err := os.Open(dir)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}

	_, err = f.Readdirnames(1)
	f.Close()

	if err == nil {
		// dir is not empty
		return nil
	} else if err != io.EOF {
		return err
	}

	return os.Remove(dir)
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
	go s.serve(addr, ln, "")

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
	go s.serve(path, ln, "")

	return nil
}

// ListenUnixBound creates a unix domain socket at path for the VM with the
// given UUID, and accepts connections on it in a goroutine. Every connection
// is bound to uuid: a client that sends an empty UUID gets uuid, and a client
// that claims another UUID is refused. The socket is owned by uid and the
// group uid, with mode 0600, so only uid and root can connect to it. A
// negative uid keeps the owner and the group. Close the socket with
// CloseUnix. The VM must be registered with RegisterVM before its client can
// complete the handshake.
func (s *Server) ListenUnixBound(path, uuid string, uid int) error {
	if uuid == "" {
		return errors.New("bound listener needs a VM UUID")
	}

	log.Info("listening on `%v` for VM %v", path, uuid)

	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	if _, ok := s.listeners[path]; ok {
		return fmt.Errorf("already listening on %v", path)
	}

	ln, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return err
	}

	if uid >= 0 {
		err = os.Chown(path, uid, uid)
	}
	if err == nil {
		err = os.Chmod(path, 0600)
	}
	if err != nil {
		ln.Close()
		return err
	}

	s.listeners[path] = ln
	go s.serve(path, ln, uuid)

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

			cli, err := s.handshake(conn, uuid)
			if err != nil {
				if errors.Is(err, errClientTooOld) {
					log.Error("%v", err.Error())
					return
				}

				log.Debug("handshake failed (due to %v) - retrying", err)
				conn.Close()
				time.Sleep(s.reconnectRate)

				continue
			}

			// This blocks, but will return on a loss of connection to the client.
			s.clientHandler(cli)

			log.Debug("client handler for %s completed", path)
		}
	}()

	return nil
}

// CloseUnix closes a unix domain socket created via ListenUnix or
// ListenUnixBound, and closes the client connections accepted on it.
func (s *Server) CloseUnix(path string) error {
	log.Info("close UNIX: %v", path)

	s.listenersLock.Lock()
	l, ok := s.listeners[path]
	if ok {
		delete(s.listeners, path)
	}
	s.listenersLock.Unlock()

	var err error
	if ok {
		err = l.Close()
	}

	// serve closes a connection that it accepted before the close above, but
	// tracks after this call
	s.closeAccepted(path)

	if !ok {
		log.Info("tried to close unknown path: %v", path)
	}

	return err
}

// listening reports whether ln is still the listener on addr.
func (s *Server) listening(addr string, ln net.Listener) bool {
	s.listenersLock.Lock()
	defer s.listenersLock.Unlock()

	return s.listeners[addr] == ln
}

// track records conn as accepted on addr. It returns false, and does not
// record conn, when max is positive and addr already has max connections.
func (s *Server) track(addr string, conn net.Conn, max int) bool {
	s.acceptLock.Lock()
	defer s.acceptLock.Unlock()

	if s.accepted == nil {
		s.accepted = make(map[string]map[net.Conn]struct{})
	}

	conns := s.accepted[addr]
	if conns == nil {
		conns = make(map[net.Conn]struct{})
		s.accepted[addr] = conns
	}

	if max > 0 && len(conns) >= max {
		return false
	}

	conns[conn] = struct{}{}
	return true
}

func (s *Server) untrack(addr string, conn net.Conn) {
	s.acceptLock.Lock()
	defer s.acceptLock.Unlock()

	if conns := s.accepted[addr]; conns != nil {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(s.accepted, addr)
		}
	}
}

// closeAccepted closes the connections accepted on addr, or on every
// listener when addr is empty.
func (s *Server) closeAccepted(addr string) {
	s.acceptLock.Lock()
	defer s.acceptLock.Unlock()

	for a, conns := range s.accepted {
		if addr != "" && a != addr {
			continue
		}
		for conn := range conns {
			conn.Close()
		}
	}
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

	s.clientLock.Lock()
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
	s.clientLock.Unlock()

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

// GetClientResponses returns the stdout and stderr recorded for each client
// that responded to command id, or to every command if id is nil. Results are
// sorted by command ID then client UUID.
func (s *Server) GetClientResponses(id *int) ([]ClientResponse, error) {
	res := []ClientResponse{}

	var ids []int
	if id != nil {
		ids = append(ids, *id)
	} else {
		entries, err := os.ReadDir(s.responsePath(nil))
		if os.IsNotExist(err) {
			return res, nil
		} else if err != nil {
			return nil, err
		}

		for _, e := range entries {
			if v, err := strconv.Atoi(e.Name()); err == nil && e.IsDir() {
				ids = append(ids, v)
			}
		}
	}

	for _, v := range ids {
		base := s.responsePath(&v)

		entries, err := os.ReadDir(base)
		if os.IsNotExist(err) && id != nil {
			return nil, fmt.Errorf("no responses for %v", v)
		} else if err != nil {
			return nil, err
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}

			r := ClientResponse{ID: v, UUID: e.Name()}

			for name, dst := range map[string]*string{"stdout": &r.Stdout, "stderr": &r.Stderr} {
				path := filepath.Join(base, e.Name(), name)

				// a received file may have created a directory with this name
				if fi, err := os.Stat(path); os.IsNotExist(err) || (err == nil && !fi.Mode().IsRegular()) {
					continue
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				*dst = string(data)
			}

			res = append(res, r)
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].ID != res[j].ID {
			return res[i].ID < res[j].ID
		}
		return res[i].UUID < res[j].UUID
	})

	return res, nil
}

// GetClientInfo returns a serializable summary of every active client, sorted
// by UUID.
func (s *Server) GetClientInfo() []ClientInfo {
	res := []ClientInfo{}

	for _, c := range s.GetClients() {
		res = append(res, ClientInfo{
			UUID:     c.UUID,
			Hostname: c.Hostname,
			Arch:     c.Arch,
			OS:       c.OS,
			Version:  c.Version,
			IPs:      c.IPs,
			MACs:     c.MACs,
		})
	}

	sort.Slice(res, func(i, j int) bool { return res[i].UUID < res[j].UUID })

	return res
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
			SHA256:        c.SHA256,
			LastCommandID: c.LastCommandID,
			Processes:     make(map[int]*Process),
		}
		res[u].Capabilities = append(res[u].Capabilities, c.Capabilities...)
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

// DeleteCommand removes a command from the active command list. The server
// drops the responses and the files that clients send later for the deleted
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
// zero. As with DeleteCommand, the server drops later responses to the
// deleted commands, except for an ID that a new command uses again.
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
	log.Debug("clearing responses")

	// grab the client lock so that no more responses can be processed until
	// we're finished.
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	log.Debug("cleared responses")

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

	s.uploadLock.Lock()
	delete(s.uploads, vm.GetUUID())
	s.uploadLock.Unlock()
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

// makeResponseDir creates the response folder and its parents if they do not
// exist.
func (s *Server) makeResponseDir() error {
	return os.MkdirAll(s.responsePath(nil), 0775)
}

// serve accepts connections on ln and serves each one in its own goroutine.
// A non-empty expectedUUID binds every connection to that VM.
func (s *Server) serve(addr string, ln net.Listener, expectedUUID string) {
	defer func() {
		s.listenersLock.Lock()
		defer s.listenersLock.Unlock()

		// CloseUnix removes the listener itself, and a new listener may
		// already use addr
		if s.listeners[addr] == ln {
			delete(s.listeners, addr)
		}
		log.Info("closed listener: %v", addr)
	}()

	max := 0
	if expectedUUID != "" {
		max = MaxBoundConns
	}

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

		if !s.track(addr, conn, max) {
			warnLimited(addr+":conns", "refused connection on %v: more than %d open connections", addr, max)
			conn.Close()
			continue
		}

		if !s.listening(addr, ln) {
			// CloseUnix closed ln, and may not see conn
			s.untrack(addr, conn)
			conn.Close()
			return
		}

		log.Debug("client connected: %v -> %v", conn.RemoteAddr(), addr)

		s.acceptWG.Add(1)
		go s.serveConn(addr, conn, expectedUUID)
	}
}

// serveConn runs the handshake and the client handler for one accepted
// connection.
func (s *Server) serveConn(addr string, conn net.Conn, expectedUUID string) {
	defer s.acceptWG.Done()
	defer s.untrack(addr, conn)
	defer conn.Close()
	defer recoverConn("cc connection on "+addr, conn)

	conn.SetReadDeadline(time.Now().Add(HandshakeTimeout))

	c, err := s.handshake(conn, expectedUUID)
	if err != nil {
		if err != io.EOF && refusals.allow(addr+":handshake") {
			// suppress EOF, VM was probably never started
			log.Error("handshake failed on %v: %v", addr, err)
		} else {
			log.Debug("handshake failed on %v: %v", addr, err)
		}
		return
	}

	conn.SetReadDeadline(time.Time{})

	if expectedUUID != "" {
		log.Debug("cc client %v connected on %v", c.uuid, addr)
	}

	s.clientHandler(c)
	log.Debug("client disconnected: %v", addr)
}

// handshake performs a handshake with the client, returning the new client if
// there were no errors.
func (s *Server) handshake(conn net.Conn, expectedUUID string) (res *client, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("handshake: recovered from panic: %v\n%s", r, debug.Stack())
			res, err = nil, fmt.Errorf("handshake failed: %v", r)
		}
	}()

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

	limit := &limitReader{r: conn, limit: MaxHandshakeSize}
	c := &client{
		conn:            conn,
		enc:             gob.NewEncoder(conn),
		dec:             gob.NewDecoder(limit),
		limit:           limit,
		pipeReaders:     make(map[string]*miniplumber.Reader),
		pipeWriters:     make(map[string]chan<- string),
		cancelHeartbeat: make(chan struct{}),
		pipesDone:       make(chan struct{}),
		policy:          CCPolicyLegacy,
	}

	// get the first client struct as a handshake
	var m Message
	if err := c.decode(&m); err != nil {
		return nil, err
	}

	if m.Type != MESSAGE_CLIENT {
		return nil, invalidf("handshake message type %v", m.Type)
	}

	// A serial or bound connection is created for one specific VM, so its
	// expected UUID is host-controlled. Guests without DMI can leave their
	// UUID empty and adopt this trusted identity from the handshake response.
	// Unbound clients must provide an identity of their own.
	if err := bindClientUUID(&m, expectedUUID); err != nil {
		return nil, err
	}

	if err := validateClient(m.Client); err != nil {
		return nil, err
	}

	if !s.UseVMs && !validToken(m.Client.UUID) {
		return nil, invalidf("client UUID %q", truncate(m.Client.UUID, maxErrorLog))
	}

	if majorVersion(m.Version) == 0 {
		// Only send heartbeats to clients that send a message version: older
		// clients fail on a message type that they do not recognize.
		return nil, fmt.Errorf("client %s: %w", m.Client.UUID, errClientTooOld)
	}

	if m.Client.Version != version.Revision {
		log.Warn("mismatched miniccc version on %v: %v", m.Client.Hostname, m.Client.Version)
	}

	update := s.updateFileFor(m.Client)

	if err := s.addClient(c, &m, update); err != nil {
		return nil, err
	}

	// addClient holds writeMu, so that the reply is the first message.
	// Send a copy, because route can change the fields of c.Client.
	ack, cl := m, *m.Client
	ack.Client = &cl
	err = c.encode(&ack)
	c.writeMu.Unlock()
	if err != nil {
		// client disconnected before it read the full handshake
		s.removeClient(c)
		if err != io.EOF {
			log.Errorln(err)
		}
		return nil, err
	}

	log.Debug("starting heartbeat to client %s", c.uuid)

	go func() {
		t := time.NewTicker(HEARTBEAT_RATE * time.Second)

		for {
			select {
			case <-c.cancelHeartbeat:
				log.Debug("stopping heartbeats to client %s", c.uuid)
				t.Stop()
				return
			case <-t.C:
				log.Debug("sending HEARTBEAT to client %s", c.uuid)
				m := Message{Type: MESSAGE_HEARTBEAT, Version: "v1"}
				c.sendMessage(&m) // a loss of connection is detected elsewhere.
			}
		}
	}()

	c.limit.limit = MaxMessageSize
	c.limit.small = LargeMessageSize
	c.limit.slots = largeMessages
	c.limit.done = c.cancelHeartbeat

	return c, nil
}

// addClient adds c, with the handshake message m, to the client list, and
// starts the auto-update of the client when update is the path of a binary. On
// success, it returns with c.writeMu held, so that the update starts after the
// reply to the handshake.
func (s *Server) addClient(c *client, m *Message, update string) error {
	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if _, ok := s.clients[m.Client.UUID]; ok {
		return fmt.Errorf("client %v already exists!", m.Client.UUID)
	}

	var namespace string

	if s.UseVMs {
		vm, ok := s.vms[m.Client.UUID]
		if !ok && uuidRE.MatchString(m.Client.UUID) {
			// try again after unmangling the uuid, which qemu does in certain
			// versions
			vm, ok = s.vms[unmangle(m.Client.UUID)]
			c.mangled = true
		}
		if !ok {
			return fmt.Errorf("unregistered client %v", truncate(m.Client.UUID, maxErrorLog))
		}
		if c.mangled {
			if _, ok := s.clients[unmangle(m.Client.UUID)]; ok {
				return fmt.Errorf("client %v already exists!", unmangle(m.Client.UUID))
			}
		}

		namespace = vm.GetNamespace()
		c.policy = vmPolicy(vm)
	}

	c.Namespace = namespace

	c.Client = m.Client
	if c.mangled {
		c.UUID = unmangle(m.Client.UUID)
	}
	log.Debug("new client: %v", m.Client)

	c.checkin = time.Now()
	c.uuid = c.UUID

	c.writeMu.Lock()
	s.clients[c.UUID] = c

	if update != "" {
		s.startUpdate(c, update)
	}

	return nil
}

var errUUIDMismatch = errors.New("client UUID does not match the channel")

// bindClientUUID applies the UUID that the host bound to the channel, if any,
// to the handshake message m. A client on a bound channel can send an empty
// UUID, the bound UUID, or its mangled form. Any other UUID is refused.
func bindClientUUID(m *Message, expectedUUID string) error {
	if m.Client == nil {
		return fmt.Errorf("client handshake missing client metadata")
	}

	claimed := m.Client.UUID
	if claimed != "" {
		if expectedUUID == "" {
			return nil
		}

		if !uuidMatches(claimed, expectedUUID) {
			warnLimited(expectedUUID+":uuid", "client on channel bound to VM %v claimed UUID %q -- closing connection", expectedUUID, truncate(claimed, maxErrorLog))
			return fmt.Errorf("%w: bound to %v, claimed %q", errUUIDMismatch, expectedUUID, truncate(claimed, maxErrorLog))
		}

		if strings.EqualFold(claimed, expectedUUID) {
			m.Client.UUID = expectedUUID
			m.UUID = expectedUUID
		}

		return nil
	}

	if expectedUUID == "" {
		return fmt.Errorf("client handshake missing UUID without serial binding")
	}

	m.Client.UUID = expectedUUID
	m.UUID = expectedUUID
	log.Info("bound client to VM UUID %s", expectedUUID)
	return nil
}

// client and transport handler for connections.
func (s *Server) clientHandler(c *client) {
	defer recoverConn("cc client "+c.uuid, c.conn)
	defer c.conn.Close()
	defer s.removeClient(c)

	// Set up minitunnel, dialing the server that should be running on the
	// client's side. Data is Trunk'd via Messages.
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	go Trunk(remote, c.uuid, c.sendMessage)

	go func() {
		tunnel, err := minitunnel.DialWithPolicy(local, s.tunnelPolicy(c))
		if err != nil {
			log.Error("dial: %v", err)
			return
		}

		s.clientLock.Lock()
		defer s.clientLock.Unlock()

		log.Debug("minitunnel created for %v", c.uuid)
		c.tunnel = tunnel
	}()

	// send the commands to our new client
	go s.sendCommands(c.uuid)

	var err error

	for err == nil {
		if s.destroyed() {
			return
		}

		var m Message
		if err = c.decode(&m); err == nil {
			err = s.handleMessage(c, &m, remote)
		}
	}

	// This is an OK error - likely just means the TCP connection was closed.
	if err == io.EOF {
		return
	}

	if errors.Is(err, errInvalidMessage) || errors.Is(err, errMessageTooLarge) {
		warnLimited(c.uuid+":invalid", "closing cc client %v: %v", c.uuid, err)
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

// handleMessage handles one message from client c. Messages are bound to the
// connection: the UUID inside a message is ignored. An error closes the
// connection.
func (s *Server) handleMessage(c *client, m *Message, remote net.Conn) error {
	log.Debug("new message: %v", m.Type)

	m.UUID = c.uuid

	if s.onMessage != nil {
		s.onMessage(m)
	}

	switch m.Type {
	case MESSAGE_TUNNEL:
		_, err := remote.Write(m.Tunnel)
		return err
	case MESSAGE_FILE:
		if m.File == nil {
			return invalidf("file message without a file")
		}
		if m.Error != "" {
			warnLimited(c.uuid+":fileerror", "file error from %v: %v", c.uuid, truncate(m.Error, maxErrorLog))
			return nil
		}
		if m.File.Data == nil && m.File.Offset == 0 {
			// client requested file
			return s.sendFile(c, m.File.Name)
		}
		// client sent file
		return s.recvFile(c, m.File)
	case MESSAGE_CLIENT:
		if err := validateClient(m.Client); err != nil {
			return err
		}
		m.Client.UUID = c.uuid

		var n int64
		if c.limit != nil {
			n = c.limit.size()
		}
		s.queueWait(n)
		s.responses <- queuedClient{m.Client, n}
	case MESSAGE_COMMAND:
		// this shouldn't be sent via the client...
	case MESSAGE_PIPE:
		return c.pipeHandler(s.plumber, m)
	case MESSAGE_UFS:
		c.ufsMessage(m)
	default:
		return invalidf("unknown message type: %v", m.Type)
	}

	return nil
}

// removeClient removes c from the client list, if it is still there, and
// closes its connection and plumbing.
func (s *Server) removeClient(c *client) {
	log.Debug("removeClient: %v", c.uuid)

	s.clientLock.Lock()
	defer s.clientLock.Unlock()

	if s.clients[c.uuid] == c {
		delete(s.clients, c.uuid)
	}

	c.shutdown()
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
		if !v.Once || !v.Sent || (uuid != "" && v.heldFor[uuid]) {
			m.Commands[k] = v.Copy()
			v.Sent = true
		}
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

// sendFile reads the file and sends it in multiple chunks to the client. The
// file must be inside the files directory, or inside its subpath.
func (s *Server) sendFile(c *client, filename string) error {
	log.Debug("sendFile: %v to %v", truncate(filename, maxErrorLog), c.uuid)

	if !validFileName(filename) {
		warnLimited(c.uuid+":file", "cc client %v: refused request for file %q outside the files directory", c.uuid, truncate(filename, maxErrorLog))
		return sendFileError(0, filename, errors.New("invalid file name"), c.sendMessage)
	}

	// try to send version from subpath first
	var f *os.File
	var err error
	for _, dir := range []string{filepath.Join(s.path, s.subpath), s.path} {
		f, err = openInRoot(dir, filename)
		if err == nil || !errors.Is(err, fs.ErrNotExist) {
			break
		}
	}

	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			warnLimited(c.uuid+":nofile", "cannot open file %v: %v", truncate(filename, maxErrorLog), err)
		} else {
			warnLimited(c.uuid+":file", "cc client %v: refused request for file %q: %v", c.uuid, truncate(filename, maxErrorLog), err)
		}
		return sendFileError(0, filename, err, c.sendMessage)
	}
	defer f.Close()

	return sendOpenFile(f, filename, 0, PART_SIZE, c.sendMessage)
}

// recvFile writes one part of a file that the client sent. The file must stay
// inside the response directory of its command and client.
func (s *Server) recvFile(c *client, f *File) error {
	if !validFileName(f.Name) {
		warnLimited(c.uuid+":file", "cc client %v: refused file %q outside the responses directory", c.uuid, truncate(f.Name, maxErrorLog))
		return invalidf("file name %q", truncate(f.Name, maxErrorLog))
	}

	if f.ID < 0 || f.Offset < 0 || len(f.Data) > MaxFileChunk || f.Offset > MaxUploadFileSize-int64(len(f.Data)) {
		return invalidf("file %q: command %v, offset %v, %v bytes", truncate(f.Name, maxErrorLog), f.ID, f.Offset, len(f.Data))
	}

	if !s.liveCommand(f.ID) {
		warnLimited(c.uuid+":file", "cc client %v: dropped file %q: no command %v", c.uuid, truncate(f.Name, maxErrorLog), f.ID)
		return nil
	}

	if f.Offset == 0 {
		entries := 1 + int64(strings.Count(filepath.ToSlash(filepath.Clean(f.Name)), "/"))
		if !s.chargeEntries(c.uuid, entries) {
			warnLimited(c.uuid+":upload", "cc client %v: refused file %q: more than %d files and directories in one hour", c.uuid, truncate(f.Name, maxErrorLog), MaxUploadEntriesPerHour)
			return invalidf("upload limit")
		}
	}

	if !s.chargeUpload(c.uuid, int64(len(f.Data))) {
		warnLimited(c.uuid+":upload", "cc client %v: refused file %q: more than %d bytes in one hour", c.uuid, truncate(f.Name, maxErrorLog), int64(MaxUploadPerHour))
		return invalidf("upload limit")
	}

	if err := s.makeResponseDir(); err != nil {
		return err
	}

	dir := filepath.Join(s.responsePath(&f.ID), c.uuid)
	if err := os.MkdirAll(dir, os.FileMode(0770)); err != nil {
		return err
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()

	err = f.recvInRoot(root, dir)
	if f.Offset > 0 && errors.Is(err, fs.ErrNotExist) {
		warnLimited(c.uuid+":file", "cc client %v: dropped part of file %q: the file did not start at offset 0", c.uuid, truncate(f.Name, maxErrorLog))
		return nil
	}

	return err
}

// route an outgoing message to one or all clients, according to UUID
func (s *Server) route(m *Message) {
	var maxCommandID int
	for i := range m.Commands {
		if i > maxCommandID {
			maxCommandID = i
		}
	}

	type delivery struct {
		c *client
		m *Message
	}

	var deliveryLock, issuedLock sync.Mutex
	var deliveries []delivery

	// prepare finds the message for one client. The caller must hold
	// clientLock.
	prepare := func(uuid string) {
		// create locally scoped pointer to message
		m := m

		c, ok := s.clients[uuid]
		if !ok {
			log.Error("no such client %v", uuid)
			return
		}

		if c.maxCommandID == maxCommandID {
			log.Debug("no commands for %v", uuid)
			return
		}

		if m.Type == MESSAGE_COMMAND {
			if s.UseVMs {
				filters := make([]*Filter, 0, len(m.Commands))
				for _, cmd := range m.Commands {
					filters = append(filters, cmd.Filter)
				}

				if !s.loadVMTags(uuid, c, filters) {
					// odd, someone must have unregistered the client...
					log.Error("unregistered client %v", uuid)
					return
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

			if len(m2.Commands) == 0 {
				c.maxCommandID = maxCommandID
				log.Debug("no commands for %v", uuid)
				return
			}

			// keep maxCommandID so that the commands are sent after the update
			if c.updating {
				issuedLock.Lock()
				defer issuedLock.Unlock()
				for id, cmd := range m2.Commands {
					if cmd.Once {
						s.commands[id].holdFor(uuid)
					}
				}

				log.Info("holding %v commands for %v during its update", len(m2.Commands), uuid)
				return
			}

			c.maxCommandID = maxCommandID

			m = &m2
		}

		deliveryLock.Lock()
		defer deliveryLock.Unlock()
		deliveries = append(deliveries, delivery{c, m})
	}

	// prepare doesn't modify the clients map so we can allow parallel reads
	s.clientLock.Lock()
	if m.UUID != "" {
		prepare(m.UUID)
	} else {
		var wg sync.WaitGroup
		for uuid := range s.clients {
			wg.Add(1)

			go func(uuid string) {
				defer wg.Done()

				prepare(uuid)
			}(uuid)
		}
		wg.Wait()
	}
	s.clientLock.Unlock()

	// send without clientLock, in parallel, so that a client that does not
	// read cannot block the others
	var wg sync.WaitGroup

	for _, d := range deliveries {
		wg.Add(1)

		go func(d delivery) {
			defer wg.Done()

			if err := d.c.sendMessage(d.m); err != nil {
				if strings.Contains(err.Error(), "broken pipe") {
					log.Debug("client disconnected: %v", d.c.uuid)
				} else {
					log.Info("unable to send message to %v: %v", d.c.uuid, err)
				}
			} else if d.m.Type == MESSAGE_COMMAND {
				issuedLock.Lock()
				defer issuedLock.Unlock()
				for id := range d.m.Commands {
					s.commands[id].Issued++
					delete(s.commands[id].heldFor, d.c.uuid)
				}
			}
		}(d)
	}

	wg.Wait()
}

// loadVMTags sets c.Tags from the tags of the VM registered for uuid, then
// overrides each key in the Tags of filters with the non-empty VM info field of
// the same name, so that c.Matches can match on them. It returns false when no
// VM is registered for uuid. The caller must hold clientLock.
func (s *Server) loadVMTags(uuid string, c *client, filters []*Filter) bool {
	vm, ok := s.vms[uuid]
	if !ok {
		return false
	}

	c.Tags = vm.GetTags()

	for _, f := range filters {
		if f == nil || f.Tags == nil {
			continue
		}

		for k := range f.Tags {
			if v, _ := vm.Info(k); v != "" {
				c.Tags[k] = v
			}
		}
	}

	return true
}

// process responses, writing files when necessary
func (s *Server) responseHandler() {
	defer close(s.handlerDone)

	for q := range s.responses {
		s.handleResponses(q.cl)
		s.queueDone(q.size)
	}
}

// queuedClient is a client update in the responses queue, with its encoded
// size.
type queuedClient struct {
	cl   *Client
	size int64
}

// queueWait waits until the responses queue has room for n bytes, and
// reserves them. A queue that is empty accepts any size.
func (s *Server) queueWait(n int64) {
	s.queueLock.Lock()
	defer s.queueLock.Unlock()

	for s.queued > 0 && s.queued+n > MaxQueuedResponses {
		s.queueCond.Wait()
	}
	s.queued += n
}

func (s *Server) queueDone(n int64) {
	s.queueLock.Lock()
	defer s.queueLock.Unlock()

	s.queued -= n
	s.queueCond.Broadcast()
}

// handleResponses processes one client update. A panic is logged and skips
// the update, so that the response handler keeps running.
func (s *Server) handleResponses(cin *Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("responseHandler: recovered from panic: %v\n%s", r, debug.Stack())
		}
	}()

	log.Debug("responseHandler: %v", cin.UUID)

	// update all the client fields
	s.updateClient(cin)

	// ingest responses from this client
	for _, v := range cin.Responses {
		log.Debug("got response %v : %v", cin.UUID, v.ID)
		if !s.liveCommand(v.ID) {
			warnLimited(cin.UUID+":response", "cc client %v: dropped response %v: no such command", cin.UUID, v.ID)
			continue
		}

		if s.chargeUpload(cin.UUID, int64(len(v.Stdout)+len(v.Stderr))) {
			s.writeResponse(cin.UUID, v)
		} else {
			warnLimited(cin.UUID+":upload", "cc client %v: dropped response %v: more than %d bytes in one hour", cin.UUID, v.ID, int64(MaxUploadPerHour))
		}

		// count the response only once its files can be read, since cc
		// users poll the count and then read the files
		s.commandCheckIn(v.ID, cin.UUID)
	}
}

// writeResponse records a client's response to a command in the response
// folder. Errors are logged: the client did respond, so it is still counted.
func (s *Server) writeResponse(uuid string, v *Response) {
	path := filepath.Join(s.responsePath(&v.ID), uuid)

	if err := s.makeResponseDir(); err != nil {
		log.Error("could not record response %v for %v: %v", v.ID, uuid, err)
		return
	}

	if err := os.MkdirAll(path, os.FileMode(0770)); err != nil {
		log.Error("could not record response %v for %v: %v", v.ID, uuid, err)
		return
	}

	// generate exitcode file
	if v.RecordExitCode {
		err := ioutil.WriteFile(filepath.Join(path, "exitcode"), []byte(strconv.Itoa(v.ExitCode)), os.FileMode(0660))
		if err != nil {
			log.Error("could not record exit code %v for %v: %v", v.ID, uuid, err)
		}
	}

	// generate stdout and stderr if they exist
	if v.Stdout != "" {
		err := ioutil.WriteFile(filepath.Join(path, "stdout"), []byte(v.Stdout), os.FileMode(0660))
		if err != nil {
			log.Error("could not record stdout %v for %v: %v", v.ID, uuid, err)
		}
	}

	if v.Stderr != "" {
		err := ioutil.WriteFile(filepath.Join(path, "stderr"), []byte(v.Stderr), os.FileMode(0660))
		if err != nil {
			log.Error("could not record stderr %v for %v: %v", v.ID, uuid, err)
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
		log.Debug("unknown client %v", cin.UUID)
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
		log.Debug("unregistered client %v", cin.UUID)
		return
	}

	if len(cin.Tags) == 0 {
		return
	}

	tags := vm.GetTags()
	size := 0
	for k, v := range tags {
		size += len(k) + len(v)
	}

	keys := make([]string, 0, len(cin.Tags))
	for k := range cin.Tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := cin.Tags[k]
		old, exists := tags[k]

		n := size + len(k) + len(v)
		if exists {
			n -= len(k) + len(old)
		}

		if (!exists && len(tags) >= MaxVMTags) || n > MaxVMTagBytes {
			warnLimited(cin.UUID+":tags", "cc client %v: dropped tag %q: more than %d tags or %d bytes of tags", cin.UUID, truncate(k, maxErrorLog), MaxVMTags, MaxVMTagBytes)
			continue
		}

		vm.SetTag(k, v)
		tags[k] = v
		size = n
	}
}

// commandCheckIn marks that the client uuid responded to command id. It
// returns false when there is no such command.
func (s *Server) commandCheckIn(id int, uuid string) bool {
	log.Debug("commandCheckIn %v %v", id, uuid)

	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	c, ok := s.commands[id]
	if ok && !slices.Contains(c.CheckedIn, uuid) {
		c.CheckedIn = append(c.CheckedIn, uuid)
	}

	return ok
}

// liveCommand reports whether command id is in the active command list.
func (s *Server) liveCommand(id int) bool {
	s.commandLock.Lock()
	defer s.commandLock.Unlock()

	_, ok := s.commands[id]
	return ok
}

// clientReaper periodically flushes old entries from the client list
func (s *Server) clientReaper() {
	ticker := time.NewTicker(s.reaperRate)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopReaper:
			log.Debug("reaping client reaper")
			return
		case <-ticker.C:
		}

		s.clientLock.Lock()
		for k, v := range s.clients {
			// checked in more recently than expiration time
			active := time.Now().Sub(v.checkin) < s.clientExpiry

			if !active {
				log.Debug("client %v expired", k)
				// the same as removeClient except we already hold clientLock
				delete(s.clients, k)
				v.shutdown()
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
