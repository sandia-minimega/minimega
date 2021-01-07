// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"fmt"
	"io"
	log "minilog"
	"minitunnel"
	"net"
	"ron"
	"sort"
)

// mux routes incoming messages from the server based on message type
func mux() {
	// start piping data to minitunnel and trunking it over the ron
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	go func() {
		if err := minitunnel.ListenAndServe(local); err != nil {
			log.Error("ListenAndServe: %v", err)
		}
	}()

	go ron.Trunk(remote, client.UUID, sendMessage)

	// Read messages from gob, mux message to the correct place
	var err error

	log.Debug("starting mux")

	for err == nil {
		var m ron.Message
		if err = client.dec.Decode(&m); err == io.EOF {
			// server disconnected... try to reconnect
			err = dial()
			continue
		} else if err != nil {
			break
		}

		log.Debug("new message: %v", m.Type)

		switch m.Type {
		case ron.MESSAGE_CLIENT:
			// ACK of the handshake
			log.Info("handshake complete")
			go periodic()
			go commandHandler()
		case ron.MESSAGE_COMMAND:
			client.commandChan <- m.Commands
		case ron.MESSAGE_FILE:
			client.fileChan <- &m
		case ron.MESSAGE_TUNNEL:
			_, err = remote.Write(m.Tunnel)
		case ron.MESSAGE_PIPE:
			pipeMessage(&m)
		case ron.MESSAGE_UFS:
			ufsMessage(&m)
		default:
			err = fmt.Errorf("unknown message type: %v", m.Type)
		}
	}

	log.Info("mux exit: %v", err)
}

func commandHandler() {
	for commands := range client.commandChan {
		var ids []int
		for k, _ := range commands {
			ids = append(ids, k)
		}
		sort.Ints(ids)

		for _, id := range ids {
			processCommand(commands[id])
		}
	}

	log.Info("command handler exit")
}
