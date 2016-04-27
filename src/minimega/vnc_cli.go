// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	log "minilog"
	"strconv"
	"time"
)

var vncCLIHandlers = []minicli.Handler{
	{ // vnc
		HelpShort: "record VNC kb or fb",
		HelpLong: `
Record keyboard and mouse events sent via the web interface to the
selected VM. Can also record the framebuffer for the specified VM so that a
user can watch a video of interactions with the VM.

With no arguments, vnc will list currently recording or playing VNC sessions.

If record is selected, a file will be created containing a record of mouse and
keyboard actions by the user or of the framebuffer for the VM.`,
		Patterns: []string{
			"vnc",
			"vnc <kb,fb> <record,> <host> <vm id or name> <filename>",
			"vnc <kb,fb> <stop,> <host> <vm id or name>",
		},
		Call: wrapSimpleCLI(cliVNCRecord),
	},
	{ // play vnc kb
		HelpShort: "play VNC kb",
		HelpLong: `
Playback and interact with a previously recorded vnc kb session file.

If play is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.

Playbacks can be paused with the pause command, and resumed using continue.
The step command will immediately move to the next event contained in the playback
file. Use the getstep command to view the current vnc event. Calling stop will a
playback.

Vnc playback also supports injecting mouse/keyboard events in the format found in
the playback file.

Comments in the playback file are logged at the info level. An example is given below.

#: This is an example of a vnc playback comment`,
		Patterns: []string{
			"vnc <play,> <host> <vm id or name> <filename>",
			"vnc <stop,> <host> <vm id or name>",
			"vnc <pause,> <host> <vm id or name>",
			"vnc <continue,> <host> <vm id or name>",
			"vnc <step,> <host> <vm id or name>",
			"vnc <getstep,> <host> <vm id or name>",
			"vnc <inject,> <host> <vm id or name> <cmd>",
		},
		Call: wrapSimpleCLI(cliVNCPlay),
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

func cliVNCPlay(c *minicli.Command) *minicli.Response {
	var err error
	var p *vncKBPlayback

	resp := &minicli.Response{Host: hostname}
	host := c.StringArgs["host"]
	vm := c.StringArgs["vm"]
	fname := c.StringArgs["filename"]

	// Get the vnc client
	for _, v := range vncKBPlaying {
		if v.Matches(host, vm) {
			p = v
			break
		}
	}

	if c.BoolArgs["play"] {
		if p != nil {
			err = fmt.Errorf("kb playback %v %v already playing", host, vm)
		} else {
			// Start the playback
			err = vncPlaybackKB(host, vm, fname)
		}
	} else {
		// Need a valid playback for all other operations
		if p == nil {
			err = fmt.Errorf("kb playback %v %v not found", host, vm)
			resp.Error = err.Error()
			return resp
		}

		if c.BoolArgs["stop"] {
			err = p.Stop()
		} else if c.BoolArgs["pause"] {
			err = p.Pause()
		} else if c.BoolArgs["continue"] {
			err = p.Continue()
		} else if c.BoolArgs["step"] {
			err = p.Step()
		} else if c.BoolArgs["getstep"] {
			resp.Response, err = p.GetStep()
		} else {
			err = p.Inject(c.StringArgs["cmd"])
		}
	}

	if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func cliVNCRecord(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	var err error

	host := c.StringArgs["host"]
	vm := c.StringArgs["vm"]
	fname := c.StringArgs["filename"]

	if host == Localhost {
		host = hostname
	}

	var client *vncClient

	if c.BoolArgs["record"] {
		if c.BoolArgs["kb"] {
			// Starting keyboard recording
			err = vncRecordKB(host, vm, fname)
		} else {
			err = vncRecordFB(host, vm, fname)
		}
	} else if c.BoolArgs["stop"] {
		if c.BoolArgs["kb"] {
			err = fmt.Errorf("kb recording %v %v not found", host, vm)
			for k, v := range vncKBRecording {
				if v.Matches(host, vm) {
					client = v.vncClient
					delete(vncKBRecording, k)
					break
				}
			}
		} else {
			err = fmt.Errorf("fb recording %v %v not found", host, vm)
			for k, v := range vncFBRecording {
				if v.Matches(host, vm) {
					client = v.vncClient
					delete(vncFBRecording, k)
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
	} else {
		// List all active recordings and playbacks
		resp.Header = []string{"host", "name", "id", "type", "time", "filename"}
		resp.Tabular = [][]string{}

		for _, v := range vncKBRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record kb",
				time.Since(v.start).String(),
				v.file.Name(),
			})
		}

		for _, v := range vncFBRecording {
			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"record fb",
				time.Since(v.start).String(),
				v.file.Name(),
			})
		}

		for _, v := range vncKBPlaying {
			var r string
			if v.state == Pause {
				r = "PAUSED"
			} else {
				r = v.timeRemaining() + " remaining"
			}

			resp.Tabular = append(resp.Tabular, []string{
				v.Host, v.Name, strconv.Itoa(v.ID),
				"playback kb",
				r,
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
