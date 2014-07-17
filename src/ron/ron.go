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
)

type Ron struct {
	UUID string

	mode   int
	port   int
	parent string
	rate   int
	path   string

	clients map[string]*Client

	commands            map[int]*Command
	commandCounter      int
	commandLock         sync.Mutex
	commandCounterLock  sync.Mutex
	clientCommandQueue  chan map[int]*Command
	masterResponseQueue chan []*Response
	responseQueue       []*Response

	responseQueueLock sync.Mutex

	clientLock sync.Mutex

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

func getUUID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		d, err := ioutil.ReadFile("/sys/devices/virtual/dmi/id/product_uuid")
		if err != nil {
			return "", err
		}
		uuid := string(d[:len(d)-1])
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

	err = os.MkdirAll(r.path+"/miniccc_responses", 0775)
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
