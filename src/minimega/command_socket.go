package main

import (
	"encoding/json"
	"io"
	log "minilog"
	"net"
	"os"
)

func command_socket_start() {
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
		command_socket_handle(conn) // don't allow multiple connections
	}
}

func command_socket_remove() {
	f := *f_base + "minimega"
	err := os.Remove(f)
	if err != nil {
		log.Errorln(err)
	}
}

func command_socket_handle(c net.Conn) {
	enc := json.NewEncoder(c)
	dec := json.NewDecoder(c)
	for {
		var c cli_command
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
		command_chan_socket <- c
		r := <-ack_chan_socket
		err = enc.Encode(&r)
		if err != nil {
			if err == io.EOF {
				log.Infoln("command client disconnected")
			} else {
				log.Errorln(err)
			}
			break
		}
	}
}
