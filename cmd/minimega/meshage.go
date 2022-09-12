// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/iomeshage"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/internal/miniplumber"
	"github.com/sandia-minimega/minimega/v2/internal/version"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
	"github.com/sandia-minimega/minimega/v2/pkg/ranges"
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
	Namespace  string
	*QueuedVMs       // embed
	TID        int32 // unique ID for command/response pair
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
	gob.Register(iomeshage.Message{})
	gob.Register(miniplumber.Message{})
	gob.Register(meshageLogMessage{})
}

func meshageStart(host, namespace string, degree, msaTimeout uint, broadcastIP string, port int) error {
	bip := net.ParseIP(broadcastIP)
	if bip == nil {
		return fmt.Errorf("invalid broadcast IP %s for meshage", broadcastIP)
	}

	meshageNode, meshageMessages = meshage.NewNode(host, namespace, degree, bip, port, version.Revision)

	meshageNode.Snoop = meshageSnooper

	meshageNode.SetMSATimeout(msaTimeout)

	go meshageMux()
	go meshageHandler()
	go meshageVMLauncher()

	return iomeshageStart(meshageNode)
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
		case iomeshage.Message:
			iom.Messages <- m
		case miniplumber.Message:
			plumber.Messages <- m
		case meshageLogMessage:
			msg := m.Body.(meshageLogMessage)

			// The mesh logger will never send debug logs, so don't take them into
			// account here.

			switch msg.Level {
			case log.INFO:
				log.Info("[node: %s] %s", msg.From, msg.Log)
			case log.WARN:
				log.Warn("[node: %s] %s", msg.From, msg.Log)
			case log.ERROR:
				log.Error("[node: %s] %s", msg.From, msg.Log)
			case log.FATAL:
				// don't let a fatal log on another node kill this node
				log.Error("[node: %s] %s", msg.From, msg.Log)
			}
		default:
			log.Errorln("got invalid message!")
		}
	}
}

func meshageSnooper(m *meshage.Message) {
	if reflect.TypeOf(m.Body) == reflect.TypeOf(iomeshage.Message{}) {
		i := m.Body.(iomeshage.Message)
		iom.MITM(&i)
	}
}

// meshageSend sends a command to a list of hosts, returning a channel that the
// responses will be sent to. This is non-blocking -- the channel is created
// and then returned after a couple of sanity checks.
func meshageSend(c *minicli.Command, hosts string) (<-chan minicli.Responses, error) {
	recipients, err := ranges.SplitList(hosts)
	if err != nil {
		return nil, err
	}

	for _, r := range recipients {
		if r == Wildcard {
			if len(recipients) > 1 {
				return nil, errors.New("wildcard included amongst list of recipients")
			}

			recipients = meshageNode.BroadcastRecipients()
			break
		}
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
func meshageLaunch(host, namespace string, queued *QueuedVMs) <-chan minicli.Responses {
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
						Host:     host,
						Response: strconv.Itoa(len(body.Errors)),
						Error:    strings.Join(body.Errors, "\n"),
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
