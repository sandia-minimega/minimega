// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"meshage"
	"minicli"
	"miniclient"
	log "minilog"
	"miniplumber"
	"os"
)

var (
	plumber *miniplumber.Plumber
)

var plumbCLIHandlers = []minicli.Handler{
	{ // plumb
		HelpShort: "plumb external programs with minimega, VMs, and other external programs",
		HelpLong:  ``,
		Patterns: []string{
			"plumb",
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
	if _, ok := c.StringArgs["src"]; !ok {
		resp.Header = []string{"pipeline"}
		resp.Tabular = [][]string{}

		for _, v := range plumber.Pipelines() {
			resp.Tabular = append(resp.Tabular, []string{v})
		}

		return nil
	} else {
		p := append([]string{c.StringArgs["src"]}, c.ListArgs["dst"]...)

		log.Debug("got plumber production: %v", p)

		return plumber.Plumb(p...)
	}
}

func cliPlumbClear(c *minicli.Command, resp *minicli.Response) error {
	if pipeline, ok := c.ListArgs["pipeline"]; ok {
		return plumber.PipelineDelete(pipeline...)
	} else {
		return plumber.PipelineDeleteAll()
	}
}

func pipeMMHandler() {
	pipe := *f_pipe

	log.Debug("got pipe: %v", pipe)

	// connect to the running minimega as a plumber
	mm, err := miniclient.Dial(*f_base)
	if err != nil {
		log.Fatalln(err)
	}

	r, w := mm.Pipe(pipe)

	go func() {
		scanner := bufio.NewScanner(r)
		for scanner.Scan() {
			_, err := os.Stdout.Write(append(scanner.Bytes(), '\n'))
			if err != nil {
				log.Fatalln(err)
			}
		}
		if err := scanner.Err(); err != nil {
			log.Fatalln(err)
		}
		os.Exit(0)
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		log.Debug("writing: %v", scanner.Text())
		_, err := w.Write(append(scanner.Bytes(), '\n'))
		if err != nil {
			log.Fatalln(err)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatalln(err)
	}

	// we can't just exit at this point, as there exists a race between
	// writing to the pipe and the other end reading and sending the data
	// over the command socket. Instead, we close the writer and wait until
	// the miniclient pipe handler exits for us.
	w.Close()

	wait := make(chan struct{})
	<-wait
}
