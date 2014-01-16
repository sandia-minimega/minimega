// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	log "minilog"
	"net/http"
	"time"
)

const (
	MODE_MASTER = iota
	MODE_RELAY
	MODE_CLIENT
)

const (
	HEARTBEAT_RATE = 5
	REAPER_RATE    = 30
	CLIENT_EXPIRED = 30
)

type hb struct {
	ID           string
	Clients      map[string]*Client
	MaxCommandID int // the highest command ID this node has seen
	Responses    []*Response
}

type ron struct {
	mode   int
	parent string
	port   int
	host   string
	rate   int
}

type RonNode interface {
	Heartbeat() *hb
	Commands(map[int]*Command)
}

func init() {
	gob.Register(hb{})
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
func (r *ron) Start(mode int, parent string, port int) error {
	r = &ron{
		mode:   mode,
		parent: parent,
		port:   port,
		rate:   HEARTBEAT_RATE,
		host:   fmt.Sprintf("http://%v:%v/heartbeat", parent, port),
	}

	switch mode {
	case MODE_MASTER:
		// a master node is simply a relay with no parent
		if parent != "" {
			return fmt.Errorf("master mode must have no parent")
		}

		err := r.newRelay()
		if err != nil {
			return err
		}
	case MODE_RELAY:
		if parent == "" {
			return fmt.Errorf("relay mode must have a parent")
		}

		err := r.newRelay()
		if err != nil {
			return err
		}
	case MODE_CLIENT:
		if parent == "" {
			return fmt.Errorf("client mode must have a parent")
		}

		err := r.newClient()
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %v %v %v", mode, parent, port)
	return nil
}

func (r *ron) newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron/", easter)
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/list/", handleList)
	http.HandleFunc("/list/raw", handleList)
	http.HandleFunc("/command/", handleCommands)
	http.HandleFunc("/command/new", handleNewCommand)
	http.HandleFunc("/command/delete", handleDeleteCommand)
	http.HandleFunc("/command/deletefiles", handleDeleteFiles)
	http.HandleFunc("/command/resubmit", handleResubmit)
	http.HandleFunc("/", handleRoot)

	host := fmt.Sprintf(":%v", r.port)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	go r.heartbeat()

	return nil
}

func (r *ron) newClient() error {
	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}

func (r *ron) heartbeat() {
	for {
		time.Sleep(time.Duration(r.rate) * time.Second)

		var h *hb
		switch r.mode {
		case MODE_MASTER:
			// do nothing
			return
		case MODE_RELAY:
			h = relayHeartbeat()
		case MODE_CLIENT:
			h = clientHeartbeat()
		default:
			log.Fatal("invalid heartbeat mode %v", r.mode)
		}

		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)

		err := enc.Encode(h)
		if err != nil {
			log.Errorln(err)
			continue
		}

		resp, err := http.Post(r.host, "ron/miniccc", &buf)
		if err != nil {
			log.Errorln(err)
			continue
		}

		newCommands := make(map[int]*Command)
		dec := gob.NewDecoder(resp.Body)

		err = dec.Decode(newCommands)
		if err != nil {
			log.Errorln(err)
			resp.Body.Close()
			continue
		}

		switch r.mode {
		case MODE_RELAY:
			// replace the command list with this one, keeping the list of respondents
			updateCommands(newCommands)
		case MODE_CLIENT:
			clientCommands(newCommands)
		}

		resp.Body.Close()
	}
}

// heartbeat is the means of communication between clients and an upstream
// parent. Clients will send status and any responses from completed commands
// in a POST, while existing commands will be returned as the response.
func handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		log.Error("no data received: %v %v", r.RemoteAddr, r.URL)
		return
	}
	defer r.Body.Close()
	dec := gob.NewDecoder(r.Body)
	var h hb
	err := dec.Decode(&h)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debug("heartbeat from %v", h.ID)

	// process the heartbeat in a goroutine so we can send the command list back faster
	go processHeartbeat(&h)

	// send the command list back
	w.Write(encodeCommands())
}
