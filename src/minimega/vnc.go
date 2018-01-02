// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
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

// NewVNCClient creates a new VNC client for the target VM.
func NewVNCClient(vm *KvmVM) *vncClient {
	rhost := fmt.Sprintf("%v:%v", hostname, vm.VNCPort)

	return &vncClient{
		ID:    vm.Name,
		Rhost: rhost,
		start: time.Now(),
		VM:    vm,
		done:  make(chan bool),
	}
}

func DialVNC(vm *KvmVM) (*vncClient, error) {
	c := NewVNCClient(vm)

	conn, err := vnc.Dial(c.Rhost)
	if err != nil {
		return nil, err
	}

	c.Conn = conn
	return c, nil
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

func vncInject(vm *KvmVM, e Event) error {
	c, err := DialVNC(vm)
	if err != nil {
		return err
	}
	defer c.Stop()

	return e.Write(c.Conn)
}
