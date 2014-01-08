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
	DEFAULT_RATE = 5
)

type ron struct {
	mode          int
	parent        string
	port          int
	buffer        bytes.Buffer
	Enc           *gob.Encoder

	rate          int
}

// New creates and returns a new ron object. Mode specifies if this object
// should be a master, relay, or client.
func NewRon(mode int, parent string, port int) (*ron, error) {
	r := &ron{
		mode:          mode,
		parent:        parent,
		port:          port,
		userHeartbeat: heartbeat,
		rate:          DEFAULT_RATE,
	}

	r.Enc = gob.NewEncoder(&r.buffer)

	switch mode {
	case MODE_MASTER:
		// a master node is simply a relay with no parent
		if parent != "" {
			return nil, fmt.Errorf("master mode must have no parent")
		}

		err := r.newRelay()
		if err != nil {
			return nil, err
		}
	case MODE_RELAY:
		if parent == "" {
			return nil, fmt.Errorf("relay mode must have a parent")
		}

		err := r.newRelay()
		if err != nil {
			return nil, err
		}
	case MODE_CLIENT:
		if parent == "" {
			return nil, fmt.Errorf("client mode must have a parent")
		}

		err := r.newClient()
		if err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid mode %v", mode)
	}

	log.Debug("registered new ron node: %v %v %v", mode, parent, port)
	return r, nil
}

func (r *ron) heartbeat() {
	for {
		time.Sleep(time.Duration(r.rate) * time.Second)
		r.userHeartbeat(r)
		host := fmt.Sprintf("http://%v:%v/heartbeat", r.parent, r.port)
		resp, err := http.Post(host, "ron/miniccc", &r.buffer)
		if err != nil {
			log.Errorln(err)
			continue
		}
		log.Debugln(resp.Body)
		resp.Body.Close()
	}
}

func (r *ron) newRelay() error {
	log.Debugln("newRelay")
	http.HandleFunc("/ron", easter)
	http.HandleFunc("/heartbeat", handleHeartbeat)
	http.HandleFunc("/", http.NotFound)

	host := fmt.Sprintf(":%v", r.port)
	go func() {
		log.Fatalln(http.ListenAndServe(host, nil))
	}()

	return nil
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
	var buf bytes.Buffer
	io.Copy(&buf, r.Body)

}

func (r *ron) newClient() error {
	// start the periodic query to the parent
	go r.heartbeat()

	return nil
}
