package main

import (
	"flag"
	"fmt"
	"goreadline"
	"meshage"
	log "minilog"
	"os"
	"os/signal"
)

var (
	f_addr   = flag.String("addr", "", "host to connect to")
	f_degree = flag.Int("degree", 1, "graph degree")
	f_log    = flag.Bool("log", false, "enable logging")
	f_b	= flag.Bool("bg", true, "don't start a cli, just wait to be killed")
	n        *meshage.Node
)

func main() {
	flag.Parse()

	if *f_log {
		log.AddLogger("stdout", os.Stdout, log.DEBUG, true)
	}

	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		teardown()
	}()

	log.Debugln("starting")
	host, _ := os.Hostname()
	log.Debugln("creating node")
	errors := make(chan error)
	var m chan *meshage.Message
	n, m, errors = meshage.NewNode(host, uint(*f_degree), 8966)
	log.Debugln("starting error handler")
	go func() {
		for {
			fmt.Println(<-errors)
		}
	}()
	log.Debugln("starting message handler")
	go messageHandler(m)
	log.Debugln("checking for host to connect to")
	if *f_addr != "" {
		n.Dial(*f_addr)
	}

	if *f_b {
		<-sig
	} else {
		cli()
	}
	teardown()
}

func teardown() {
	goreadline.Rlcleanup()
	fmt.Println()
	os.Exit(0)
}
