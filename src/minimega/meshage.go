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
	"miniplumber"
	"ranges"
	"reflect"
	"strings"
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

// meshageVMLaunch is sent by the scheduler to launch VMs on a remote host
type meshageVMLaunch struct {
	Namespace string
	QueuedVMs       // embed
	TID       int32 // unique ID for command/response pair
}

// meshageVMResponse is sent back to the scheduler to notify it of any errors
// that occured
type meshageVMResponse struct {
	Errors []string // Errors from launch, can't actualy encode error type
	TID    int32    // unique ID for command/response pair
}

var (
	meshageNode     *meshage.Node
	meshageMessages chan *meshage.Message

	meshageCommandChan    = make(chan *meshage.Message, 1024)
	meshageResponseChan   = make(chan *meshage.Message, 1024)
	meshageVMLaunchChan   = make(chan *meshage.Message, 1024)
	meshageVMResponseChan = make(chan *meshage.Message, 1024)

	meshageTimeout = time.Duration(math.MaxInt64) // default is no timeout
)

func init() {
	gob.Register(meshageCommand{})
	gob.Register(meshageResponse{})
	gob.Register(meshageVMLaunch{})
	gob.Register(meshageVMResponse{})
	gob.Register(iomeshage.IOMMessage{})
	gob.Register(miniplumber.Message{})
}

func meshageStart(host string, namespace string, degree, msaTimeout uint, port int) {
	meshageNode, meshageMessages = meshage.NewNode(host, namespace, degree, port, version.Revision)

	meshageNode.Snoop = meshageSnooper

	meshageNode.SetMSATimeout(msaTimeout)

	go meshageMux()
	go meshageHandler()
	go meshageVMLauncher()

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
		case meshageVMLaunch:
			meshageVMLaunchChan <- m
		case meshageVMResponse:
			meshageVMResponseChan <- m
		case iomeshage.IOMMessage:
			iom.Messages <- m
		case miniplumber.Message:
			plumber.Messages <- m
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
	ns := GetNamespace()

	if hosts == Wildcard {
		if ns == nil {
			return meshageNode.BroadcastRecipients(), nil
		}

		recipients := []string{}

		// Wildcard expands to all hosts in the namespace, except the local
		// host, if included
		for host := range ns.Hosts {
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
	if ns != nil {
		for _, host := range recipients {
			if !ns.Hosts[host] {
				log.Warn("%v is not part of namespace %v", host, ns.Name)
			}
		}
	}

	return recipients, nil
}

// meshageSend sends a command to a list of hosts, returning a channel that the
// responses will be sent to. This is non-blocking -- the channel is created
// and then returned after a couple of sanity checks. Should be not be invoked
// as a goroutine as it checks the active namespace when expanding hosts.
func meshageSend(c *minicli.Command, hosts string) (<-chan minicli.Responses, error) {
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
			case <-time.After(meshageTimeout):
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

// meshageLaunch sends a command to a launch VMs on the specified hosts,
// returning a channel for the responses. This is non-blocking -- the channel
// is created and then returned after a couple of sanity checks.
func meshageLaunch(host, namespace string, queued QueuedVMs) <-chan minicli.Responses {
	out := make(chan minicli.Responses)

	to := []string{host}
	msg := meshageVMLaunch{
		Namespace: namespace,
		QueuedVMs: queued,
		TID:       rand.Int31(),
	}

	go func() {
		defer close(out)

		if _, err := meshageNode.Set(to, msg); err != nil {
			out <- errResp(err)
			return
		}

		log.Info("VM schedule sent to %v, waiting on response", host)

		for {
			// wait on a response from the client
			select {
			case resp := <-meshageVMResponseChan:
				body := resp.Body.(meshageVMResponse)
				if body.TID != msg.TID {
					// put it back for another goroutine to pick up...
					meshageVMResponseChan <- resp
				} else {
					// wrap response up into a minicli.Response
					resp := &minicli.Response{
						Host:  host,
						Error: strings.Join(body.Errors, "\n"),
					}

					out <- minicli.Responses{resp}
					return
				}
			case <-time.After(meshageTimeout):
				// Didn't hear back from any node within the timeout
				log.Error("timed out waiting for %v to launch VMs", host)
				return
			}
		}
	}()

	return out
}
