// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	"meshage"
	log "minilog"
	"os"
	"ranges"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	meshageCommandLock sync.Mutex
)

func meshageHandler() {
	for {
		m := <-meshageCommand
		go func() {
			commandChanMeshage <- m.Body.(cliCommand)

			//generate a response
			r := <-ackChanMeshage
			r.TID = m.Body.(cliCommand).TID
			recipient := []string{m.Source}
			_, err := meshageNode.Set(recipient, meshage.UNORDERED, r)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}

// cli commands for meshage control
func meshageDegree(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		return cliResponse{
			Response: fmt.Sprintf("%d", meshageNode.GetDegree()),
		}
	case 1:
		a, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		meshageNode.SetDegree(uint(a))
		return cliResponse{}
	default:
		return cliResponse{
			Error: "mesh_degree takes zero or one argument",
		}
	}
	return cliResponse{}
}

func meshageDial(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "mesh_dial takes one argument",
		}
	}
	err := meshageNode.Dial(c.Args[0])
	ret := cliResponse{}
	if err != nil {
		ret.Error = err.Error()
	}
	return ret
}

func meshageDot(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "mesh_dot takes one argument",
		}
	}
	f, err := os.Create(c.Args[0])
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	d := meshageNode.Dot()
	f.WriteString(d)
	f.Close()
	return cliResponse{}
}

func meshageStatus(c cliCommand) cliResponse {
	if len(c.Args) != 0 {
		return cliResponse{
			Error: "mesh_status takes no arguments",
		}
	}
	mesh := meshageNode.Mesh()
	degree := meshageNode.GetDegree()
	nodes := len(mesh)
	host, err := os.Hostname()
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	clients := len(mesh[host])
	ret := fmt.Sprintf("mesh size %d\ndegree %d\nclients connected to this node: %d", nodes, degree, clients)
	return cliResponse{
		Response: ret,
	}
}

func meshageList(c cliCommand) cliResponse {
	if len(c.Args) != 0 {
		return cliResponse{
			Error: "mesh_list takes no arguments",
		}
	}

	mesh := meshageNode.Mesh()

	var keys []string
	for k, _ := range mesh {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var ret string
	for _, key := range keys {
		v := mesh[key]
		ret += fmt.Sprintf("%s\n", key)
		sort.Strings(v)
		for _, x := range v {
			ret += fmt.Sprintf(" |--%s\n", x)
		}
	}
	return cliResponse{
		Response: ret,
	}
}

func meshageHangup(c cliCommand) cliResponse {
	if len(c.Args) != 1 {
		return cliResponse{
			Error: "mesh_hangup takes one argument",
		}
	}
	err := meshageNode.Hangup(c.Args[0])
	ret := cliResponse{}
	if err != nil {
		ret.Error = err.Error()
	}
	return ret
}

func meshageTimeoutCLI(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		return cliResponse{
			Response: fmt.Sprintf("%v", meshageTimeout),
		}
	case 1:
		a, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		meshageTimeout = time.Duration(a) * time.Second
		return cliResponse{}
	default:
		return cliResponse{
			Error: "mesh_timeout takes zero or one argument",
		}
	}
	return cliResponse{}
}

func meshageMSATimeout(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		return cliResponse{
			Response: fmt.Sprintf("%d", meshageNode.GetMSATimeout()),
		}
	case 1:
		a, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cliResponse{
				Error: err.Error(),
			}
		}
		meshageNode.SetMSATimeout(uint(a))
		return cliResponse{}
	default:
		return cliResponse{
			Error: "mesh_msa_timeout takes zero or one argument",
		}
	}
	return cliResponse{}
}

