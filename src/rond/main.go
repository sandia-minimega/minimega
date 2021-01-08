// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
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
	"strings"

	"github.com/peterh/liner"
)

var (
	f_port    = flag.Int("port", 9005, "port to listen on")
	f_path    = flag.String("path", "/tmp/rond", "path for files")
	f_nostdin = flag.Bool("nostdin", false, "disable reading from stdin")

	f_e = flag.Bool("e", false, "execute command on running rond")
)

var (
	rond     *ron.Server
	hostname string
)

func main() {
	flag.Parse()

	log.Init()

	if *f_e {
		rond, err := Dial(filepath.Join(*f_path, "rond"))
		if err != nil {
			log.Fatal("unable to dial: %v", err)
		}
		rond.Pager = minipager.DefaultPager

		// TODO: Need to escape?
		cmd := strings.Join(flag.Args(), " ")
		log.Info("got command: `%v`", cmd)

		rond.RunAndPrint(cmd, false)
		return
	}

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

	rond, err = ron.NewServer(*f_path, "", nil)
	if err != nil {
		log.Fatal("unable to create server: %v", err)
	}

	if err := rond.Listen(*f_port); err != nil {
		log.Fatal("unable to listen: %v", err)
	}
	rond.UseVMs = false

	if *f_nostdin {
		commandSocket()
	} else {
		localREPL()
	}
}

func localREPL() {
	input := liner.NewLiner()
	defer input.Close()

	input.SetCtrlCAborts(true)
	input.SetTabCompletionStyle(liner.TabPrints)

	for {
		line, err := input.Prompt("rond$ ")
		if err == liner.ErrPromptAborted {
			continue
		} else if err == io.EOF {
			break
		}

		log.Debug("got line from stdin: `%s`", line)
		input.AppendHistory(line)

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

			enc := json.NewEncoder(c)
			dec := json.NewDecoder(c)

			var err error

			for err == nil {
				var r Request

				if err = dec.Decode(&r); err != nil {
					log.Errorln(err)
					return
				}

				log.Debug("got commands from socket: `%v`", r.Command)

				resps, err := minicli.ProcessString(r.Command, false)
				if err != nil {
					log.Errorln(err)
					return
				}

				// HAX: Work around so that we can add the more boolean.
				var prev minicli.Responses

				// Keep sending until we hit the first error, then just consume the
				// channel to ensure that we release any locks acquired by cmd.
				for resp := range resps {
					if prev != nil && err == nil {
						err = sendLocalResp(enc, prev, true)
					} else if err != nil && len(resp) > 0 {
						log.Info("dropping resp from %v", resp[0].Host)
					}

					prev = resp
				}

				if err == nil {
					err = sendLocalResp(enc, prev, false)
				}
			}
		}(conn)
	}
}

func sendLocalResp(enc *json.Encoder, resp minicli.Responses, more bool) error {
	r := Response{
		More: more,
	}
	if resp != nil {
		r.Resp = resp
		r.Rendered = resp.String()
	}

	return enc.Encode(&r)
}
