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
	"strings"
	"syscall"
	"version"
)

const (
	BASE_PATH = "/tmp/miniccc"
)

var (
	f_loglevel = flag.String("level", "warn", "set log level: [debug, info, warn, error, fatal]")
	f_log      = flag.Bool("v", true, "log on stderr")
	f_logfile  = flag.String("logfile", "", "also log to file")
	f_port     = flag.Int("port", 8967, "port to listen on")
	f_version  = flag.Bool("version", false, "print the version")
	f_role     = flag.String("role", "client", "role [master,relay,client]")
	f_parent   = flag.String("parent", "", "parent to connect to (if relay or client)")
	f_base     = flag.String("base", BASE_PATH, "directory to serve files from")
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
	if !strings.HasSuffix(*f_base, "/") {
		*f_base += "/"
	}

	if *f_version {
		fmt.Println("miniccc", version.Revision, version.Date)
		fmt.Println(version.Copyright)
		os.Exit(0)
	}

	logSetup()

	// signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	// attempt to set up the base path
	log.Debugln("make base directories")
	err := os.MkdirAll(*f_base, os.FileMode(0770))
	if err != nil {
		log.Fatalln(err)
	}
	err = os.MkdirAll((*f_base)+"files", os.FileMode(0770))
	if err != nil {
		log.Fatalln(err)
	}
	err = os.MkdirAll((*f_base)+"responses", os.FileMode(0770))
	if err != nil {
		log.Fatalln(err)
	}

	// start a ron node
	var r *ron.Ron
	switch *f_role {
	case "client":
		log.Debugln("starting in client mode")
		r, err = ron.New(ron.MODE_CLIENT, *f_parent, *f_port)
		if err != nil {
			log.Fatalln(err)
		}
	case "relay":
		log.Debugln("starting in relay mode")
		r, err = ron.New(ron.MODE_RELAY, *f_parent, *f_port)
		if err != nil {
			log.Fatalln(err)
		}
	case "master":
		log.Debugln("starting in master mode")
		r, err = ron.New(ron.MODE_MASTER, "", *f_port)
		if err != nil {
			log.Fatalln(err)
		}
	default:
		log.Fatal("invalid role %v", *f_role)
	}

	fmt.Println("%x", r)
	<-sig
	// terminate
}
