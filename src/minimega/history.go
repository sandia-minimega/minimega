// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"minicli"
	"os"
)

var historyCLIHandlers = []minicli.Handler{
	{ // history
		HelpShort: "show command history",
		Patterns: []string{
			"history",
		},
		Call: wrapSimpleCLI(cliHistory),
	},
	{ // clear history
		HelpShort: "reset history",
		HelpLong: `
Reset the command history. See "help history" for more information.`,
		Patterns: []string{
			"clear history",
		},
		Call: wrapSimpleCLI(cliHistoryClear),
	},
	{ // write
		HelpShort: "write the command history to a file",
		HelpLong: `
Write the command history to file. This is useful for handcrafting configs on
the minimega command line and then saving them for later use. Args that failed,
as well as some commands that do not impact the VM state, such as 'help', do
not get recorded.`,
		Patterns: []string{
			"write <file>",
		},
		Call: wrapSimpleCLI(cliWrite),
	},
}

func init() {
	registerHandlers("history", historyCLIHandlers)
}

func cliHistory(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{
		Host:     hostname,
		Response: minicli.History(),
	}

	return resp
}

func cliHistoryClear(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	minicli.ClearHistory()

	return resp
}

func cliWrite(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	file, err := os.Create(c.StringArgs["file"])
	if err != nil {
		resp.Error = err.Error()
		return resp
	}
	defer file.Close()

	_, err = file.WriteString(minicli.History())
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
