// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var client struct {
	ron.Client // embed
	sync.Mutex // embed

	// claim is the UUID that miniccc sends until the server acknowledges the
	// handshake on a connection: the -uuid flag or the guest's own UUID. It is
	// empty on a host-bound channel, so that the host sets the UUID of the VM
	// that the channel is bound to on every connection.
	claim string
	// acked is true when the server acknowledged the handshake on the
	// current connection.
	acked bool

	Processes map[int]*Process

	lastHeartbeat time.Time

	// writeMu guards enc and lost, which dial also sets with the client lock
	// held. Take writeMu after the client lock.
	writeMu sync.Mutex

	conn io.ReadWriteCloser
	enc  *gob.Encoder
	dec  *gob.Decoder
	// lost receives the first error on conn
	lost chan error

	commandChan chan map[int]*ron.Command
	fileChan    chan *ron.Message
}

type Process struct {
	PID     int
	Command []string
	process *os.Process
}

func NewClient() {
	client.claim = resolveClientUUID(*f_uuid, getUUID)
	client.UUID = client.claim
	client.Arch = runtime.GOARCH
	client.OS = runtime.GOOS
	client.Version = version.Revision

	client.Processes = make(map[int]*Process)

	client.commandChan = make(chan map[int]*ron.Command, 1024)
	client.fileChan = make(chan *ron.Message, 1024)
}

func resolveClientUUID(configured string, discover func() string) string {
	if uuid := strings.ToLower(strings.TrimSpace(configured)); uuid != "" {
		return uuid
	}

	return discover()
}

// ackHandshake records that the server acknowledged the handshake on the
// current connection and returns the client UUID. A client that claimed no
// UUID takes the one in the ack. That UUID can change between connections,
// for example when a VM snapshot is restored into another VM.
func ackHandshake(ack *ron.Client) string {
	client.Lock()
	defer client.Unlock()

	client.acked = true

	if client.claim == "" && ack != nil && ack.UUID != "" && ack.UUID != client.UUID {
		if client.UUID == "" {
			log.Debug("adopted serial-bound UUID: %v", ack.UUID)
		} else {
			log.Info("cc channel now bound to %v", ack.UUID)
		}
		client.UUID = ack.UUID
	}

	return client.UUID
}

// clientUUID returns the client UUID.
func clientUUID() string {
	client.Lock()
	defer client.Unlock()

	return client.UUID
}

// sendMessage sends m on the current connection. An error ends the
// connection, so that miniccc connects again.
func sendMessage(m *ron.Message) error {
	client.writeMu.Lock()
	defer client.writeMu.Unlock()

	if client.enc == nil {
		return errNotConnected
	}

	err := client.enc.Encode(m)
	if err != nil {
		connLost(client.lost, fmt.Errorf("sending cc message: %w", err))
	}

	return err
}

// appendResponse allows a client to post a *Response to a given command. The
// response will be queued until the next heartbeat.
func appendResponse(r *ron.Response) {
	log.Debug("response: %v", r.ID)

	client.Lock()
	defer client.Unlock()

	client.LastCommandID = r.ID
	client.Responses = append(client.Responses, r)
}

func addTag(k, v string) {
	log.Debug("tag: %v %v", k, v)

	client.Lock()
	defer client.Unlock()

	if client.Tags == nil {
		client.Tags = make(map[string]string)
	}
	client.Tags[k] = v
}

// updateNetworkInfo updates the hostname, IPs, and MACs for the client.
// Assumes that the client lock is held.
func updateNetworkInfo() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Error("unable to get hostname: %v", err)
	}

	client.Hostname = hostname
	client.IPs = nil
	client.MACs = nil

	nics, err := net.Interfaces()
	if err != nil {
		log.Error("unable to get interfaces: %v", err)
	}

	for _, nic := range nics {
		if nic.HardwareAddr.String() == "" {
			// skip localhost and other weird interfaces
			continue
		}

		log.Debug("found mac: %v", nic.HardwareAddr)
		client.MACs = append(client.MACs, nic.HardwareAddr.String())

		addrs, err := nic.Addrs()
		if err != nil {
			log.Error("unable to get addrs for %v: %v", nic.HardwareAddr, err)
		}

		for _, addr := range addrs {
			switch addr := addr.(type) {
			case *net.IPNet:
				client.IPs = append(client.IPs, addr.IP.String())
			default:
				log.Debug("unknown network type: %v", addr)
			}
		}
	}
}
