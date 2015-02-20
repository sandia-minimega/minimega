// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"strconv"
)

var vncCLIHandlers = []minicli.Handler{
	{ // vnc
		HelpShort: "record or playback VNC kbd/mouse input",
		HelpLong: `
Record or playback keyboard and mouse events sent via the web interface to the
selected VM.

With no arguments, vnc will list currently recording or playing VNC sessions.

If record is selected, a file will be created containing a record of mouse and
keyboard actions by the user.

If playback is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.`,
		Patterns: []string{
			"vnc",
			"vnc <kb,fb> <record,> <host> <vm id or name> <filename>",
			"vnc <kb,fb> <norecord,> <host> <vm id or name>",
			"vnc <playback,> <host> <vm id or name> <filename>",
			"vnc <noplayback,> <host> <vm id or name>",
		},
		Call: wrapSimpleCLI(cliVNC),
	},
	{ // clear vnc
		HelpShort: "reset VNC state",
		HelpLong: `
Resets the state for VNC recordings. See "help vnc" for more information.`,
		Patterns: []string{
			"clear vnc",
		},
		Call: wrapSimpleCLI(cliVNCClear),
	},
}

func init() {
	registerHandlers("vnc", vncCLIHandlers)
}

func cliVNC(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	host := c.StringArgs["host"]
	vm := c.StringArgs["vm"]
	fname := c.StringArgs["filename"]

	if c.BoolArgs["record"] && c.BoolArgs["kb"] {
		// Starting keyboard recording
		err = vncRecordKB(host, vm, fname)
	} else if c.BoolArgs["record"] && c.BoolArgs["fb"] {
		// Starting framebuffer recording
		err = vncRecordFB(host, vm, fname)
	} else if c.BoolArgs["norecord"] || c.BoolArgs["noplayback"] {
		var client *vncClient

		if c.BoolArgs["norecord"] && c.BoolArgs["kb"] {
			err = fmt.Errorf("kb recording %v %v not found", host, vm)
			for k, v := range vncKBRecording {
				if v.Matches(host, vm) {
					client = v.vncClient
					delete(vncKBRecording, k)
					break
				}
			}
		} else if c.BoolArgs["norecord"] && c.BoolArgs["fb"] {
			err = fmt.Errorf("fb recording %v %v not found", host, vm)
			for k, v := range vncFBRecording {
				if v.Matches(host, vm) {
					client = v.vncClient
					delete(vncFBRecording, k)
					break
				}
			}
		} else if c.BoolArgs["noplayback"] {
			err = fmt.Errorf("kb playback %v %v not found", host, vm)
			for k, v := range vncKBPlaying {
				if v.Matches(host, vm) {
					client = v.vncClient
					delete(vncKBPlaying, k)
					break
				}
			}
		}

		if client != nil {
			if err := client.Stop(); err != nil {
				log.Error("%v", err)
			}
			err = nil
		}
	} else if c.BoolArgs["playback"] {
		// Start keyboard playback
		err = vncPlaybackKB(host, vm, fname)
	} else {
		// List all active recordings and playbacks
		resp.Header = []string{"Host", "VM name", "VM id", "Type", "Filename"}
		resp.Tabular = [][]string{}

		for _, v := range vncKBRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record kb",
				v.file.Name(),
			})
		}

		for _, v := range vncFBRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record fb",
				v.file.Name(),
			})
		}

		for _, v := range vncKBPlaying {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"playback kb",
				v.file.Name(),
			})
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func cliVNCClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	err := vncClear()
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
