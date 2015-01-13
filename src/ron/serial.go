// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package ron

import (
	"encoding/gob"
	"goserial"
	log "minilog"
)

const (
	BAUDRATE = 115200
)

func (r *Ron) serialDial() error {
	c := &serial.Config{
		Name: r.serialPath,
		Baud: BAUDRATE,
	}

	s, err := serial.OpenPort(c)
	if err != nil {
		return err
	}

	r.serialClientHandle = s

	return nil
}

func (r *Ron) serialHeartbeat(h *hb) (map[int]*Command, error, bool) {
	if r.serialClientHandle == nil {
		log.Fatalln("no serial handle!")
	}

	enc := gob.NewEncoder(r.serialClientHandle)

	err := enc.Encode(h)
	if err != nil {
		return nil, err, false
	}

	newCommands := make(map[int]*Command)
	dec := gob.NewDecoder(r.serialClientHandle)

	err = dec.Decode(&newCommands)
	if err != nil {
		return nil, err, true
	}

	return newCommands, nil, true
}
