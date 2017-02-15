// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	"goreadline"
	"minicli"
	log "minilog"
	"minipager"
	"os"
	"ron"
)

var (
	f_port = flag.Int("port", 9005, "port to listen on")
	f_path = flag.String("path", "/tmp/rond", "path for files")
)

var (
	rond     *ron.Server
	hostname string
)

func main() {
	flag.Parse()

	log.Init()

	// register CLI handlers
	for i := range cliHandlers {
		err := minicli.Register(&cliHandlers[i])
		if err != nil {
			log.Fatal("invalid handler, `%v` -- %v", cliHandlers[i].HelpShort, err)
		}
	}

	var err error
	hostname, err = os.Hostname()
	if err != nil {
		log.Fatal("unable to get hostname: %v", hostname)
	}

	rond, err = ron.NewServer(*f_port, *f_path)
	if err != nil {
		log.Fatal("unable to create server: %v", err)
	}

	rond.UseVMs = false

	for {
		line, err := goreadline.Readline("rond$ ", true)
		if err != nil {
			return
		}
		command := string(line)
		log.Debug("got from stdin: `%s`", line)

		cmd, err := minicli.Compile(command)
		if err != nil {
			log.Error("%v", err)
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		for resp := range minicli.ProcessCommand(cmd) {
			minipager.DefaultPager.Page(resp.String())

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}
