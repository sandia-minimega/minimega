// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"net/http"
	"os"
	"runtime"
	"strings"
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
	RESPONSE_PATH  = "/miniccc_responses"
)

type Ron struct {
	UUID string

	mode   int
	port   int
	parent string
	rate   int
	path   string

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
	OSVer     string
	CSDVer    string
	EditionID string
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
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
		// a master node is a relay with no parent
		if parent != "" {
			return nil, fmt.Errorf("master mode must have no parent")
		}

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

func getUUID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		d, err := ioutil.ReadFile("/sys/devices/virtual/dmi/id/product_uuid")
		if err != nil {
			return "", err
		}
		uuid := string(d[:len(d)-1])
		uuid = strings.ToLower(uuid)
		log.Debug("got UUID: %v", uuid)
		return uuid, nil
	default:
		return "", fmt.Errorf("OS %v UUID not supported yet", runtime.GOOS)
	}
}

func (r *Ron) startMaster() error {
	log.Debugln("startMaster")

	err := os.MkdirAll(r.path, 0775)
	if err != nil {
		return err
	}

	err = os.MkdirAll(r.path+RESPONSE_PATH, 0775)
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

	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}
