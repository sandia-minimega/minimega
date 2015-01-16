// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"minicli"
	log "minilog"
	"os/exec"
	"strings"
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
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliShell(c, false)
		}),
	},
	{ // background
		HelpShort: "execute a command in the background",
		HelpLong: `
Execute a command under the credentials of the running user.

Commands run in the background and control returns immediately. Any output is
logged.`,
		Patterns: []string{
			"background <command>...",
		},
		Call: wrapSimpleCLI(func(c *minicli.Command) *minicli.Response {
			return cliShell(c, true)
		}),
	},
}

func init() {
	registerHandlers("shell", shellCLIHandlers)
}

func cliShell(c *minicli.Command, background bool) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	var sOut bytes.Buffer
	var sErr bytes.Buffer

	command := strings.Join(c.ListArgs["command"], " ")

	p, err := exec.LookPath(c.ListArgs["command"][0])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	fields := fieldsQuoteEscape("\"", command)

	cmd := &exec.Cmd{
		Path:   p,
		Args:   fields,
		Env:    nil,
		Dir:    "",
		Stdout: &sOut,
		Stderr: &sErr,
	}
	log.Info("starting: %v", command)
	err = cmd.Start()
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	if background {
		go func() {
			err = cmd.Wait()
			if err != nil {
				log.Error(err.Error())
				return
			}

			log.Info("command %v exited", command)
			log.Info(sOut.String())
			log.Info(sErr.String())
		}()
	} else {
		err = cmd.Wait()
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		resp.Response = sOut.String()
		resp.Error = sErr.String()
	}

	return resp
}
