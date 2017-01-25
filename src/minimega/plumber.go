// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"meshage"
	"minicli"
	"miniclient"
	log "minilog"
	"miniplumber"
)

var (
	plumber *miniplumber.Plumber
)

var plumbCLIHandlers = []minicli.Handler{
	{ // plumb
		HelpShort: "plumb external programs with minimega, VMs, and other external programs",
		HelpLong:  ``,
		Patterns: []string{
			"plumb <src> <dst>...",
		},
		Call: wrapSimpleCLI(cliPlumb),
	},
	{
		HelpShort: "reset plumber state",
		HelpLong:  ``,
		Patterns: []string{
			"clear plumb [pipeline]...",
		},
		Call: wrapBroadcastCLI(cliPlumbClear),
	},
}

func plumberStart(node *meshage.Node) {
	plumber = miniplumber.New(node)
}

func cliPlumb(c *minicli.Command, resp *minicli.Response) error {
	p := append([]string{c.StringArgs["src"]}, c.ListArgs["dst"]...)

	log.Debug("got plumber production: %v", p)

	return plumber.Plumb(p...)
}

func cliPlumbClear(c *minicli.Command, resp *minicli.Response) error {
	if pipeline, ok := c.ListArgs["pipeline"]; ok {
		return plumber.PipelineDelete(pipeline...)
	} else {
		return plumber.PipelineDeleteAll()
	}
}

func pipeMMHandler() {
	var pipe string
	var value string

	if flag.NArg() != 1 && flag.NArg() != 2 {
		log.Fatalln("-pipe requires exactly one or two arguments")
	}

	pipe = flag.Arg(0)
	log.Debug("got pipe: %v", pipe)

	if flag.NArg() == 2 {
		value = flag.Arg(1)
		log.Debug("got pipe write value: %v", value)
	}

	// connect to the running minimega as a plumber
	mm, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatalln(err)
	}

	for resp := range mm.Pipe(pipe, value) {
		if resp.Rendered != "" {
			fmt.Println(resp.Rendered)
		}
	}

	return
}
