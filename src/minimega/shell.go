package main

import (
	"bytes"
	log "minilog"
	"os/exec"
	"strings"
)

func shellCLI(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Error: "shell takes one or more arguments",
		}
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p, err := exec.LookPath(c.Args[0])
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	cmd := &exec.Cmd{
		Path:   p,
		Args:   c.Args,
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("shell: %v", strings.Join(c.Args, " "))
	err = cmd.Run()
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	return cliResponse{
		Response: sOut.String(),
		Error:    sErr.String(),
	}
}

func backgroundCLI(c cliCommand) cliResponse {
	if len(c.Args) == 0 {
		return cliResponse{
			Error: "shell takes one or more arguments",
		}
	}

	var sOut bytes.Buffer
	var sErr bytes.Buffer
	p, err := exec.LookPath(c.Args[0])
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	cmd := &exec.Cmd{
		Path:   p,
		Args:   c.Args,
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("shell: %v", strings.Join(c.Args, " "))
	err = cmd.Start()
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}

	go func() {
		cmd.Wait()
		log.Info("command %v exited", strings.Join(c.Args, " "))
		log.Info(sOut.String())
		log.Info(sErr.String())
	}()

	return cliResponse{}
}