func meshageSet(c cliCommand) cliResponse {
	meshageCommandLock.Lock()
	defer meshageCommandLock.Unlock()

	if len(c.Args) < 2 {
		return cliResponse{
			Error: "mesh_set takes at least two arguments",
		}
	}

	traversal := meshage.UNORDERED

	addHost := false
	if c.Args[0] == "annotate" {
		addHost = true
	}

	commandOffset := 1
	if addHost {
		commandOffset = 2
	}

	recipients := getRecipients(c.Args[commandOffset-1])
	command := makeCommand(strings.Join(c.Args[commandOffset:], " "))

	if command.Command == "mesh_broadcast" || command.Command == "mesh_set" {
		return cliResponse{
			Error: "compound mesh commands are not allowed",
		}
	}

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	TID := r.Int31()
	command.TID = TID
	n, err := meshageNode.Set(recipients, traversal, command)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	// wait on a response from the recipient
	var respString string
	var respError string
SET_WAIT_LOOP:
	for i := 0; i < n; {
		select {
		case resp := <-meshageResponse:
			body := resp.Body.(cliResponse)
			if body.TID != TID {
				log.Warn("invalid TID from response channel: %d", resp.Body.(cliResponse).TID)
			} else {
				if body.Response != "" {
					if addHost {
						respString += fmt.Sprintf("[%v] %v\n", resp.Source, body.Response)
					} else {
						respString += fmt.Sprintf("%v\n", body.Response)
					}
				}
				if body.Error != "" {
					respError += fmt.Sprintf("[%v] %v\n", resp.Source, body.Error)
				}
				i++
			}
		case <-time.After(meshageTimeout):
			respError += fmt.Sprintf("meshage timeout: %v", command)
			break SET_WAIT_LOOP
		}
	}
	return cliResponse{
		Response: respString,
		Error:    respError,
	}
}

func meshageBroadcast(c cliCommand) cliResponse {
	meshageCommandLock.Lock()
	defer meshageCommandLock.Unlock()

	if len(c.Args) == 0 {
		return cliResponse{
			Error: "mesh_broadcast takes at least one argument",
		}
	}

	traversal := meshage.UNORDERED

	addHost := false
	if c.Args[0] == "annotate" {
		addHost = true
	}

	commandOffset := 0
	if addHost {
		commandOffset = 1
	}

	command := makeCommand(strings.Join(c.Args[commandOffset:], " "))

	if command.Command == "mesh_broadcast" || command.Command == "mesh_set" {
		return cliResponse{
			Error: "compound mesh commands are not allowed",
		}
	}

	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	TID := r.Int31()
	command.TID = TID
	n, err := meshageNode.Broadcast(traversal, command)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	// wait on a response from the recipient
	var respString string
	var respError string
BROADCAST_WAIT_LOOP:
	for i := 0; i < n; {
		select {
		case resp := <-meshageResponse:
			body := resp.Body.(cliResponse)
			if body.TID != TID {
				log.Warn("invalid TID from response channel: %d", resp.Body.(cliResponse).TID)
			} else {
				if body.Response != "" {
					if addHost {
						respString += fmt.Sprintf("[%v] %v\n", resp.Source, body.Response)
					} else {
						respString += fmt.Sprintf("%v\n", body.Response)
					}
				}
				if body.Error != "" {
					respError += fmt.Sprintf("[%v] %v\n", resp.Source, body.Error)
				}
				i++
			}
		case <-time.After(meshageTimeout):
			respError += fmt.Sprintf("meshage timeout: %v", command)
			break BROADCAST_WAIT_LOOP
		}
	}
	return cliResponse{
		Response: respString,
		Error:    respError,
	}
}

func getRecipients(r string) []string {
	f := strings.Split(r, ",")

	// fix splits on things like kn[1-5,200,150]
	var hosts []string
	appendState := false
	for _, v := range f {
		if strings.Contains(v, "[") {
			appendState = true
			hosts = append(hosts, v)
			if !strings.Contains(v, "]") {
				hosts[len(hosts)-1] += ","
			}
			continue
		}
		if appendState == true {
			hosts[len(hosts)-1] += v
			if strings.Contains(v, "]") {
				appendState = false
			} else {
				hosts[len(hosts)-1] += ","
			}
			continue
		}
		hosts = append(hosts, v)
	}
	log.Debugln("getRecipients first pass: ", hosts)

	var hostsExpanded []string
	for _, v := range hosts {
		index := strings.IndexRune(v, '[')
		if index == -1 {
			hostsExpanded = append(hostsExpanded, v)
			continue
		}
		prefix := v[:index]
		rangeObj, _ := ranges.NewRange(prefix, 0, int(^uint(0)>>1))
		ret, _ := rangeObj.SplitRange(v)
		log.Debug("expanded range: %v", ret)
		hostsExpanded = append(hostsExpanded, ret...)
	}
	log.Debugln("getRecipients expanded pass: ", hostsExpanded)
	return hostsExpanded
}
