// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"sort"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// mux routes incoming messages from the server based on message type
func mux(done chan struct{}) {
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
		var (
			m ron.Message
			d = time.Duration(math.Round(2.5 * ron.HEARTBEAT_RATE))
		)

		err = timeout(d*time.Second, func() (err error) {
			err = client.dec.Decode(&m)
			if err != nil {
				err = fmt.Errorf("decoding cc message: %w", err)
			}

			return
		})

		if errors.Is(err, errTimeout) || errors.Is(err, io.EOF) {
			log.Warn("server connection lost: resetting client")

			// server connection lost, so reset client
			resetClient()
			return
		}

		if err != nil {
			break
		}

		log.Debug("new message: %v", m.Type)

		switch m.Type {
		case ron.MESSAGE_CLIENT:
			// ACK of the handshake
			log.Info("handshake complete")

			go periodic(done)
			go commandHandler(done)
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
		case ron.MESSAGE_HEARTBEAT:
			// Don't need to do anything with these... they just get sent by the
			// server on a known frequency so the client can detect a loss of
			// connection when using the virtual serial port.
		default:
			err = fmt.Errorf("unknown message type: %v", m.Type)
		}
	}

	log.Info("mux exit: %v", err)
}

func commandHandler(done chan struct{}) {
	for {
		select {
		case commands := <-client.commandChan:
			var ids []int
			for k, _ := range commands {
				ids = append(ids, k)
			}
			sort.Ints(ids)

			for _, id := range ids {
				processCommand(commands[id])
			}
		case <-done:
			log.Info("command handler exit")
			return
		}
	}
}
