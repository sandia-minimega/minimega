// minimega
// 
// Copyright (2012) Sandia Corporation. 
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
// the U.S. Government retains certain rights in this software.
//
// David Fritz <djfritz@sandia.gov>

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
	f_loglevel  = flag.String("level", "error", "set log level: [debug, info, warn, error, fatal]")
	f_log       = flag.Bool("v", true, "log on stderr")
	f_logfile   = flag.String("logfile", "", "also log to file")
	f_base      = flag.String("base", "/tmp/minimega", "base path for minimega data")
	f_e         = flag.Bool("e", false, "execute command on running minimega")
	f_degree    = flag.Int("degree", 0, "meshage starting degree")
	f_port      = flag.Int("port", 8966, "meshage port to listen on")
	f_force	= flag.Bool("force", false, "force minimega to run even if it appears to already be running")
	vms         vm_list
	signal_once bool = false
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

	log_setup()

	// special case, catch -e and execute a command on an already running
	// minimega instance
	if *f_e {
		local_command()
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

	r := external_check(cli_command{})
	if r.Error != "" {
		log.Error("%v", r.Error)
	}

	// attempt to set up the base path
	err = os.MkdirAll(*f_base, os.FileMode(0770))
	if err != nil {
		log.Fatalln(err)
	}

	ksm_save()

	// create a node for meshage
	host, err := os.Hostname()
	if err != nil {
		log.Fatalln(err)
	}
	meshageInit(host, uint(*f_degree), *f_port)

	// invoke the cli
	go cli_mux()
	go command_socket_start()

	// check for a script on the command line, and invoke it as a read command
	for _, a := range flag.Args() {
		log.Infoln("reading script:", a)
		c := cli_command{
			Command: "read",
			Args:    []string{a},
		}
		command_chan_local <- c
		for {
			r := <-ack_chan_local
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

	cli()
	teardown()
}

func teardown() {
	if signal_once {
		log.Fatal("caught signal, exiting without cleanup")
	}
	signal_once = true
	vms.kill(-1)
	dhcpKill(-1)
	err := current_bridge.Destroy()
	if err != nil {
		log.Error("%v", err)
	}
	ksm_restore()
	command_socket_remove()
	goreadline.Rlcleanup()
	os.Exit(0)
}

func local_command() {
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

	c := cli_command{
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
		var r cli_response
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
