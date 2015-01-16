// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	"minicli"
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

var meshageCLIHandlers = []minicli.Handler{
	{ // mesh degree
		HelpShort: "view or set the current degree for this mesh node",
		Patterns: []string{
			"mesh degree [degree]",
			"clear mesh degree",
		},
		Call: wrapSimpleCLI(cliMeshageDegree),
	},
	{ // mesh dial
		HelpShort: "attempt to connect this node to another node",
		Patterns: []string{
			"mesh dial <hostname>",
		},
		Call: wrapSimpleCLI(cliMeshageDial),
	},
	{ // mesh dot
		HelpShort: "output a graphviz formatted dot file",
		HelpLong: `
Output a graphviz formatted dot file representing the connected topology.`,
		Patterns: []string{
			"mesh dot <filename>",
		},
		Call: wrapSimpleCLI(cliMeshageDot),
	},
	{ // mesh hangup
		HelpShort: "disconnect from a client",
		Patterns: []string{
			"mesh hangup <hostname>",
		},
		Call: wrapSimpleCLI(cliMeshageHangup),
	},
	{ // mesh list
		HelpShort: "display the mesh adjacency list",
		Patterns: []string{
			"mesh list",
		},
		Call: wrapSimpleCLI(cliMeshageList),
	},
	{ // mesh status
		HelpShort: "display a short status report of the mesh",
		Patterns: []string{
			"mesh status",
		},
		Call: wrapSimpleCLI(cliMeshageStatus),
	},
	{ // mesh timeout
		HelpShort: "view or set the mesh timeout",
		HelpLong: `
View or set the timeout on sending mesh commands.

When a mesh command is issued, if a response isn't sent within mesh_timeout
seconds, the command will be dropped and any future response will be discarded.
Note that this does not cancel the outstanding command - the node receiving the
command may still complete - but rather this node will stop waiting on a
response.`,
		Patterns: []string{
			"mesh timeout [timeout]",
		},
		Call: wrapSimpleCLI(cliMeshageTimeout),
	},
	{ // mesh set
		HelpShort: "send a command to one or more connected clients",
		HelpLong: `
Send a command to one or more connected clients. For example, to get the
vm info from nodes kn1 and kn2:

	mesh_set kn[1-2] vm info

You can use * to send a command to all connected clients.`,
		Patterns: []string{
			"mesh set <vms or *> (command)",
		},
		Call: nil, // TODO: meshageSet,
	},
}

func init() {
	registerHandlers("meshage", meshageCLIHandlers)
}

func meshageHandler() {
	for {
		m := <-meshageCommand
		go func() {
			commandChanMeshage <- m.Body.(cliCommand)

			//generate a response
			var bufError string
			var bufResponse string
			var ret cliResponse
			for {
				r := <-ackChanMeshage
				if r.Error != "" {
					if bufError == "" {
						bufError = r.Error
					} else {
						if !strings.HasSuffix(bufError, "\n") {
							bufError += "\n"
						}
						bufError += r.Error
					}
				}
				if r.Response != "" {
					if bufResponse == "" {
						bufResponse = r.Response
					} else {
						if !strings.HasSuffix(bufResponse, "\n") {
							bufResponse += "\n"
						}
						bufResponse += r.Response
					}
				}
				if !r.More {
					log.Debugln("got last message")
					break
				} else {
					log.Debugln("expecting more data")
				}
			}
			ret.TID = m.Body.(cliCommand).TID
			ret.Error = bufError
			ret.Response = bufResponse
			recipient := []string{m.Source}
			_, err := meshageNode.Set(recipient, ret)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}

// cli commands for meshage control
func cliMeshageDegree(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if isClearCommand(c) {
		meshageNode.SetDegree(0)
	} else if c.StringArgs["degree"] != "" {
		degree, err := strconv.ParseUint(c.StringArgs["degree"], 0, 10)
		if err != nil {
			resp.Error = err.Error()
		} else {
			meshageNode.SetDegree(uint(degree))
		}
	} else {
		resp.Response = fmt.Sprintf("%d", meshageNode.GetDegree())
	}

	return resp
}

func cliMeshageDial(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := meshageNode.Dial(c.StringArgs["hostname"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliMeshageDot(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	f, err := os.Create(c.StringArgs["filename"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer f.Close()

	f.WriteString(meshageNode.Dot())

	return resp
}

func cliMeshageHangup(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := meshageNode.Hangup(c.StringArgs["hostname"])
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}

func cliMeshageList(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	mesh := meshageNode.Mesh()

	var keys []string
	for k, _ := range mesh {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		v := mesh[key]
		resp.Response += fmt.Sprintf("%s\n", key)
		sort.Strings(v)
		for _, x := range v {
			resp.Response += fmt.Sprintf(" |--%s\n", x)
		}
	}

	return resp
}

func cliMeshageStatus(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	mesh := meshageNode.Mesh()
	degree := meshageNode.GetDegree()
	nodes := len(mesh)

	resp.Header = []string{"mesh size", "degree", "peers"}
	resp.Tabular = [][]string{
		[]string{
			strconv.Itoa(nodes),
			strconv.FormatUint(uint64(degree), 10),
			strconv.Itoa(len(mesh[hostname])),
		},
	}

	return resp
}

func cliMeshageTimeout(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	if isClearCommand(c) {
		meshageTimeout = time.Duration(MESH_TIMEOUT_DEFAULT) * time.Second
	} else if c.StringArgs["timeout"] != "" {
		timeout, err := strconv.Atoi(c.StringArgs["timeout"])
		if err != nil {
			resp.Error = err.Error()
		} else {
			meshageTimeout = time.Duration(timeout) * time.Second
		}
	} else {
		resp.Response = fmt.Sprintf("%v", meshageTimeout)
	}

	return resp
}

func meshageMSATimeout(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 0:
		return cliResponse{
			Response: fmt.Sprintf("%v", meshageNode.GetMSATimeout()),
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
	n, err := meshageNode.Set(recipients, command)
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

func meshageBroadcastTwo(c *minicli.Command, hosts string) chan minicli.Responses {
	// TODO
	return nil
}

func meshageBroadcast(c cliCommand) cliResponse {
	meshageCommandLock.Lock()
	defer meshageCommandLock.Unlock()

	if len(c.Args) == 0 {
		return cliResponse{
			Error: "mesh_broadcast takes at least one argument",
		}
	}

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
	n, err := meshageNode.Broadcast(command)
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
