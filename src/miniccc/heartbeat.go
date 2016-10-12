// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"ron"
	"time"
	"version"
)

const HeartbeatRate = 5 * time.Second

// periodically send the client heartbeat.
func periodic() {
	for {
		log.Debug("periodic")

		now := time.Now()
		if now.Sub(Client.lastHeartbeat) > HeartbeatRate {
			// issue a heartbeat
			heartbeat()
		}

		sleep := HeartbeatRate - now.Sub(Client.lastHeartbeat)
		time.Sleep(sleep)
	}
}

// heartbeat sends the latest client info to the ron server.
func heartbeat() {
	Client.Lock()
	defer Client.Unlock()

	updateNetworkInfo()

	log.Debug("sending heartbeat")

	c := &ron.Client{
		UUID:           Client.UUID,
		Arch:           Client.Arch,
		OS:             Client.OS,
		Hostname:       Client.Hostname,
		IPs:            Client.IPs,
		MACs:           Client.MACs,
		CommandCounter: Client.CommandCounter,
		Version:        version.Revision,
		Processes:      make(map[int]*ron.Process),
	}

	for k, v := range Client.Processes {
		c.Processes[k] = &ron.Process{
			PID:     v.PID,
			Command: v.Command,
		}
	}

	c.Responses = Client.Responses
	Client.Responses = []*ron.Response{}
	c.Tags = Client.Tags
	Client.Tags = make(map[string]string)

	m := &ron.Message{
		Type:   ron.MESSAGE_CLIENT,
		UUID:   c.UUID,
		Client: c,
	}

	if err := sendMessage(m); err != nil {
		log.Error("heartbeat failed: %v", err)
		return
	}

	Client.lastHeartbeat = time.Now()
}
