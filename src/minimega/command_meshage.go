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
	meshageNode     *meshage.Node
	meshageMessages chan *meshage.Message
	meshageCommand  chan *meshage.Message
	meshageResponse chan *meshage.Message
	meshageErrors   chan error
	meshageTimeout  time.Duration
)

func init() {
	gob.Register(cli_command{})
	gob.Register(cli_response{})
}

func meshageInit(host string, degree uint, port int) {
	meshageNode, meshageMessages, meshageErrors = meshage.NewNode(host, degree, port)

	meshageCommand = make(chan *meshage.Message, 1024)
	meshageResponse = make(chan *meshage.Message, 1024)

	meshageTimeout = time.Duration(10)

	go meshageMux()
	go meshageErrorHandler()
	go meshageHandler()
}

func meshageErrorHandler() {
	for {
		err := <-meshageErrors
		log.Errorln(err)
	}
}

func meshageMux() {
	for {
		m := <-meshageMessages
		switch reflect.TypeOf(m.Body) {
		case reflect.TypeOf(cli_command{}):
			meshageCommand <- m
		case reflect.TypeOf(cli_response{}):
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
			command_chan_meshage <- m.Body.(cli_command)

			//generate a response
			r := <-ack_chan_meshage
			r.TID = m.Body.(cli_command).TID
			recipient := []string{m.Source}
			err := meshageNode.Set(recipient, r)
			if err != nil {
				log.Errorln(err)
			}
		}()
	}
}

// cli commands for meshage control
func meshageDegree(c cli_command) cli_response {
	switch len(c.Args) {
	case 0:
		return cli_response{
			Response: fmt.Sprintf("%d", meshageNode.GetDegree()),
		}
	case 1:
		a, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err.Error(),
			}
		}
		meshageNode.SetDegree(uint(a))
		return cli_response{}
	default:
		return cli_response{
			Error: "mesh_degree takes zero or one argument",
		}
	}
	return cli_response{}
}

func meshageDial(c cli_command) cli_response {
	if len(c.Args) != 1 {
		return cli_response{
			Error: "mesh_dial takes one argument",
		}
	}
	err := meshageNode.Dial(c.Args[0])
	ret := cli_response{}
	if err != nil {
		ret.Error = err.Error()
	}
	return ret
}

func meshageDot(c cli_command) cli_response {
	if len(c.Args) != 1 {
		return cli_response{
			Error: "mesh_dot takes one argument",
		}
	}
	f, err := os.Create(c.Args[0])
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}

	d := meshageNode.Dot()
	f.WriteString(d)
	f.Close()
	return cli_response{}
}

func meshageStatus(c cli_command) cli_response {
	if len(c.Args) != 0 {
		return cli_response{
			Error: "mesh_status takes no arguments",
		}
	}
	mesh := meshageNode.Mesh()
	degree := meshageNode.GetDegree()
	nodes := len(mesh)
	host, err := os.Hostname()
	if err != nil {
		return cli_response{
			Error: err.Error(),
		}
	}
	clients := len(mesh[host])
	ret := fmt.Sprintf("mesh size %d\ndegree %d\nclients connected to this node: %d", nodes, degree, clients)
	return cli_response{
		Response: ret,
	}
}

func meshageList(c cli_command) cli_response {
	if len(c.Args) != 0 {
		return cli_response{
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
	return cli_response{
		Response: ret,
	}
}

func meshageHangup(c cli_command) cli_response {
	if len(c.Args) != 1 {
		return cli_response{
			Error: "mesh_hangup takes one argument",
		}
	}
	err := meshageNode.Hangup(c.Args[0])
	ret := cli_response{}
	if err != nil {
		ret.Error = err.Error()
	}
	return ret
}

func meshageMSATimeout(c cli_command) cli_response {
	switch len(c.Args) {
	case 0:
		return cli_response{
			Response: fmt.Sprintf("%d", meshageNode.GetMSATimeout()),
		}
	case 1:
		a, err := strconv.Atoi(c.Args[0])
		if err != nil {
			return cli_response{
				Error: err.Error(),
			}
		}
		meshageNode.SetMSATimeout(uint(a))
		return cli_response{}
	default:
		return cli_response{
			Error: "mesh_msa_timeout takes zero or one argument",
		}
	}
	return cli_response{}
}

func meshageSet(c cli_command) cli_response {
	if len(c.Args) < 2 {
		return cli_response{
			Error: "mesh_set takes at least two arguments",
		}
	}
	recipients := getRecipients(c.Args[0])
	command := makeCommand(strings.Join(c.Args[1:], " "))
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	TID := r.Int31()
	command.TID = TID
	err := meshageNode.Set(recipients, command)
	if err != nil {
		return cli_response{
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
			body := resp.Body.(cli_response)
			if body.TID != TID {
				log.Warn("invalid TID from response channel: %d", resp.Body.(cli_response).TID)
			} else {
				if body.Response != "" {
					respString += body.Response + "\n"
				}
				if body.Error != "" {
					respError += body.Error + "\n"
				}
				i++
			}
		case <-time.After(meshageTimeout * time.Second):
			e := fmt.Sprintf("meshage timeout: %v", command)
			log.Errorln(e)
			respError += e
			break SET_WAIT_LOOP
		}
	}
	return cli_response{
		Response: respString,
		Error:    respError,
	}
}

func meshageBroadcast(c cli_command) cli_response {
	if len(c.Args) == 0 {
		return cli_response{
			Error: "mesh_broadcast takes at least one argument",
		}
	}

	command := makeCommand(strings.Join(c.Args, " "))
	s := rand.NewSource(time.Now().UnixNano())
	r := rand.New(s)
	TID := r.Int31()
	command.TID = TID
	n, err := meshageNode.Broadcast(command)
	if err != nil {
		return cli_response{
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
			body := resp.Body.(cli_response)
			if body.TID != TID {
				log.Warn("invalid TID from response channel: %d", resp.Body.(cli_response).TID)
			} else {
				if body.Response != "" {
					respString += body.Response + "\n"
				}
				if body.Error != "" {
					respError += body.Error + "\n"
				}
				i++
			}
		case <-time.After(meshageTimeout * time.Second):
			e := fmt.Sprintf("meshage timeout: %v", command)
			log.Errorln(e)
			respError += e
			break BROADCAST_WAIT_LOOP
		}
	}
	return cli_response{
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
	log.Debug("expanded range: %v\n", ret)
	return ret
}
