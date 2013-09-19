// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"iomeshage"
	"meshage"
	log "minilog"
	"reflect"
	"time"
)

var (
	meshageNode           *meshage.Node
	meshageMessages       chan *meshage.Message
	meshageCommand        chan *meshage.Message
	meshageResponse       chan *meshage.Message
	meshageTimeout        time.Duration
	meshageTimeoutDefault = time.Duration(10 * time.Second)
)

func init() {
	gob.Register(cliCommand{})
	gob.Register(cliResponse{})
	gob.Register(iomeshage.IOMMessage{})
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
		case reflect.TypeOf(iomeshage.IOMMessage{}):
			iom.Messages <- m
		default:
			log.Errorln("got invalid message!")
		}
	}
}
