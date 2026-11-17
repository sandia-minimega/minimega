// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"fmt"
	"math"
	"net"
	"sort"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/minitunnel"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// serve starts to serve the connection that dial opened and sends the
// handshake. It returns a function that waits until the connection is lost
// and returns the cause.
func serve() func() error {
	client.Lock()
	conn, dec, lost := client.conn, client.dec, client.lost
	client.Unlock()

	done := make(chan struct{})
	cause := make(chan error, 1)

	// Stop serving at the first loss. This starts before the handshake: the
	// close ends a handshake that blocks because the server does not read.
	go func() {
		err := <-lost

		close(done)
		conn.Close()

		cause <- err
	}()

	go mux(done, dec, lost)
	heartbeat() // handshake is first heartbeat

	return func() error {
		return <-cause
	}
}

// connLost reports err as the cause of the loss of the connection that lost
// belongs to. Only the first error on a connection is kept.
func connLost(lost chan<- error, err error) {
	select {
	case lost <- err:
	default:
	}
}

// mux routes incoming messages from the server based on message type. It
// reports the error that ends the connection on lost.
func mux(done chan struct{}, dec *gob.Decoder, lost chan<- error) {
	// start piping data to minitunnel and trunking it over the ron
	local, remote := net.Pipe()
	defer local.Close()
	defer remote.Close()

	go func() {
		if err := minitunnel.ListenAndServe(local); err != nil {
			log.Error("ListenAndServe: %v", err)
		}
	}()

	tunnelStarted := false

	// Read messages from gob, mux message to the correct place
	var err error

	log.Debug("starting mux")

	for err == nil {
		var (
			m ron.Message
			d = time.Duration(math.Round(2.5 * ron.HEARTBEAT_RATE))
		)

		err = timeout(d*time.Second, func() (err error) {
			err = dec.Decode(&m)
			if err != nil {
				err = fmt.Errorf("decoding cc message: %w", err)
			}

			return
		})

		if err != nil {
			break
		}

		log.Debug("new message: %v", m.Type)

		switch m.Type {
		case ron.MESSAGE_CLIENT:
			// ACK of the handshake
			uuid := ackHandshake(m.Client)
			if !tunnelStarted {
				go ron.Trunk(remote, uuid, sendMessage)
				tunnelStarted = true
			}
			log.Debug("handshake complete")

			go periodic(done)
			go commandHandler(done)
		case ron.MESSAGE_COMMAND:
			client.commandChan <- m.Commands
		case ron.MESSAGE_FILE:
			if m.File != nil && m.File.Update {
				if m.Error != "" {
					abortUpdate()
					log.Error("miniccc auto-update: %v", m.Error)
				} else if err := handleUpdateFile(m.File); err != nil {
					log.Error("miniccc auto-update: %v", err)
				}
			} else {
				client.fileChan <- &m
			}
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

	log.Debug("mux exit: %v", err)
	connLost(lost, err)
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
			log.Debug("command handler exit")
			return
		}
	}
}
