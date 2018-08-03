// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"minicli"
	"path/filepath"
	"sync"
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
the specified VM(s). See "vm start" for a full description of the allowable
targets. VMs without a valid playback that are part of the target will return a
"kb playback not found" error.

Playbacks can be paused with the pause command, and resumed using continue. The
step command will immediately move to the next event contained in the playback
file. Use the getstep command to view the current vnc event. Calling stop will
end a playback.

VNC playback also supports injecting mouse/keyboard events in the format found
in the playback file. Injected commands must omit the time delta as they are
sent immediately.

vnc host vm_id inject PointerEvent,0,465,245

Comments in the playback file are logged at the info level. An example is given
below.

#: This is an example of a vnc playback comment`,
		Patterns: []string{
			"vnc <play,> <vm target> <filename>",
			"vnc <stop,> <vm target>",
			"vnc <pause,> <vm target>",
			"vnc <continue,> <vm target>",
			"vnc <step,> <vm target>",
			"vnc <getstep,> <vm target>",
			"vnc <inject,> <vm target> <cmd>",
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

	target := c.StringArgs["vm"]

	// synchronize adding rows to resp.Tabular for getstep
	var mu sync.Mutex
	if c.BoolArgs["getstep"] {
		resp.Header = []string{"name", "step"}
	}

	return ns.Apply(target, func(vm VM, _ bool) (bool, error) {
		kvm, ok := vm.(*KvmVM)
		if !ok {
			return false, nil
		}

		switch {
		case c.BoolArgs["play"]:
			return true, ns.vncPlayer.PlaybackKB(kvm, fname)
		case c.BoolArgs["stop"]:
			return true, ns.vncPlayer.Stop(kvm)
		case c.BoolArgs["inject"]:
			return true, ns.vncPlayer.Inject(kvm, c.StringArgs["cmd"])
		case c.BoolArgs["pause"]:
			return true, ns.vncPlayer.Pause(kvm)
		case c.BoolArgs["continue"]:
			return true, ns.vncPlayer.Continue(kvm)
		case c.BoolArgs["step"]:
			return true, ns.vncPlayer.Step(kvm)
		case c.BoolArgs["getstep"]:
			res, err := ns.vncPlayer.GetStep(kvm)
			if err != nil {
				return true, err
			}

			// append to tabular
			mu.Lock()
			defer mu.Unlock()

			resp.Tabular = append(resp.Tabular, []string{
				vm.GetName(),
				res,
			})
		}

		// strange...
		return true, errors.New("unreachable")
	})
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
