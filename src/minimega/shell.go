package main

import (
	"os/exec"
	"bytes"
	"strings"
	log "minilog"
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
		Args:   c.Args[1:],
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("shell: %v\n", strings.Join(c.Args, " "))
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
