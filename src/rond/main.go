// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"minicli"
	log "minilog"
	"minipager"
	"net"
	"os"
	"path/filepath"
	"ron"

	"github.com/chzyer/readline"
)

var (
	f_port    = flag.Int("port", 9005, "port to listen on")
	f_path    = flag.String("path", "/tmp/rond", "path for files")
	f_nostdin = flag.Bool("nostdin", false, "disable reading from stdin")
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

	rond, err = ron.NewServer(*f_port, *f_path, nil)
	if err != nil {
		log.Fatal("unable to create server: %v", err)
	}

	rond.UseVMs = false

	if *f_nostdin {
		commandSocket()
	} else {
		localREPL()
	}
}

func localREPL() {
	rl, err := readline.New("rond$ ")
	if err != nil {
		log.Fatalln(err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			continue
		} else if err == io.EOF {
			break
		}

		log.Debug("got line from stdin: `%s`", line)

		resps, err := minicli.ProcessString(line, false)
		if err != nil {
			log.Errorln(err)
			continue
		}

		for resp := range resps {
			minipager.DefaultPager.Page(resp.String())

			errs := resp.Error()
			if errs != "" {
				fmt.Fprintln(os.Stderr, errs)
			}
		}
	}
}

func commandSocket() {
	l, err := net.Listen("unix", filepath.Join(*f_path, "rond"))
	if err != nil {
		log.Fatal("commandSocket: %v", err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("commandSocket: accept: %v", err)
		}
		log.Infoln("client connected")

		go func(c net.Conn) {
			defer c.Close()

			// just read comments off the wire
			scanner := bufio.NewScanner(conn)
			for scanner.Scan() {
				line := scanner.Text()

				log.Debug("got line from socket: `%v`", line)

				resps, err := minicli.ProcessString(string(line), false)
				if err != nil {
					log.Errorln(err)
					continue
				}

				for resp := range resps {
					_, err := c.Write([]byte(resp.String()))
					if err != nil {
						log.Error("unable to write response: %v", err)
						continue
					}
				}
			}
			if err := scanner.Err(); err != nil {
				log.Errorln(err)
			}
		}(conn)
	}
}
