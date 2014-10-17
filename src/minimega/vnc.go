// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"strconv"
)

func cliVNC(c cliCommand) cliResponse {
	switch len(c.Args) {
	case 3: // [norecord|noplayback] <host> <vm>
		if c.Args[0] != "norecord" && c.Args[0] != "noplayback" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		host := c.Args[1]
		vm, err := strconv.Atoi(c.Args[2])
		if err != nil {
			log.Errorln(err)
			return cliResponse{
				Error: err.Error(),
			}
		}
		rhost := fmt.Sprintf("%v:%v", host, 5900+vm)
		switch {
		case c.Args[0] == "norecord":
			if _, ok := vncRecording[rhost]; ok {
				vncRecording[rhost].Close()
				delete(vncRecording, rhost)
			}
		case c.Args[0] == "noplayback":
			if _, ok := vncPlaying[rhost]; ok {
				vncPlaying[rhost].Stop()
				// will be deleted elsewhere
			}
		}
	case 4: // [record|playback] <host> <vm> <file>
		if c.Args[0] != "record" && c.Args[0] != "playback" {
			return cliResponse{
				Error: "malformed command",
			}
		}
		host := c.Args[1]
		vm, err := strconv.Atoi(c.Args[2])
		if err != nil {
			log.Errorln(err)
			return cliResponse{
				Error: err.Error(),
			}
		}
		filename := c.Args[3]
		rhost := fmt.Sprintf("%v:%v", host, 5900+vm)

		switch {
		case c.Args[0] == "record":
			vmr, err := NewVMRecord(filename)
			if err != nil {
				log.Errorln(err)
				return cliResponse{
					Error: err.Error(),
				}
			}
			vncRecording[rhost] = vmr
		case c.Args[0] == "playback":
			vmp, err := NewVMPlayback(filename)
			if err != nil {
				log.Errorln(err)
				return cliResponse{
					Error: err.Error(),
				}
			}
			vncPlaying[rhost] = vmp
			go vmp.Run()
		}
	default:
		return cliResponse{
			Error: "malformed command",
		}
	}
	return cliResponse{}
}
