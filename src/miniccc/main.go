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
	"runtime"
	"syscall"
	"time"
	"version"
)

// Retry to connect for 120 minutes, fail after that
const Retries = 480
const RetryInterval = 15 * time.Second

var (
	f_port    = flag.Int("port", 9002, "port to connect to")
	f_version = flag.Bool("version", false, "print the version")
	f_parent  = flag.String("parent", "", "parent to connect to (if relay or client)")
	f_path    = flag.String("path", "/tmp/miniccc", "path to store files in")
	f_serial  = flag.String("serial", "", "use serial device instead of tcp")
	f_family  = flag.String("family", "tcp", "[tcp,unix] family to dial on")
	f_tag     = flag.Bool("tag", false, "add a key value tag in minimega for this vm")
	f_pipe    = flag.String("pipe", "", "read/write to or from a named pipe")
)

const banner = `miniccc, Copyright (2014) Sandia Corporation.
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

	log.Init()

	if *f_pipe != "" {
		pipeHandler(*f_pipe)
		return
	}

	if *f_tag {
		if runtime.GOOS == "windows" {
			log.Fatalln("tag updates are not available on windows miniccc clients")
		}

		if err := updateTag(); err != nil {
			log.Errorln(err)
		}
		return
	}

	// attempt to set up the base path
	if err := os.MkdirAll(*f_path, os.FileMode(0777)); err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	log.Debug("starting ron client with UUID: %v", client.UUID)

	if err := dial(); err != nil {
		log.Fatal("unable to connect: %v", err)
	}

	go mux()
	heartbeat() // handshake is first heartbeat

	// create a listening domain socket for tag updates
	if runtime.GOOS != "windows" {
		go commandSocketStart()
	}

	// wait for SIGTERM
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

func dial() error {
	client.Lock()
	defer client.Unlock()

	var err error

	for i := Retries; i > 0; i-- {
		if *f_serial == "" {
			log.Debug("dial: %v:%v:%v", *f_family, *f_parent, *f_port)

			var addr string
			switch *f_family {
			case "tcp":
				addr = fmt.Sprintf("%v:%v", *f_parent, *f_port)
			case "unix":
				addr = *f_parent
			default:
				log.Fatal("invalid ron dial network family: %v", *f_family)
			}

			client.conn, err = net.Dial(*f_family, addr)
		} else {
			client.conn, err = dialSerial(*f_serial)
		}

		if err == nil {
			client.enc = gob.NewEncoder(client.conn)
			client.dec = gob.NewDecoder(client.conn)
			return nil
		}

		log.Error("%v, retries = %v", err, i)
		time.Sleep(15 * time.Second)
	}

	return err
}

func updateTag() error {
	host := filepath.Join(*f_path, "miniccc")

	args := flag.Args()
	if len(args) != 2 {
		return fmt.Errorf("invalid arguments: %v", args)
	}

	k := args[0]
	v := args[1]

	conn, err := net.Dial("unix", host)
	if err != nil {
		return err
	}

	enc := gob.NewEncoder(conn)

	err = enc.Encode(MODE_TAG)
	if err != nil {
		return err
	}

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
