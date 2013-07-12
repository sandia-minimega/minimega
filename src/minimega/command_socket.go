// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"io"
	log "minilog"
	"net"
	"os"
)

func commandSocketStart() {
	l, err := net.Listen("unix", *f_base+"minimega")
	if err != nil {
		log.Errorln(err)
		teardown()
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Errorln(err)
		}
		log.Infoln("client connected")
		commandSocketHandle(conn) // don't allow multiple connections
	}
}

func commandSocketRemove() {
	f := *f_base + "minimega"
	err := os.Remove(f)
	if err != nil {
		log.Errorln(err)
	}
}

func commandSocketHandle(c net.Conn) {
	enc := json.NewEncoder(c)
	dec := json.NewDecoder(c)
	done := false
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
		// just shove it in the cli command channel
		commandChanSocket <- c
		for {
			r := <-ackChanSocket
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
