// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"goreadline"
	"io"
	"minicli"
	log "minilog"
	"net"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
)

type localResponse struct {
	Resp minicli.Responses
	More bool // whether there are more responses coming
}

type remoteMinimega struct {
	url string

	conn net.Conn

	enc *json.Encoder
	dec *json.Decoder
}

func NewRemoteMinimega() (*remoteMinimega, error) {
	var mm = &remoteMinimega{
		url: path.Join(*f_base, "minimega"),
	}
	var err error

	// try to connect to the local minimega
	mm.conn, err = net.Dial("unix", mm.url)
	if err != nil {
		return nil, err
	}

	mm.enc = json.NewEncoder(mm.conn)
	mm.dec = json.NewDecoder(mm.conn)

	return mm, nil
}

func localAttach() {
	// try to connect to the local minimega
	mm, err := NewRemoteMinimega()
	if err != nil {
		log.Fatalln(err)
	}

	// set up signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		log.Debug("caught signal, disconnecting")
		goreadline.Rlcleanup()
		os.Exit(0)
	}()

	// start our own rlwrap
	fmt.Println("CAUTION: calling 'quit' or 'exit' will cause the minimega daemon to exit")
	fmt.Println("use 'disconnect' or ^d to exit just the minimega command line")
	fmt.Println()
	defer goreadline.Rlcleanup()

	var exitNext bool
	for {
		prompt := fmt.Sprintf("minimega:%v$ ", mm.url)
		line, err := goreadline.Rlwrap(prompt, true)
		if err != nil {
			return
		}
		command := string(line)
		log.Debug("got from stdin: `%s`", line)

		// HAX: Shortcut some commands without using minicli
		if command == "disconnect" {
			log.Debugln("disconnecting")
			return
		} else if command == "quit" {
			if !exitNext {
				fmt.Println("CAUTION: calling 'quit' or 'exit' will cause the minimega daemon to exit")
				fmt.Println("If you really want to make the minimega daemon exit, enter quit/exit again")
				exitNext = true
				continue
			}
		}

		exitNext = false

		cmd, err := minicli.CompileCommand(command)
		if err != nil {
			log.Error("invalid command: `%s`", command)
			//fmt.Println("closest match: TODO")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		for resp := range mm.runCommand(cmd) {
			pageOutput(resp.String())
		}
	}
}

func localCommand() {
	a := flag.Args()

	log.Debugln("got args:", a)

	command := strings.Join(a, " ")

	// TODO: Need to escape?
	cmd, err := minicli.CompileCommand(command)
	if err != nil {
		log.Fatal("invalid command: `%s`", command)
	}

	if cmd == nil {
		log.Debugln("cmd is nil")
		return
	}

	log.Infoln("got command:", cmd)

	mm, err := NewRemoteMinimega()
	if err != nil {
		log.Fatalln(err)
	}

	for resp := range mm.runCommand(cmd) {
		output := resp.String()
		if output != "" {
			fmt.Println(output)
		}
	}
}

// runCommand runs a command through a JSON pipe.
func (mm *remoteMinimega) runCommand(cmd *minicli.Command) chan minicli.Responses {
	err := mm.enc.Encode(*cmd)
	if err != nil {
		log.Errorln("local command gob encode: %v", err)
		return nil
	}
	log.Debugln("encoded command:", cmd)

	respChan := make(chan minicli.Responses)

	go func() {
		defer close(respChan)

		for {
			var r localResponse
			err = mm.dec.Decode(&r)
			if err != nil {
				if err == io.EOF {
					log.Infoln("server disconnected")
					return
				}

				log.Errorln("local command gob decode: %v", err)
				return
			}

			respChan <- r.Resp
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}()

	return respChan
}
