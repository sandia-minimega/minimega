// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"fmt"
	"iomeshage"
	"math"
	"math/rand"
	"meshage"
	"minicli"
	log "minilog"
	"ranges"
	"reflect"
	"time"
	"version"
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
	meshageTimeout      time.Duration // default is no timeout
)

func init() {
	gob.Register(meshageCommand{})
	gob.Register(meshageResponse{})
	gob.Register(iomeshage.IOMMessage{})
}

func meshageInit(host string, namespace string, degree, msaTimeout uint, port int) {
	meshageNode, meshageMessages = meshage.NewNode(host, namespace, degree, port, version.Revision)

	meshageCommandChan = make(chan *meshage.Message, 1024)
	meshageResponseChan = make(chan *meshage.Message, 1024)

	meshageNode.Snoop = meshageSnooper

	meshageNode.SetMSATimeout(msaTimeout)

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

// meshageRecipients expands a hosts into a list of hostnames. Supports
// expanding Wildcard to all hosts in the mesh or all hosts in the active
// namespace.
func meshageRecipients(hosts string) ([]string, error) {
	if hosts == Wildcard {
		if namespace == "" {
			return meshageNode.BroadcastRecipients(), nil
		}

		recipients := []string{}

		// Wildcard expands to all hosts in the namespace, except the local
		// host, if included
		for host := range namespaces[namespace].Hosts {
			if host == hostname {
				log.Info("excluding localhost, %v, from `%v`", hostname, Wildcard)
				continue
			}

			recipients = append(recipients, host)
		}

		return recipients, nil
	}

	recipients, err := ranges.SplitList(hosts)
	if err != nil {
		return nil, err
	}

	// If a namespace is active, warn if the user is trying to mesh send hosts
	// outside the namespace
	if namespace != "" {
		for _, host := range recipients {
			if !namespaces[namespace].Hosts[host] {
				log.Warn("%v is not part of namespace %v", host, namespace)
			}
		}
	}

	return recipients, nil
}

// meshageSend sends a command to a list of hosts, returning a channel that the
// responses will be sent to. This is non-blocking -- the channel is created
// and then returned after a couple of sanity checks. Should be not be invoked
// as a goroutine as it uses the global namespace variable to expand the hosts.
func meshageSend(c *minicli.Command, hosts string) (chan minicli.Responses, error) {
	// HAX: Ensure we aren't sending read or mesh send commands over meshage
	if hasCommand(c, "read") || hasCommand(c, "mesh send") {
		return nil, fmt.Errorf("cannot run `%s` over mesh", c.Original)
	}

	// expand the hosts to a list of recipients, must be done synchronously
	recipients, err := meshageRecipients(hosts)
	if err != nil {
		return nil, err
	}

	meshageCommandLock.Lock()
	out := make(chan minicli.Responses)

	// Build a mesh command from the command, assigning a random ID
	meshageID := rand.Int31()
	meshageCmd := meshageCommand{Command: *c, TID: meshageID}

	go func() {
		defer meshageCommandLock.Unlock()
		defer close(out)

		recipients, err = meshageNode.Set(recipients, meshageCmd)
		if err != nil {
			out <- errResp(err)
			return
		}

		log.Debug("meshage sent, waiting on %d responses", len(recipients))

		// host -> response
		resps := map[string]*minicli.Response{}

		timeout := meshageTimeout
		// If the timeout is 0, set to "unlimited"
		if timeout == 0 {
			timeout = math.MaxInt64
		}

		// wait on a response from each recipient
	recvLoop:
		for len(resps) < len(recipients) {
			select {
			case resp := <-meshageResponseChan:
				body := resp.Body.(meshageResponse)
				if body.TID != meshageID {
					log.Warn("invalid TID from response channel: %d", body.TID)
				} else {
					resps[body.Host] = &body.Response
				}
			case <-time.After(timeout):
				// Didn't hear back from any node within the timeout
				break recvLoop
			}
		}

		// Fill in the responses for recipients that timed out
		resp := minicli.Responses{}
		for _, host := range recipients {
			if v, ok := resps[host]; ok {
				resp = append(resp, v)
			} else if host != hostname {
				resp = append(resp, &minicli.Response{
					Host:  host,
					Error: "timed out",
				})
			}
		}

		out <- resp
	}()

	return out, nil
}
