// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

var shellCLIHandlers = []minicli.Handler{
	{ // shell
		HelpShort: "execute a command",
		HelpLong: `
Execute a command under the credentials of the running user.

Commands run until they complete or error, so take care not to execute a command
that does not return.`,
		Patterns: []string{
			"shell <command>...",
		},
		Call: wrapSimpleCLI(func(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
			return cliShell(c, resp)
		}),
	},
	{ // background
		HelpShort: "execute a command in the background",
		HelpLong: `
Execute a command under the credentials of the running user.

Commands run in the background and control returns immediately. Any output is
logged at the "info" level.`,
		Patterns: []string{
			"background <command>...",
		},
		Call: cliBackground,
	},
	{ // background status
		HelpShort: "Get the status of background commands",
		HelpLong: `
Get the status of a background command / commands.

To get the status of all background commands run

	background-status

To get the status of a specific command, run

	background-status [id]`,
		Patterns: []string{
			"background-status [id]",
		},
		Call: cliBackgroundStatus,
	},
	{ // background output
		HelpShort: "Get the stdout of a background command",
		HelpLong: `
Get the standard output of a background command.

	background-output <id>`,
		Patterns: []string{
			"background-output <id>",
		},
		Call: cliBackgroundOutput,
	},
	{ // background error
		HelpShort: "Get the stderr of a background command",
		HelpLong: `
Get the standard error of a background command.

	background-error <id>`,
		Patterns: []string{
			"background-error <id>",
		},
		Call: cliBackgroundError,
	},
	{ //clear background-status
		HelpShort: "Clear background-status information",
		Patterns: []string{
			"clear background-status",
		},
		Call: cliClearBackgroundStatus,
	},
}

func cliShell(c *minicli.Command, resp *minicli.Response) error {
	var sOut bytes.Buffer
	var sErr bytes.Buffer

	p, err := exec.LookPath(c.ListArgs["command"][0])
	if err != nil {
		return err
	}

	args := []string{p}
	if len(c.ListArgs["command"]) > 1 {
		args = append(args, c.ListArgs["command"][1:]...)
	}

	cmd := &exec.Cmd{
		Path:   p,
		Args:   args,
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("starting: %v", args)
	if err := cmd.Start(); err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		return err
	}

	resp.Response = sOut.String()
	resp.Error = sErr.String()

	return nil
}

type BackgroundProcess struct {
	ID        int
	Command   *exec.Cmd
	Running   bool
	Error     error
	TimeStart time.Time
	TimeEnd   time.Time
	Stdout    string
	Stderr    string
}

func (bp BackgroundProcess) ToTabular() []string {
	errorsString := ""
	if bp.Error != nil {
		errorsString = bp.Error.Error()
	}
	return []string{
		strconv.FormatInt(int64(bp.ID), 10),
		strconv.FormatInt(int64(bp.Command.Process.Pid), 10),
		strconv.FormatBool(bp.Running),
		errorsString,
		bp.TimeStart.Format("Jan 02 15:04:05 MST"),
		bp.TimeEnd.Format("Jan 02 15:04:05 MST"),
		bp.Command.String(),
	}
}

const (
	backgroundProcessMaxLen = 50
)

var (
	backgroundProcessesRWLock sync.RWMutex
	backgroundProcesses           = make(map[int]*BackgroundProcess)
	backgroundProcessNextID   int = 1

	backgroundProcessTableHeader = []string{"ID", "PID", "RUNNING", "ERROR", "TIME_START", "TIME_END", "COMMAND"}
)

type BackgroundWriter struct {
	ProcessID int
}

func (bw *BackgroundWriter) Write(data []byte) (n int, err error) {
	logString := fmt.Sprintf("Background process %d: %s", bw.ProcessID, string(data))
	log.Info(logString)
	return len(data), nil
}

func cliBackground(c *minicli.Command, respChan chan<- minicli.Responses) {

	flushOldBackgroundStatus()

	p, err := exec.LookPath(c.ListArgs["command"][0])
	if err != nil {
		respChan <- minicli.Responses{&minicli.Response{Error: fmt.Sprintf("%v", err)}}
		return
	}

	args := []string{p}
	if len(c.ListArgs["command"]) > 1 {
		args = append(args, c.ListArgs["command"][1:]...)
	}

	cmd := &exec.Cmd{
		Path: p,
		Args: args,
		Env:  nil,
		Dir:  "",
	}

	var sOut bytes.Buffer //stdout buffer
	var sErr bytes.Buffer //stderr buffer

	backgroundProcessesRWLock.Lock()

	id := backgroundProcessNextID

	bgWriter := &BackgroundWriter{ProcessID: id}
	cmd.Stdout = io.MultiWriter(&sOut, bgWriter)
	cmd.Stderr = io.MultiWriter(&sErr, bgWriter)

	bp := &BackgroundProcess{
		ID:        id,
		Command:   cmd,
		Running:   true,
		TimeStart: time.Now(),
	}
	backgroundProcessNextID += 1
	backgroundProcesses[bp.ID] = bp

	backgroundProcessesRWLock.Unlock()

	log.Info("starting process id %v: %v", id, args)
	respChan <- minicli.Responses{&minicli.Response{
		Host:     hostname,
		Response: fmt.Sprintf("Started background process with id %d", id),
	}}

	go func() {
		err := cmd.Run()

		backgroundProcessesRWLock.Lock()
		defer backgroundProcessesRWLock.Unlock()

		backgroundProcesses[id].Running = false
		backgroundProcesses[id].Error = err
		backgroundProcesses[id].TimeEnd = time.Now()
		backgroundProcesses[id].Stdout = sOut.String()
		backgroundProcesses[id].Stderr = sErr.String()

		log.Info("command %v exited", args)
	}()
}

