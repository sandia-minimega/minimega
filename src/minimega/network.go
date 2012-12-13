package main

import (
	"meshage"
	log "minilog"
)

var (
	meshageNode     *meshage.Node
	meshageMessages chan *meshage.Message
	meshageErrors   chan error
)

func meshageInit(host string, degree uint, port int) {
	meshageNode, meshageMessages, meshageErrors = meshage.NewNode(host, degree, port)

	go meshageErrorHandler()
}

func meshageErrorHandler() {
	for {
		err := <-meshageErrors
		log.Errorln(err)
	}
}
