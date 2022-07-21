// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"os"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

var historyCLIHandlers = []minicli.Handler{
	{ // history
		HelpShort: "show command history",
		HelpLong: `
history displays a list of all the commands that have been invoked since
minimega started on this host, or since the last time the history was cleared.
History includes only valid commands and comments. Invalid lines and blank
lines are not recorded. There are some commands that interact differently with
history, namely read. Instead of recording the "read" command in the history,
minimega records all the valid commands executed from the read file in the
history. This allows the full execution history to be listed using history.`,
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
the minimega command line and then saving them for later use.`,
		Patterns: []string{
			"write <file>",
		},
		Call: wrapSimpleCLI(cliWrite),
	},
}

func cliHistory(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	resp.Response = minicli.History()
	return nil
}

func cliHistoryClear(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	minicli.ClearHistory()
	c.Record = false
	return nil
}

func cliWrite(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	file, err := os.Create(c.StringArgs["file"])
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.WriteString(minicli.History())
	return err
}
