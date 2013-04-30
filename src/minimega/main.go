// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

// TODO: vyatta: arbitrary interfaces with static ips
// TODO: vyatta: route areas
// TODO: vyatta: ipv6 support for routing and arbitrary ips
// TODO: vyatta: dhcp support
// TODO: vyatta: serial backchannel for dhcp/mac/ipv6 learning
// TODO: cli: rewrite to have namespaces style input
// TODO: cli: helper functions for common tasks (eliminate low level requirements)
// TODO: documentation and examples
// TODO: web frontend
// TODO: fix all the logging nonsense
// TODO: meshage: file transfer
// TODO: meshage/cli: timeouts and commands that never return breaking the command flow. Relax the ordering?

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
)

var (
	f_loglevel = flag.String("level", "error", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_base     = flag.String("base", "/tmp/minimega", "base path for minimega data")
	f_e        = flag.Bool("e", false, "execute command on running minimega")
	f_degree   = flag.Int("degree", 0, "meshage starting degree")
	f_port     = flag.Int("port", 8966, "meshage port to listen on")
	f_force    = flag.Bool("force", false, "force minimega to run even if it appears to already be running")
	f_nostdin  = flag.Bool("nostdin", false, "disable reading from stdin, useful for putting minimega in the background")
	vms        vmList
	signalOnce bool = false
)

var banner string = `minimega, Copyright 2012 Sandia Corporation.
minimega comes with ABSOLUTELY NO WARRANTY. This is free software, and you are
welcome to redistribute it under certain conditions. See the included LICENSE
for details.
`

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: minimega [option]... [file]...")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()
	if !strings.HasSuffix(*f_base, "/") {
		*f_base += "/"
	}

	logSetup()

	// special case, catch -e and execute a command on an already running
	// minimega instance
	if *f_e {
		localCommand()
		return
	}

	// check for a running instance of minimega
	_, err := os.Stat(*f_base + "minimega")
	if err == nil {
		if !*f_force {
			log.Fatalln("minimega appears to already be running, override with -force")
		}
		log.Warn("minimega may already be running, proceed with caution")
	}

	// set up signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		log.Info("caught signal, tearing down, ctrl-c again will force quit")
		teardown()
	}()

	r := externalCheck(cliCommand{})
	if r.Error != "" {
		log.Error("%v", r.Error)
	}

	// attempt to set up the base path
	err = os.MkdirAll(*f_base, os.FileMode(0770))
	if err != nil {
		log.Fatalln(err)
	}

	// TODO: make cli commands for KSM
	ksmSave()

	// create a node for meshage
	host, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}
	meshageInit(host, uint(*f_degree), *f_port)

	// invoke the cli
	go cliMux()
	go commandSocketStart()

	// check for a script on the command line, and invoke it as a read command
	for _, a := range flag.Args() {
		log.Infoln("reading script:", a)
		c := cliCommand{
			Command: "read",
			Args:    []string{a},
		}
		commandChanLocal <- c
		for {
			r := <-ackChanLocal
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

	if !*f_nostdin {
		cli()
	} else {
		<-sig
	}
	teardown()
}

func teardown() {
	if signalOnce {
		log.Fatal("caught signal, exiting without cleanup")
	}
	signalOnce = true
	vms.kill(-1)
	dnsmasqKill(-1)
	err := currentBridge.Destroy()
	if err != nil {
		log.Error("%v", err)
	}
	ksmRestore()
	commandSocketRemove()
	goreadline.Rlcleanup()
	os.Exit(0)
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
