// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"fmt"
	"math/rand"
	"meshage"
	log "minilog"
	"os"
	"ranges"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	meshageNode           *meshage.Node
	meshageMessages       chan *meshage.Message
	meshageCommand        chan *meshage.Message
	meshageResponse       chan *meshage.Message
	meshageLog            bool
	meshageTimeout        time.Duration
	meshageTimeoutDefault = time.Duration(10 * time.Second)
)

func init() {
	gob.Register(cliCommand{})
	gob.Register(cliResponse{})
}

func meshageInit(host string, namespace string, degree uint, port int) {
	meshageNode, meshageMessages = meshage.NewNode(host, namespace, degree, port)

	meshageCommand = make(chan *meshage.Message, 1024)
	meshageResponse = make(chan *meshage.Message, 1024)

	meshageTimeout = time.Duration(10 * time.Second)

	go meshageMux()
	go meshageHandler()

	iomeshageInit(meshageNode)

	// wait a bit to let things settle
	time.Sleep(500 * time.Millisecond)
}

func meshageMux() {
	for {
		m := <-meshageMessages
		switch reflect.TypeOf(m.Body) {
		case reflect.TypeOf(cliCommand{}):
			meshageCommand <- m
		case reflect.TypeOf(cliResponse{}):
			meshageResponse <- m
		default:
			log.Errorln("got invalid message!")
		}
	}
}

func meshageHandler() {
	for {
		m := <-meshageCommand
		go func() {
			commandChanMeshage <- m.Body.(cliCommand)

			//generate a response
			r := <-ackChanMeshage
			r.TID = m.Body.(cliCommand).TID
			recipient := []string{m.Source}
			err := meshageNode.Set(recipient, meshage.UNORDERED, r)
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

// Parse the first argument of the command and return the traveral type and true.
// If the first argument is not a listed traveral type, return meshage.UNORDERED and false.
// The boolean return is used to truncate the field from the passed message.
func meshageTraversal(t string) (int, bool) {
	switch strings.ToLower(t) {
	case "unordered":
		return meshage.UNORDERED, true
	case "depth":
		return meshage.DEPTH, true
	case "breadth":
		return meshage.BREADTH, true
	}
	return meshage.UNORDERED, false
}

func meshageSet(c cliCommand) cliResponse {
	if len(c.Args) < 2 {
		return cliResponse{
			Error: "mesh_set takes at least two arguments",
		}
	}

	traversal, truncate := meshageTraversal(c.Args[1])
	commandOffset := 1
	if truncate {
		commandOffset = 2
	}

	recipients := getRecipients(c.Args[0])
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
	err := meshageNode.Set(recipients, traversal, command)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	// wait on a response from the recipient
	var respString string
	var respError string
SET_WAIT_LOOP:
	for i := 0; i < len(recipients); {
		select {
		case resp := <-meshageResponse:
			body := resp.Body.(cliResponse)
			if body.TID != TID {
				log.Warn("invalid TID from response channel: %d", resp.Body.(cliResponse).TID)
			} else {
				if body.Response != "" {
					respString += body.Response + "\n"
				}
				if body.Error != "" {
					respError += body.Error + "\n"
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
	if len(c.Args) == 0 {
		return cliResponse{
			Error: "mesh_broadcast takes at least one argument",
		}
	}

	traversal, truncate := meshageTraversal(c.Args[0])
	commandOffset := 0
	if truncate {
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
					respString += body.Response + "\n"
				}
				if body.Error != "" {
					respError += body.Error + "\n"
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
	index := strings.IndexRune(r, '[')
	if index == -1 {
		return []string{r}
	}
	prefix := r[:index]
	rangeObj, _ := ranges.NewRange(prefix, 0, int(^uint(0)>>1))
	ret, _ := rangeObj.SplitRange(r)
	log.Debug("expanded range: %v", ret)
	return ret
}
