// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/ron"
	"github.com/sandia-minimega/minimega/v2/internal/version"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

const HeartbeatRate = 5 * time.Second

// periodically send the client heartbeat.
func periodic(done chan struct{}) {
	for {
		t := time.NewTimer(HeartbeatRate)

		select {
		case <-t.C:
			log.Debug("periodic")

			now := time.Now()
			if now.Sub(client.lastHeartbeat) > HeartbeatRate {
				// issue a heartbeat
				heartbeat()
			}

			sleep := HeartbeatRate - now.Sub(client.lastHeartbeat)
			// time.Sleep(sleep)
			t.Reset(sleep)
		case <-done:
			if !t.Stop() {
				<-t.C
			}

			log.Debug("stopping periodic heartbeat")

			return
		}
	}
}

// heartbeat sends the latest client info to the ron server.
func heartbeat() {
	client.Lock()
	defer client.Unlock()

	updateNetworkInfo()

	log.Debug("sending heartbeat")

	c := &ron.Client{
		UUID:          client.UUID,
		Arch:          client.Arch,
		OS:            client.OS,
		Hostname:      client.Hostname,
		IPs:           client.IPs,
		MACs:          client.MACs,
		LastCommandID: client.LastCommandID,
		Version:       version.Revision,
		Processes:     make(map[int]*ron.Process),
	}

	for k, v := range client.Processes {
		c.Processes[k] = &ron.Process{
			PID:     v.PID,
			Command: v.Command,
		}
	}

	c.Responses = client.Responses
	client.Responses = []*ron.Response{}
	c.Tags = client.Tags
	client.Tags = make(map[string]string)

	m := &ron.Message{
		Type:    ron.MESSAGE_CLIENT,
		UUID:    c.UUID,
		Client:  c,
		Version: "v1",
	}

	if err := sendMessage(m); err != nil {
		log.Error("heartbeat failed: %v", err)
		return
	}

	client.lastHeartbeat = time.Now()
}
