// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	"io"
	log "minilog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	MODE_MASTER = iota
	MODE_CLIENT
)

const (
	HEARTBEAT_RATE = 5
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
	RESPONSE_PATH  = "miniccc_responses"
)

type Ron struct {
	UUID string

	mode   int
	port   int
	parent string
	rate   int
	path   string

	// serial port support
	serialPath         string
	serialClientHandle io.ReadWriteCloser
	masterSerialConns  map[string]io.ReadWriteCloser

	commands           map[int]*Command
	commandCounter     int
	commandLock        sync.Mutex
	commandCounterLock sync.Mutex

	masterResponseQueue chan []*Response

	responseQueueLock   sync.Mutex
	clientLock          sync.Mutex
	clients             map[string]*Client
	clientResponseQueue []*Response
	clientCommandQueue  chan map[int]*Command
	clientExpiredCount  int

	OSVer     string
	CSDVer    string
	EditionID string
}

type Client struct {
	UUID      string
	Hostname  string
	Arch      string
	OS        string
	IP        []string
	MAC       []string
	Checkin   time.Time
	Responses []*Response
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, or client.
func New(port int, mode int, parent string, path string) (*Ron, error) {
	uuid, err := getUUID()
	if err != nil {
		return nil, err
	}

	r := &Ron{
		UUID:                uuid,
		port:                port,
		mode:                mode,
		parent:              parent,
		rate:                HEARTBEAT_RATE,
		path:                path,
		commands:            make(map[int]*Command),
		clients:             make(map[string]*Client),
		clientCommandQueue:  make(chan map[int]*Command, 1024),
		masterResponseQueue: make(chan []*Response, 1024),
	}

	switch mode {
	case MODE_MASTER:
		if parent != "" {
			return nil, fmt.Errorf("master mode must have no parent")
		}

		r.masterSerialConns = make(map[string]io.ReadWriteCloser)

		err := r.startMaster()
		if err != nil {
			log.Errorln(err)
			return nil, err
		}
		go r.expireReaper()
		go r.clientReaper()
		go r.masterResponseProcessor()
	case MODE_CLIENT:
		if parent == "" {
			return nil, fmt.Errorf("client mode must have parent")
		}

		err := r.startClient()
		if err != nil {
			log.Errorln(err)
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %#v", r)

	return r, nil
}

// NewSerial is a special New for clients only that connects on a serial port
// instead of over tcp.
func NewSerial(serialPath string, mode int, path string) (*Ron, error) {
	uuid, err := getUUID()
	if err != nil {
		return nil, err
	}

	r := &Ron{
		UUID:                uuid,
		mode:                mode,
		rate:                HEARTBEAT_RATE,
		path:                path,
		serialPath:          serialPath,
		commands:            make(map[int]*Command),
		clients:             make(map[string]*Client),
		clientCommandQueue:  make(chan map[int]*Command, 1024),
		masterResponseQueue: make(chan []*Response, 1024),
	}

	switch mode {
	case MODE_MASTER:
		return nil, fmt.Errorf("NewSerial can only be invoked as a client")
	case MODE_CLIENT:
		err := r.startClient()
		if err != nil {
			log.Errorln(err)
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %#v", r)

	return r, nil
}

func (r *Ron) PostResponse(response *Response) {
	r.responseQueueLock.Lock()
	defer r.responseQueueLock.Unlock()
	r.clientResponseQueue = append(r.clientResponseQueue, response)
}

func (r *Ron) GetCommands() map[int]*Command {
	// return a deep copy of the command list
	ret := make(map[int]*Command)
	r.commandLock.Lock()
	defer r.commandLock.Unlock()

	for k, v := range r.commands {
		ret[k] = &Command{
			ID:             v.ID,
			Record:         v.Record,
			Background:     v.Background,
			Command:        v.Command,
			FilesSend:      v.FilesSend,
			FilesRecv:      v.FilesRecv,
			checkedIn:      v.checkedIn,
			ExpireClients:  v.ExpireClients,
			ExpireStarted:  v.ExpireStarted,
			ExpireDuration: v.ExpireDuration,
			ExpireTime:     v.ExpireTime,
		}
		for _, y := range v.Filter {
			ret[k].Filter = append(ret[k].Filter, y)
		}
	}
	return ret
}

func (r *Ron) GetNewCommands() map[int]*Command {
	return <-r.clientCommandQueue
}

func (r *Ron) GetPort() int {
	return r.port
}

func (r *Ron) GetActiveClients() []string {
	var clients []string

	r.clientLock.Lock()
	defer r.clientLock.Unlock()

	for c, _ := range r.clients {
		clients = append(clients, c)
	}

	log.Debug("active clients: %v", clients)
	return clients
}

func (r *Ron) startMaster() error {
	log.Debugln("startMaster")

	err := os.MkdirAll(r.path, 0775)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(r.path, RESPONSE_PATH), 0775)
	if err != nil {
		return err
	}

	http.Handle("/files/", http.StripPrefix("/files", http.FileServer(http.Dir(r.path))))
	http.HandleFunc("/ron/", easter)
	http.HandleFunc("/heartbeat", r.handleHeartbeat)

	host := fmt.Sprintf(":%v", r.port)

	// TODO: allow graceful shutdown of ron nodes
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	return nil
}

func (r *Ron) startClient() error {
	log.Debugln("startClient")

	if r.serialPath != "" {
		err := r.serialDial()
		if err != nil {
			return err
		}
	}

	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}
