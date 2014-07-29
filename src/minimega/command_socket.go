// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"io"
	"math/rand"
	log "minilog"
	"net"
	"os"
	"sync"
	"time"
)

var (
	commandSocketLock   sync.Mutex
	commandSocketRoutes map[int32]chan cliResponse
)

func init() {
	commandSocketRoutes = make(map[int32]chan cliResponse)
	go commandSocketMux()
}

func commandSocketStart() {
	l, err := net.Listen("unix", *f_base+"minimega")
	if err != nil {
		log.Error("commandSocketStart: %v", err)
		teardown()
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("commandSocketStart: accept: %v", err)
		}
		log.Infoln("client connected")
		go commandSocketHandle(conn)
	}
}

func commandSocketRemove() {
	f := *f_base + "minimega"
	err := os.Remove(f)
	if err != nil {
		log.Errorln(err)
	}
}

func socketRegister(TID int32, c chan cliResponse) {
	commandSocketLock.Lock()
	defer commandSocketLock.Unlock()

	if _, ok := commandSocketRoutes[TID]; ok {
		log.Error("TID %v already registered", TID)
	} else {
		commandSocketRoutes[TID] = c
	}
}

func socketUnregister(TID int32) {
	commandSocketLock.Lock()
	defer commandSocketLock.Unlock()

	if _, ok := commandSocketRoutes[TID]; ok {
		delete(commandSocketRoutes, TID)
	} else {
		log.Error("TID %v not registered", TID)
	}
}

func commandSocketMux() {
	log.Debug("commandSocketMux")
	for {
		c := <-ackChanSocket
		respChan, ok := commandSocketRoutes[c.TID]
		if !ok {
			log.Error("commandSocket invalid TID %v", c.TID)
			continue
		}
		respChan <- c
	}
}

func commandSocketHandle(c net.Conn) {
	enc := json.NewEncoder(c)
	dec := json.NewDecoder(c)
	done := false

	respChan := make(chan cliResponse, 1024)

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	TID := r.Int31()

	socketRegister(TID, respChan)
	defer socketUnregister(TID)

	for !done {
		var c cliCommand
		err := dec.Decode(&c)
		if err != nil {
			if err == io.EOF {
				log.Infoln("command client disconnected")
			} else {
				log.Errorln(err)
			}
			break
		}

		c.TID = TID

		log.Debug("got command over socket: %v", c)

		// just shove it in the cli command channel
		commandChanSocket <- c
		for {
			r := <-respChan
			err = enc.Encode(&r)
			if err != nil {
				if err == io.EOF {
					log.Infoln("command client disconnected")
				} else {
					log.Errorln(err)
				}
				done = true
			}
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}
}
