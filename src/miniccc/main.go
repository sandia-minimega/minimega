// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"flag"
	"fmt"
	log "minilog"
	"os"
	"os/signal"
	"ron"
	"syscall"
	"version"
)

var (
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_port     = flag.Int("port", 9002, "port to connect to")
	f_version  = flag.Bool("version", false, "print the version")
	f_parent   = flag.String("parent", "", "parent to connect to (if relay or client)")
	f_path     = flag.String("path", "/tmp/miniccc", "path to store files in")
	f_serial   = flag.String("serial", "", "use serial device instead of tcp")
	f_family   = flag.String("family", "tcp", "[tcp,unix] family to dial on")
	c          *ron.Client
)

var banner string = `miniccc, Copyright (2014) Sandia Corporation. 
Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation, 
the U.S. Government retains certain rights in this software.
`

func usage() {
	fmt.Println(banner)
	fmt.Println("usage: miniccc [option]... ")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if *f_version {
		fmt.Println("miniccc", version.Revision, version.Date)
		fmt.Println(version.Copyright)
		os.Exit(0)
	}

	logSetup()

	// signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	// start a ron client
	var err error
	c, err = ron.NewClient(*f_family, *f_port, *f_parent, *f_serial, *f_path)
	if err != nil {
		log.Fatal("creating ron node: %v", err)
	}

	log.Debug("starting ron client with UUID: %v", c.UUID)

	<-sig
	// terminate
}
