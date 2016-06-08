// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	log "minilog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
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
	f_tag      = flag.Bool("tag", false, "add a key value tag in minimega for this vm")
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

	if *f_tag {
		err := updateTag()
		if err != nil {
			log.Errorln(err)
		}
		return
	}

	// signal handling
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	// attempt to set up the base path
	err := os.MkdirAll(*f_path, os.FileMode(0777))
	if err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	// start a ron client
	c, err = ron.NewClient(*f_family, *f_port, *f_parent, *f_serial, *f_path)
	if err != nil {
		log.Fatal("creating ron node: %v", err)
	}

	log.Debug("starting ron client with UUID: %v", c.UUID)

	// create a listening domain socket for tag updates
	go commandSocketStart()

	<-sig
	// terminate
}

func commandSocketStart() {
	l, err := net.Listen("unix", filepath.Join(*f_path, "miniccc"))
	if err != nil {
		log.Fatalln(err)
	}

	for {
		conn, err := l.Accept()
		if err != nil {
			log.Error("command socket: %v", err)
		}
		log.Debugln("client connected")
		go commandSocketHandle(conn)
	}
}

func commandSocketHandle(conn net.Conn) {
	var err error
	var k string
	var v string

	defer conn.Close()

	dec := gob.NewDecoder(conn)

	err = dec.Decode(&k)
	if err != nil {
		log.Errorln(err)
		return
	}
	err = dec.Decode(&v)
	if err != nil {
		log.Errorln(err)
		return
	}

	log.Debug("adding key/value: %v : %v", k, v)
	c.Tag(k, v)
}

func updateTag() error {
	host := filepath.Join(*f_path, "miniccc")

	args := flag.Args()
	if len(args) != 2 {
		return fmt.Errorf("inavlid arguments: %v", args)
	}

	k := args[0]
	v := args[1]

	conn, err := net.Dial("unix", host)
	if err != nil {
		return err
	}

	enc := gob.NewEncoder(conn)

	err = enc.Encode(k)
	if err != nil {
		return err
	}

	err = enc.Encode(v)
	if err != nil {
		return err
	}

	return nil
}
