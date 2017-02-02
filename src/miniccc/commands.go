// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	log "minilog"
	"os/exec"
	"ron"
	"strings"
)

func processCommand(cmd *ron.Command) {
	log.Debug("processCommand %v", cmd)

	resp := &ron.Response{
		ID: cmd.ID,
	}

	// get any files needed for the command
	if len(cmd.FilesSend) != 0 {
		recvFiles(cmd.FilesSend)
	}

	// kill processes before starting new ones
	if cmd.PID != 0 {
		kill(cmd.PID)
	}
	if cmd.KillAll != "" {
		killAll(cmd.KillAll)
	}

	// adjust the log level, if a new level is provided
	if cmd.Level != nil {
		log.Info("setting level to: %v", *cmd.Level)
		log.SetLevelAll(*cmd.Level)
	}

	if len(cmd.Command) != 0 {
		resp.Stdout, resp.Stderr = runCommand(cmd.Command, cmd.Background)
	}

	if len(cmd.FilesRecv) != 0 {
		resp.Files = readFiles(cmd.FilesRecv)
	}

	appendResponse(resp)
}

func runCommand(command []string, background bool) (string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	path, err := exec.LookPath(command[0])
	if err != nil {
		log.Errorln(err)
		return "", err.Error()
	}

	cmd := &exec.Cmd{
		Path:   path,
		Args:   command,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	log.Info("executing: %v", command)

	if background {
		log.Debug("starting in background")
		if err := cmd.Start(); err != nil {
			log.Errorln(err)
			return "", stderr.String()
		}

		pid := cmd.Process.Pid

		client.Lock()
		defer client.Unlock()
		client.Processes[pid] = &Process{
			PID:     pid,
			Command: command,
			process: cmd.Process,
		}

		go func() {
			cmd.Wait()
			log.Info("command exited: %v", command)
			if stdout.Len() > 0 {
				log.Info(stdout.String())
			}
			if stderr.Len() > 0 {
				log.Info(stderr.String())
			}

			client.Lock()
			defer client.Unlock()
			delete(client.Processes, pid)
		}()

		return "", ""
	}

	if err := cmd.Run(); err != nil {
		log.Errorln(err)
	}
	return stdout.String(), stderr.String()
}

func kill(pid int) {
	client.Lock()
	defer client.Unlock()

	if pid == -1 {
		// Wildcard
		log.Info("killing all processes")
		for _, p := range client.Processes {
			if err := p.process.Kill(); err != nil {
				log.Errorln(err)
			}
		}

		return
	}

	log.Info("killing PID %v", pid)
	if p, ok := client.Processes[pid]; ok {
		if err := p.process.Kill(); err != nil {
			log.Errorln(err)
		}

		return
	}

	log.Error("no such process: %v", pid)
}

func killAll(needle string) {
	client.Lock()
	defer client.Unlock()

	log.Info("killing all processes matching `%v`", needle)

	for _, p := range client.Processes {
		if strings.Contains(strings.Join(p.Command, " "), needle) {
			log.Info("killing matched process: %v", p.Command)
			if err := p.process.Kill(); err != nil {
				log.Errorln(err)
			}
		}
	}
}
