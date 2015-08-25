// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"fmt"
	"iomeshage"
	"math/rand"
	"meshage"
	"minicli"
	log "minilog"
	"ranges"
	"reflect"
	"time"
)

const (
	MESH_TIMEOUT_DEFAULT = 10
)

type meshageCommand struct {
	minicli.Command       // embed a minicli command
	TID             int32 // unique ID for command/response pair
}

type meshageResponse struct {
	minicli.Response       // embed a minicli response
	TID              int32 // unique ID for command/response pair
}

var (
	meshageNode         *meshage.Node
	meshageMessages     chan *meshage.Message
	meshageCommandChan  chan *meshage.Message
	meshageResponseChan chan *meshage.Message
	meshageTimeout      time.Duration
)

func init() {
	gob.Register(meshageCommand{})
	gob.Register(meshageResponse{})
	gob.Register(iomeshage.IOMMessage{})
}

func meshageInit(host string, namespace string, degree uint, port int) {
	meshageNode, meshageMessages = meshage.NewNode(host, namespace, degree, port)

	meshageCommandChan = make(chan *meshage.Message, 1024)
	meshageResponseChan = make(chan *meshage.Message, 1024)

	meshageTimeout = time.Duration(MESH_TIMEOUT_DEFAULT) * time.Second

	meshageNode.Snoop = meshageSnooper

	meshageNode.SetMSATimeout(uint(*f_msaTimeout))

	go meshageMux()
	go meshageHandler()

	iomeshageInit(meshageNode)

	// wait a bit to let things settle
	time.Sleep(500 * time.Millisecond)
}

func meshageMux() {
	for {
		m := <-meshageMessages
		switch m.Body.(type) {
		case meshageCommand:
			meshageCommandChan <- m
		case meshageResponse:
			meshageResponseChan <- m
		case iomeshage.IOMMessage:
			iom.Messages <- m
		default:
			log.Errorln("got invalid message!")
		}
	}
}

func meshageSnooper(m *meshage.Message) {
	if reflect.TypeOf(m.Body) == reflect.TypeOf(iomeshage.IOMMessage{}) {
		i := m.Body.(iomeshage.IOMMessage)
		iom.MITM(&i)
	}
}

func meshageBroadcast(c *minicli.Command, respChan chan minicli.Responses) {
	meshageSend(c, Wildcard, respChan)
}

func meshageSend(c *minicli.Command, hosts string, respChan chan minicli.Responses) {
	var (
		err        error
		recipients []string
	)

	meshageCommandLock.Lock()
	defer meshageCommandLock.Unlock()

	orig := c.Original

	// HAX: Ensure we aren't sending read or mesh send commands over meshage
	if hasCommand(c, "read") || hasCommand(c, "mesh send") {
		resp := &minicli.Response{
			Host:  hostname,
			Error: fmt.Sprintf("cannot run `%s` over mesh", orig),
		}
		respChan <- minicli.Responses{resp}
		return
	}

	meshageID := rand.Int31()
	// Build a mesh command from the subcommand, assigning a random ID
	meshageCmd := meshageCommand{Command: *c, TID: meshageID}

	if hosts == Wildcard {
		// Broadcast command to all hosts
		recipients = meshageNode.BroadcastRecipients()
	} else {
		// Send to specified list of recipients
		recipients, err = ranges.SplitList(hosts)
	}

	if err == nil {
		recipients, err = meshageNode.Set(recipients, meshageCmd)
	}

	if err != nil {
		resp := &minicli.Response{
			Host:  hostname,
			Error: err.Error(),
		}
		respChan <- minicli.Responses{resp}
		return
	}

	log.Debug("meshage sent, waiting on %d responses", len(recipients))
	meshResps := map[string]*minicli.Response{}

	// wait on a response from each recipient
loop:
	for len(meshResps) < len(recipients) {
		select {
		case resp := <-meshageResponseChan:
			body := resp.Body.(meshageResponse)
			if body.TID != meshageID {
				log.Warn("invalid TID from response channel: %d", body.TID)
			} else {
				meshResps[body.Host] = &body.Response
			}
		case <-time.After(meshageTimeout):
			// Didn't hear back from any node within the timeout
			log.Info("meshage send timed out")
			break loop
		}
	}

	// Fill in the responses for recipients that timed out
	resp := minicli.Responses{}
	for _, host := range recipients {
		if v, ok := meshResps[host]; ok {
			resp = append(resp, v)
		} else if host != hostname {
			resp = append(resp, &minicli.Response{
				Host:  host,
				Error: "timed out",
			})
		}
	}

	respChan <- resp
	return
}
