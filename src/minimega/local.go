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
	"strings"
	"syscall"
)

type localResponse struct {
	Resp minicli.Responses
	More bool // whether there are more responses coming
}

func localAttach() {
	// try to connect to the local minimega
	f := *f_base + "minimega"
	conn, err := net.Dial("unix", f)
	if err != nil {
		log.Fatalln(err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

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
		prompt := fmt.Sprintf("minimega:%v$ ", f)
		line, err := goreadline.Rlwrap(prompt)
		if err != nil {
			return
		}
		command := string(line)
		log.Debug("got from stdin:", line)

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
			fmt.Println("closest match: TODO")
			continue
		}

		// No command was returned, must have been a blank line or a comment
		// line. Either way, don't try to run a nil command.
		if cmd == nil {
			continue
		}

		err = sendLocalCommand(enc, dec, cmd)
		if err != nil {
			log.Errorln(err)
			return
		}
	}
}

func localCommand() {
	a := flag.Args()

	log.Debugln("got args:", a)

	// TODO: Need to escape?
	cmd, err := minicli.CompileCommand(strings.Join(a, " "))
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Infoln("got command:", cmd)

	// try to connect to the local minimega
	f := *f_base + "minimega"
	conn, err := net.Dial("unix", f)
	if err != nil {
		log.Fatalln(err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	err = sendLocalCommand(enc, dec, cmd)
	if err != nil {
		log.Errorln(err)
	}
}

// sendLocalCommand runs a command through a JSON pipe established elsewhere.
// Prints the responses to stdout.
func sendLocalCommand(enc *json.Encoder, dec *json.Decoder, cmd *minicli.Command) error {
	err := enc.Encode(*cmd)
	if err != nil {
		return fmt.Errorf("local command gob encode: %v", err)
	}
	log.Debugln("encoded command:", cmd)

	for {
		var r localResponse
		err = dec.Decode(&r)
		if err != nil {
			if err == io.EOF {
				log.Infoln("server disconnected")
				return nil
			}

			err = fmt.Errorf("local command gob decode: %v", err)
			return err
		}

		fmt.Println(r.Resp)
		if !r.More {
			log.Debugln("got last message")
			break
		} else {
			log.Debugln("expecting more data")
		}
	}

	return nil
}
