// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"strconv"
	"time"
	"vnc"
)

type vncClient struct {
	Host  string
	Name  string
	ID    int
	Rhost string

	err error

	Conn *vnc.Conn
	file *os.File

	start time.Time
	done  chan bool
}

func NewVNCClient(host, idOrName string) (*vncClient, error) {
	// Resolve localhost to the actual hostname
	if host == Localhost {
		host = hostname
	}

	vm := findRemoteVM(host, idOrName)
	if vm == nil {
		return nil, vmNotFound(host + ":" + idOrName)
	}

	rhost := fmt.Sprintf("%v:%v", host, 5900+vm.GetID())

	c := &vncClient{
		Rhost: rhost,
		Host:  host,
		Name:  vm.GetName(),
		ID:    vm.GetID(),
		start: time.Now(),
		done:  make(chan bool),
	}

	return c, nil
}

func (v *vncClient) Matches(host, vm string) bool {
	return v.Host == host && (v.Name == vm || strconv.Itoa(v.ID) == vm)
}

func (v *vncClient) Stop() error {

	if v.file != nil {
		v.file.Close()
	}

	if v.Conn != nil {
		v.Conn.Close()
	}

	return v.err
}

func vncClear() error {
	for k, v := range vncKBRecording {
		log.Debug("stopping kb recording for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncKBRecording, k)
	}

	for k, v := range vncFBRecording {
		log.Debug("stopping fb recording for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncFBRecording, k)
	}

	for k, v := range vncKBPlaying {
		log.Debug("stopping kb playing for %v", k)
		if err := v.Stop(); err != nil {
			log.Error("%v", err)
		}

		delete(vncKBPlaying, k)
	}

	return nil
}
