// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"time"
	"vnc"
)

type vncClient struct {
	VM    *KvmVM
	ID    string
	Rhost string

	err error

	Conn *vnc.Conn
	file *os.File

	start time.Time
	done  chan bool
}

// NewVNCClient creates a new VNC client. NewVNCClient is only called via the
// cli so we can assume that cmdLock is held.
// This is sent via wrapVMTargetCLI so we assume the command will always be
// delivered to the correct host
func NewVNCClient(vm *KvmVM) (*vncClient, error) {
	rhost := fmt.Sprintf("%v:%v", hostname, vm.VNCPort)
	id := fmt.Sprintf("%v:%v", vm.Namespace, vm.Name)

	c := &vncClient{
		ID:    id, // ID is namespace:name
		Rhost: rhost,
		start: time.Now(),
		VM:    vm,
		done:  make(chan bool),
	}

	return c, nil
}

func (v *vncClient) Matches(host, vm string) bool {
	return v.VM.Host == host && v.VM.Name == vm
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

func vncClear() {
	for k, v := range vncKBRecording {
		if inNamespace(v.VM) {
			log.Debug("stopping kb recording for %v", k)
			if err := v.Stop(); err != nil {
				log.Error("%v", err)
			}

			delete(vncKBRecording, k)
		}
	}

	for k, v := range vncFBRecording {
		if inNamespace(v.VM) {
			log.Debug("stopping fb recording for %v", k)
			if err := v.Stop(); err != nil {
				log.Error("%v", err)
			}

			delete(vncFBRecording, k)
		}
	}

	for k, v := range vncKBPlaying {
		if inNamespace(v.VM) {
			log.Debug("stopping kb playing for %v", k)
			if err := v.Stop(); err != nil {
				log.Error("%v", err)
			}

			delete(vncKBPlaying, k)
		}
	}
}
