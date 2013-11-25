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
	log "minilog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

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
	for {
		prompt := fmt.Sprintf("minimega:%v$ ", f)
		line, err := goreadline.Rlwrap(prompt)
		if err != nil {
			return
		}
		log.Debug("got from stdin:", line)

		c := makeCommand(string(line))

		if c.Command == "disconnect" {
			log.Debugln("disconnecting")
			return
		}

		err = enc.Encode(&c)
		if err != nil {
			log.Errorln(err)
			return
		}
		log.Debugln("encoded command:", c)

		for {
			var r cliResponse
			err = dec.Decode(&r)
			if err != nil {
				if err == io.EOF {
					log.Infoln("server disconnected")
				} else {
					log.Errorln(err)
				}
				return
			}
			if r.Error != "" {
				log.Errorln(r.Error)
			}
			if r.Response != "" {
				if strings.HasSuffix(r.Response, "\n") {
					fmt.Print(r.Response)
				} else {
					fmt.Println(r.Response)
				}
			}
			if !r.More {
				log.Debugln("got last message")
				break
			} else {
				log.Debugln("expecting more data")
			}
		}
	}

}

func localCommand() {
	a := flag.Args()
	var command string
	var args []string

	log.Debugln("got args:", a)

	if len(a) > 0 {
		command = a[0]
	}
	if len(a) > 1 {
		args = a[1:]
	}

	log.Infoln("got command:", command)
	log.Infoln("got args:", args)

	// try to connect to the local minimega
	f := *f_base + "minimega"
	conn, err := net.Dial("unix", f)
	if err != nil {
		log.Fatalln(err)
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	c := cliCommand{
		Command: command,
		Args:    args,
	}
	err = enc.Encode(&c)
	if err != nil {
		log.Errorln(err)
		return
	}
	log.Debugln("encoded command:", c)

	for {
		var r cliResponse
		err = dec.Decode(&r)
		if err != nil {
			if err == io.EOF {
				log.Infoln("server disconnected")
			} else {
				log.Errorln(err)
			}
			return
		}
		if r.Error != "" {
			log.Errorln(r.Error)
		}
		if r.Response != "" {
			if strings.HasSuffix(r.Response, "\n") {
				fmt.Print(r.Response)
			} else {
				fmt.Println(r.Response)
			}
		}
		if !r.More {
			log.Debugln("got last message")
			break
		} else {
			log.Debugln("expecting more data")
		}
	}
}
