// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bytes"
	"os/exec"

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
			return cliShell(c, resp, false)
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
		Call: wrapSimpleCLI(func(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
			return cliShell(c, resp, true)
		}),
	},
}

func cliShell(c *minicli.Command, resp *minicli.Response, background bool) error {
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

	if background {
		go func() {
			if err := cmd.Wait(); err != nil {
				log.Error(err.Error())
				return
			}

			log.Info("command %v exited", args)
			if out := sOut.String(); out != "" {
				log.Info(out)
			}
			if err := sErr.String(); err != "" {
				log.Info(err)
			}
		}()

		return nil
	}

	if err = cmd.Wait(); err != nil {
		return err
	}

	resp.Response = sOut.String()
	resp.Error = sErr.String()

	return nil
}
