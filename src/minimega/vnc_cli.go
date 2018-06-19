// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"minicli"
	"path/filepath"
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
and keyboard actions by the user or of the framebuffer for the VM.

Note: recordings are written to the host where the VM is running.`,
		Patterns: []string{
			"vnc <record,> <kb,fb> <vm name> <filename>",
			"vnc <stop,> <kb,fb> <vm name>",
		},
		Call:    wrapVMTargetCLI(cliVNCRecord),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
	},
	{
		HelpShort: "play VNC kb",
		HelpLong: `
Playback and interact with a previously recorded vnc kb session file.

If play is selected, the specified file (created using vnc record) will be read
and processed as a sequence of time-stamped mouse/keyboard events to send to
the specified VM.

Playbacks can be paused with the pause command, and resumed using continue. The
step command will immediately move to the next event contained in the playback
file. Use the getstep command to view the current vnc event. Calling stop will
end a playback.

Vnc playback also supports injecting mouse/keyboard events in the format found
in the playback file. Injected commands must omit the time delta as they are
sent immediately.

vnc host vm_id inject PointerEvent,0,465,245

Comments in the playback file are logged at the info level. An example is given
below.

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
		Call:    wrapVMTargetCLI(cliVNCPlay),
		Suggest: wrapVMSuggest(VM_ANY_STATE, false),
	},
	{
		HelpShort: "reset VNC state",
		HelpLong: `
Resets the state for VNC recordings. See "help vnc" for more information.`,
		Patterns: []string{
			"clear vnc",
		},
		Call: wrapBroadcastCLI(func(ns *Namespace, _ *minicli.Command, _ *minicli.Response) error {
			ns.vncRecorder.Clear()
			ns.vncPlayer.Clear()
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

func cliVNCPlay(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	fname := c.StringArgs["filename"]
	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(fname) {
		// TODO: should we capture to the VM directory instead?
		fname = filepath.Join(*f_iomBase, fname)
	}

	vm, err := ns.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	id := vm.Name

	if c.BoolArgs["play"] {
		return ns.PlaybackKB(vm, fname)
	} else if c.BoolArgs["stop"] {
		ns.vncPlayer.Lock()
		defer ns.vncPlayer.Unlock()

		if p := ns.vncPlayer.m[id]; p != nil {
			return p.Stop()
		}

		return fmt.Errorf("kb playback %v not found", vm.Name)
	} else if c.BoolArgs["inject"] {
		return ns.vncPlayer.Inject(vm, c.StringArgs["cmd"])
	}

	// Need a valid playback for all other operations
	ns.vncPlayer.RLock()
	defer ns.vncPlayer.RUnlock()

	p := ns.vncPlayer.m[id]
	if p == nil {
		return fmt.Errorf("kb playback %v not found", vm.Name)
	}

	// Running playback commands
	if c.BoolArgs["pause"] {
		return p.Pause()
	} else if c.BoolArgs["continue"] {
		return p.Continue()
	} else if c.BoolArgs["step"] {
		return p.Step()
	} else if c.BoolArgs["getstep"] {
		r, err := p.GetStep()
		if err != nil {
			return err
		}
		resp.Response = r
	}

	return nil
}

func cliVNCRecord(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	fname := c.StringArgs["filename"]
	// Ensure that relative paths are always relative to /files/
	if !filepath.IsAbs(fname) {
		// TODO: should we capture to the VM directory instead?
		fname = filepath.Join(*f_iomBase, fname)
	}

	vm, err := ns.FindKvmVM(c.StringArgs["vm"])
	if err != nil {
		return err
	}

	if c.BoolArgs["record"] {
		if c.BoolArgs["kb"] {
			return ns.RecordKB(vm, fname)
		}

		return ns.RecordFB(vm, fname)
	}

	// must want to stop recording
	ns.vncRecorder.Lock()
	defer ns.vncRecorder.Unlock()

	id := vm.Name

	if c.BoolArgs["kb"] {
		if v, ok := ns.vncRecorder.kb[id]; ok {
			delete(ns.vncRecorder.kb, id)
			return v.vncClient.Stop()
		}

		return fmt.Errorf("kb recording %v not found", vm.Name)
	}

	if v, ok := ns.vncRecorder.fb[id]; ok {
		delete(ns.vncRecorder.fb, id)
		return v.vncClient.Stop()
	}

	return fmt.Errorf("fb recording %v not found", vm.Name)
}

// List all active recordings and playbacks
func cliVNCList(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Header = []string{"name", "type", "time", "filename"}

	ns.vncRecorder.RLock()
	defer ns.vncRecorder.RUnlock()
	ns.vncPlayer.RLock()
	defer ns.vncPlayer.RUnlock()

	ns.vncPlayer.reap()

	for _, v := range ns.vncRecorder.kb {
		resp.Tabular = append(resp.Tabular, []string{
			v.VM.Name, "record kb",
			time.Since(v.start).String(),
			v.file.Name(),
		})
	}

	for _, v := range ns.vncRecorder.fb {
		resp.Tabular = append(resp.Tabular, []string{
			v.VM.Name, "record fb",
			time.Since(v.start).String(),
			v.file.Name(),
		})
	}

	for _, v := range ns.vncPlayer.m {
		if info := v.Info(); info != nil {
			resp.Tabular = append(resp.Tabular, info)
		}
	}

	return nil
}
