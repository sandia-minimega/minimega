// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"time"
)

var vncCLIHandlers = []minicli.Handler{
	{ // vnc
		HelpShort: "record VNC kb or fb",
		HelpLong: `
Record keyboard and mouse events sent via the web interface to the
selected VM. Can also record the framebuffer for the specified VM so that a
user can watch a video of interactions with the VM.

If record is selected, a file will be created containing a record of mouse
and keyboard actions by the user or of the framebuffer for the VM.`,
		Patterns: []string{
			"vnc <kb,fb> <record,> <vm name> <filename>",
			"vnc <kb,fb> <stop,> <vm name>",
		},
		Call: wrapSimpleCLI(cliVNCRecord),
	},
	{
		HelpShort: "play VNC kb",
		HelpLong: `
Playback and interact with a previously recorded vnc kb session file.

If play is selected, the specified file (created using vnc record) will be
read and processed as a sequence of time-stamped mouse/keyboard events to send
to the specified VM.

Playbacks can be paused with the pause command, and resumed using continue.
The step command will immediately move to the next event contained in the playback
file. Use the getstep command to view the current vnc event. Calling stop will end
a playback.

Vnc playback also supports injecting mouse/keyboard events in the format found in
the playback file. Injected commands must omit the time delta as they are sent
immediately.

vnc host vm_id inject PointerEvent,0,465,245

Comments in the playback file are logged at the info level. An example is given below.

#: This is an example of a vnc playback comment`,
		Patterns: []string{
			"vnc <play,> <vm name> <filename>",
			"vnc <stop,> <vm name>",
			"vnc <pause,> <vm name>",
			"vnc <continue,> <vm name>",
			"vnc <step,> <vm name>",
			"vnc <getstep,> <vm name>",
			"vnc <inject,> <vm name> <cmd>",
		},
		Call: wrapBroadcastCLI(cliVNCPlay),
	},
	{
		HelpShort: "reset VNC state",
		HelpLong: `
Resets the state for VNC recordings. See "help vnc" for more information.`,
		Patterns: []string{
			"clear vnc",
		},
		Call: wrapBroadcastCLI(func(_ *minicli.Command, _ *minicli.Response) error {
			vncClear()
			return nil
		}),
	},
	{
		HelpShort: "list all running vnc playback/recording instances",
		HelpLong: `
List all running vnc playback/recording instances. See "help vnc" for more information.`,
		Patterns: []string{
			"vnc",
		},
		Call: wrapBroadcastCLI(cliVNCList),
	},
}

func cliVNCPlay(c *minicli.Command, resp *minicli.Response) error {
	var err error
	var p *vncKBPlayback

	fname := c.StringArgs["filename"]

	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return fmt.Errorf("vm %s not found", c.StringArgs["vm"])
	}

	// Get the playback
	id := fmt.Sprintf("%v:%v", vm.Name, vm.Namespace)
	p, _ = vncKBPlaying[id]

	if c.BoolArgs["play"] {
		if p != nil {
			err = fmt.Errorf("kb playback %v already playing", vm.Name)
		} else {
			// Start the playback
			err = vncPlaybackKB(vm, fname)
		}
	} else {
		// Need a valid playback for all other operations
		if p == nil {
			return fmt.Errorf("kb playback %v not found", vm.Name)
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
	return err
}

func cliVNCRecord(c *minicli.Command, resp *minicli.Response) error {
	var err error

	fname := c.StringArgs["filename"]

	vm, err := vms.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return fmt.Errorf("vm %s not found", c.StringArgs["vm"])
	}

	var client *vncClient
	id := fmt.Sprintf("%v:%v", vm.Name, vm.Namespace)

	if c.BoolArgs["record"] {
		// Start a keyboard / framebuffer recording
		if c.BoolArgs["kb"] {
			err = vncRecordKB(vm, fname)
		} else {
			err = vncRecordFB(vm, fname)
		}
	}
	if c.BoolArgs["stop"] {
		if c.BoolArgs["kb"] {
			err = fmt.Errorf("kb recording %v not found", vm.Name)
			if v, ok := vncKBRecording[id]; ok {
				client = v.vncClient
				delete(vncKBRecording, id)
			}
		} else {
			err = fmt.Errorf("fb recording %v not found", vm.Name)
			if v, ok := vncFBRecording[id]; ok {
				client = v.vncClient
				delete(vncFBRecording, id)
			}
		}

		if client != nil {
			return client.Stop()
		}
	}
	return err
}

// List all active recordings and playbacks
func cliVNCList(c *minicli.Command, resp *minicli.Response) error {
	// List all active recordings and playbacks
	resp.Header = []string{"host", "name", "type", "time", "filename"}
	resp.Tabular = [][]string{}

	for _, v := range vncKBRecording {
		resp.Tabular = append(resp.Tabular, []string{
			v.VM.Host, v.VM.Name, "record kb",
			time.Since(v.start).String(),
			v.file.Name(),
		})
	}

	for _, v := range vncFBRecording {
		resp.Tabular = append(resp.Tabular, []string{
			v.VM.Host, v.VM.Name, "record fb",
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
			v.VM.Host, v.VM.Name, "playback kb",
			r,
			v.file.Name(),
		})
	}
	return nil
}
