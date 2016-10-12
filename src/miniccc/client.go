// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"io"
	log "minilog"
	"net"
	"os"
	"ron"
	"runtime"
	"sync"
	"time"
	"version"
)

var Client struct {
	ron.Client // embed
	sync.Mutex // embed

	Processes map[int]*Process

	Namespace string // populated via handshake

	//Tags map[string]string // populated via heartbeat

	lastHeartbeat time.Time

	writeMu sync.Mutex

	conn io.ReadWriteCloser
	enc  *gob.Encoder
	dec  *gob.Decoder

	commandChan chan map[int]*ron.Command
	fileChan    chan *ron.Message
}

type Process struct {
	PID     int
	Command []string
	process *os.Process
}

// init client fields
func init() {
	Client.UUID = getUUID()
	Client.Arch = runtime.GOARCH
	Client.OS = runtime.GOOS
	Client.Version = version.Revision

	Client.Processes = make(map[int]*Process)

	Client.commandChan = make(chan map[int]*ron.Command)
	Client.fileChan = make(chan *ron.Message)
}

func sendMessage(m *ron.Message) error {
	Client.writeMu.Lock()
	defer Client.writeMu.Unlock()

	return Client.enc.Encode(m)
}

// appendResponse allows a client to post a *Response to a given command. The
// response will be queued until the next heartbeat.
func appendResponse(r *ron.Response) {
	log.Debug("response: %v", r.ID)

	Client.Lock()
	defer Client.Unlock()

	Client.CommandCounter = r.ID
	Client.Responses = append(Client.Responses, r)
}

func addTag(k, v string) {
	log.Debug("tag: %v %v", k, v)

	Client.Lock()
	defer Client.Unlock()

	Client.Tags[k] = v
}

// updateNetworkInfo updates the hostname, IPs, and MACs for the Client.
// Assumes that the Client lock is held.
func updateNetworkInfo() {
	hostname, err := os.Hostname()
	if err != nil {
		log.Error("unable to get hostname: %v", err)
	}

	Client.Hostname = hostname
	Client.IPs = nil
	Client.MACs = nil

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
		Client.MACs = append(Client.MACs, nic.HardwareAddr.String())

		addrs, err := nic.Addrs()
		if err != nil {
			log.Error("unable to get addrs for %v: %v", nic.HardwareAddr, err)
		}

		for _, addr := range addrs {
			switch addr := addr.(type) {
			case *net.IPNet:
				Client.IPs = append(Client.IPs, addr.IP.String())
			default:
				log.Debug("unknown network type: %v", addr)
			}
		}
	}
}
