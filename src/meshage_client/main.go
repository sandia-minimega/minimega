package main

import (
	"meshage"
	"fmt"
	"flag"
	"os/signal"
	"os"
	log "minilog"
)

var (
	f_addr = flag.String("addr", "", "host to connect to")
	f_degree = flag.Int("degree", 1, "graph degree")
	f_log = flag.Bool("log", false, "enable logging")
)

func main() {
	flag.Parse()

	if *f_log {
		log.AddLogger("stdout", os.Stdout, log.DEBUG, true)
	}

	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt)

	log.Debugln("starting")
	host, _ := os.Hostname()
	log.Debugln("creating node")
	n, _, errors := meshage.NewNode(host, uint(*f_degree))
	log.Debugln("starting error handler")
	go func() {
		for {
			fmt.Println(<-errors)
		}
	}()
	log.Debugln("checking for host to connect to")
	if *f_addr != "" {
		err := n.Dial(*f_addr)
		if err != nil {
			fmt.Println(err)
		}
	}

	log.Debugln("waiting on signal")
	<-sig
}

