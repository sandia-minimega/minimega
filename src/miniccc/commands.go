// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	log "minilog"
	"os/exec"
	"path/filepath"
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
		resp.Stdout, resp.Stderr = runCommand(cmd.Stdin, cmd.Stdout, cmd.Stderr, cmd.Command, cmd.Background)
	}

	if len(cmd.FilesRecv) != 0 {
		sendFiles(cmd.ID, cmd.FilesRecv)
	}

	appendResponse(resp)
}

// lookPath wraps exec.LookPath to check $PATH and the files path
func lookPath(file string) (string, error) {
	path, err := exec.LookPath(file)
	if err == nil {
		return path, nil
	}

	// file contains a slash, shouldn't search files path
	if strings.Contains(file, "/") {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}

	file = filepath.Join(*f_path, "files", file)
	return exec.LookPath(file)
}

func runCommand(stdin, stdout, stderr string, command []string, background bool) (string, string) {
	done := make(chan struct{})
	var bufout, buferr bytes.Buffer

	path, err := lookPath(command[0])
	if err != nil {
		log.Errorln(err)
		close(done)
		return "", err.Error()
	}

	cmd := &exec.Cmd{
		Path: path,
		Args: command,
	}

	if stdin != "" {
		pStdin, err := cmd.StdinPipe()
		if err != nil {
			log.Errorln(err)
			return "", ""
		}

		cStdin, err := NewPlumberReader(stdin)
		if err != nil {
			log.Errorln(err)
			return "", ""
		}

		go func() {
			<-done
			cStdin.Close()
		}()

		go func() {
			for v := range cStdin.C {
				_, err := pStdin.Write([]byte(v))
				if err != nil {
					log.Errorln(err)
					return
				}
			}
			pStdin.Close()
		}()
	}

	if stdout != "" {
		pStdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Errorln(err)
			close(done)
			return "", ""
		}

		cStdout, err := NewPlumberWriter(stdout)
		if err != nil {
			log.Errorln(err)
			close(done)
			return "", ""
		}

		go func() {
			defer close(cStdout)
			scanner := bufio.NewScanner(pStdout)
			for scanner.Scan() {
				select {
				case cStdout <- scanner.Text() + "\n":
				case <-done:
					return
				}
			}
			if err := scanner.Err(); err != nil {
				log.Errorln(err)
				return
			}
		}()
	} else {
		cmd.Stdout = &bufout
	}

	if stderr != "" {
		pStderr, err := cmd.StderrPipe()
		if err != nil {
			log.Errorln(err)
			close(done)
			return "", ""
		}

		cStderr, err := NewPlumberWriter(stderr)
		if err != nil {
			log.Errorln(err)
			close(done)
			return "", ""
		}

		go func() {
			defer close(cStderr)
			scanner := bufio.NewScanner(pStderr)
			for scanner.Scan() {
				select {
				case cStderr <- scanner.Text() + "\n":
				case <-done:
					return
				}
			}
			if err := scanner.Err(); err != nil {
				log.Errorln(err)
				return
			}
		}()
	} else {
		cmd.Stderr = &buferr
	}

	log.Info("executing: %v", command)

	if background {
		log.Debug("starting in background")
		if err := cmd.Start(); err != nil {
			log.Errorln(err)
			return "", buferr.String()
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
			if bufout.Len() > 0 {
				log.Info(bufout.String())
			}
			if buferr.Len() > 0 {
				log.Info(buferr.String())
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
	return bufout.String(), buferr.String()
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
