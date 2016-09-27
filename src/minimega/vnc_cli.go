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
		HelpShort: "record or playback VNC kb or fb",
		HelpLong: `
Record or playback keyboard and mouse events sent via the web interface to the
selected VM. Can also record the framebuffer for the specified VM so that a
users can watch a video of interactions with the VM.

With no arguments, vnc will list currently recording or playing VNC sessions.

If record is selected, a file will be created containing a record of mouse and
keyboard actions by the user or of the framebuffer for the VM.

If playback is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.`,
		Patterns: []string{
			"vnc <kb,fb> <record,> <vm name> <filename>",
			"vnc <kb,fb> <norecord,> <vm name>",
			"vnc <playback,> <vm name> <filename>",
			"vnc <noplayback,> <vm name>",
		},
		Call: wrapVMTargetCLI(cliVNC),
	},
	{
		HelpShort: "reset VNC state",
		HelpLong: `
Resets the state for VNC recordings. See "help vnc" for more information.`,
		Patterns: []string{
			"clear vnc",
		},
		Call: wrapVMTargetCLI(cliVNCClear),
	},
	{
		HelpShort: "list all running vnc playback/recording instances",
		HelpLong: `
List all running vnc playback/recording instances. See "help vnc" for more information.`,
		Patterns: []string{
			"vnc",
		},
		Call: wrapBroadcastCLI(vncList),
	},
}

// List all active recordings and playbacks
func vncList(c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{"host", "name", "id", "type", "filename"}
	resp.Tabular = [][]string{}

	for _, v := range vncKBRecording {
		resp.Tabular = append(resp.Tabular, []string{
			hostname, v.Name, strconv.Itoa(v.ID),
			"record kb",
			v.file.Name(),
		})
	}

	for _, v := range vncFBRecording {
		resp.Tabular = append(resp.Tabular, []string{
			hostname, v.Name, strconv.Itoa(v.ID),
			"record fb",
			v.file.Name(),
		})
	}

	for _, v := range vncKBPlaying {
		resp.Tabular = append(resp.Tabular, []string{
			hostname, v.Name, strconv.Itoa(v.ID),
			"playback kb",
			v.file.Name(),
		})
	}

	return nil
}

func cliVNC(c *minicli.Command, resp *minicli.Response) error {
	vm := c.StringArgs["vm"]
	fname := c.StringArgs["filename"]

	if c.BoolArgs["record"] && c.BoolArgs["kb"] {
		// Starting keyboard recording
		return vncRecordKB(vm, fname)
	} else if c.BoolArgs["record"] && c.BoolArgs["fb"] {
		// Starting framebuffer recording
		return vncRecordFB(vm, fname)
	} else if c.BoolArgs["norecord"] || c.BoolArgs["noplayback"] {
		var err error
		var client *vncClient

		if c.BoolArgs["norecord"] && c.BoolArgs["kb"] {
			err = fmt.Errorf("kb recording %v %v not found", hostname, vm)
			for k, v := range vncKBRecording {
				if v.Matches(hostname, vm) {
					client = v.vncClient
					delete(vncKBRecording, k)
					break
				}
			}
		} else if c.BoolArgs["norecord"] && c.BoolArgs["fb"] {
			err = fmt.Errorf("fb recording %v %v not found", hostname, vm)
			for k, v := range vncFBRecording {
				if v.Matches(hostname, vm) {
					client = v.vncClient
					delete(vncFBRecording, k)
					break
				}
			}
		} else if c.BoolArgs["noplayback"] {
			err = fmt.Errorf("kb playback %v %v not found", hostname, vm)
			for k, v := range vncKBPlaying {
				if v.Matches(hostname, vm) {
					client = v.vncClient
					delete(vncKBPlaying, k)
					break
				}
			}
		}

		if client != nil {
			return client.Stop()
		}

		return err
	} else if c.BoolArgs["playback"] {
		// Start keyboard playback
		return vncPlaybackKB(vm, fname)
	}
	return nil
}

func cliVNCClear(c *minicli.Command, resp *minicli.Response) error {
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