func cliBackgroundStatus(c *minicli.Command, respChan chan<- minicli.Responses) {
	idStr := c.StringArgs["id"]

	if idStr == "" {
		cliBackgroundStatusAll(respChan)
		return
	}

	idInt, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respChan <- errResp(err)
		return
	}

	backgroundProcessesRWLock.RLock()
	defer backgroundProcessesRWLock.RUnlock()

	entry, ok := backgroundProcesses[int(idInt)]
	if !ok {
		respChan <- errResp(fmt.Errorf("provided id does not exist in background process table"))
		return
	}

	resp := &minicli.Response{
		Host:    hostname,
		Header:  backgroundProcessTableHeader,
		Tabular: [][]string{entry.ToTabular()},
	}
	respChan <- minicli.Responses{resp}

}

func cliBackgroundStatusAll(respChan chan<- minicli.Responses) {
	backgroundProcessesRWLock.RLock()
	defer backgroundProcessesRWLock.RUnlock()

	var table [][]string
	for _, entry := range backgroundProcesses {
		table = append(table, entry.ToTabular())
	}

	resp := &minicli.Response{
		Host:    hostname,
		Header:  backgroundProcessTableHeader,
		Tabular: table,
	}
	respChan <- minicli.Responses{resp}

}

func cliBackgroundOutput(c *minicli.Command, respChan chan<- minicli.Responses) {
	idStr := c.StringArgs["id"]

	if idStr == "" {
		respChan <- errResp(fmt.Errorf("ID is a required field"))
		return
	}

	idInt, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respChan <- errResp(err)
		return
	}

	backgroundProcessesRWLock.RLock()
	defer backgroundProcessesRWLock.RUnlock()

	entry, ok := backgroundProcesses[int(idInt)]
	if !ok {
		respChan <- errResp(fmt.Errorf("provided id does not exist in background process table"))
		return
	}

	if entry.Running {
		respChan <- errResp(fmt.Errorf("wait for process to finish before reading output"))
		return
	}

	resp := &minicli.Response{
		Response: entry.Stdout,
		Host:     hostname,
	}
	respChan <- minicli.Responses{resp}

}

func cliBackgroundError(c *minicli.Command, respChan chan<- minicli.Responses) {
	idStr := c.StringArgs["id"]

	if idStr == "" {
		respChan <- errResp(fmt.Errorf("ID is a required field"))
		return
	}

	idInt, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		respChan <- errResp(err)
		return
	}

	backgroundProcessesRWLock.RLock()
	defer backgroundProcessesRWLock.RUnlock()

	entry, ok := backgroundProcesses[int(idInt)]
	if !ok {
		respChan <- errResp(fmt.Errorf("provided id does not exist in background process table"))
		return
	}

	if entry.Running {
		respChan <- errResp(fmt.Errorf("wait for process to finish before reading output"))
		return
	}

	resp := &minicli.Response{
		Response: entry.Stderr,
		Host:     hostname,
	}
	respChan <- minicli.Responses{resp}

}

func cliClearBackgroundStatus(c *minicli.Command, respChan chan<- minicli.Responses) {
	backgroundProcessesRWLock.Lock()
	backgroundProcessesRWLock.Unlock()

	var keys []int
	for key, entry := range backgroundProcesses {
		if !entry.Running {
			keys = append(keys, key)
		}
	}

	if len(keys) == len(backgroundProcesses) {
		//free the entire map if all are not running and clear is called
		backgroundProcesses = make(map[int]*BackgroundProcess)
	} else {
		for _, key := range keys {
			delete(backgroundProcesses, key)
		}

	}

	resp := &minicli.Response{
		Host:     hostname,
		Response: fmt.Sprintf("Cleared %d elements from background-status", len(keys)),
	}

	respChan <- minicli.Responses{resp}
}

// flush any statuses over the max length, sorted by ending time
func flushOldBackgroundStatus() {
	backgroundProcessesRWLock.Lock()
	backgroundProcessesRWLock.Unlock()

	if len(backgroundProcesses) < backgroundProcessMaxLen {
		return
	}

	var notRunningKeys []int
	for key, entry := range backgroundProcesses {
		if !entry.Running {
			notRunningKeys = append(notRunningKeys, key)
		}
	}

	sort.Slice(notRunningKeys, func(a, b int) bool {
		a_key := notRunningKeys[a]
		b_key := notRunningKeys[b]
		a_entry := backgroundProcesses[a_key]
		b_entry := backgroundProcesses[b_key]

		return a_entry.TimeEnd.Before(b_entry.TimeEnd)
	})

	for _, key := range notRunningKeys {
		if len(backgroundProcesses) <= backgroundProcessMaxLen {
			break
		}

		delete(backgroundProcesses, key)
	}

}
