// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// +build linux

package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	log "minilog"
	"version"
)

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

	// init client
	NewClient()

	if *f_pipe != "" {
		pipeHandler(*f_pipe)
		return
	}

	if *f_tag {
		if err := updateTag(); err != nil {
			log.Errorln(err)
		}

		return
	}

	// attempt to set up the base path
	if err := os.MkdirAll(*f_path, os.FileMode(0777)); err != nil {
		log.Fatal("mkdir base path: %v", err)
	}

	log.Info("starting ron client with UUID: %v", client.UUID)

	pidPath := filepath.Join(*f_path, "miniccc.pid")

	// try to find existing miniccc process
	data, err := ioutil.ReadFile(pidPath)
	if err == nil {
		pid, err := strconv.Atoi(string(data))
		if err == nil {
			log.Info("search for miniccc pid: %v", pid)
			if processExists(pid) {
				log.Fatal("miniccc already running")
			}
			log.Info("process not found")
		}
	}

	// write PID file
	pid := strconv.Itoa(os.Getpid())
	if err := ioutil.WriteFile(pidPath, []byte(pid), 0664); err != nil {
		log.Fatal("write pid failed: %v", err)
	}

	defer os.Remove(pidPath)

	resetClient()

	// path for tag update
	udsPath := filepath.Join(*f_path, "miniccc")

	// clean up defunct state
	os.Remove(udsPath)

	go commandSocketStart(udsPath)

	// wait for SIGTERM
	sig := make(chan os.Signal, 1024)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
}

var done chan struct{}

// called by mux if loss of server is detected
func resetClient() {
	if done != nil {
		// Stop periodic heartbeat and command handler (started by mux) so they can
		// be safely restarted again below.
		close(done)
		done = nil
	}

	// If called by mux due to loss of server connection, clean up shop and try
	// again since the server side will detect a loss of connection and reset as
	// well.
	if client.conn != nil {
		client.conn.Close()

		client.enc = nil
		client.dec = nil
		client.conn = nil
	}

	if err := dial(); err != nil {
		log.Fatal("unable to connect: %v", err)
	}

	done = make(chan struct{})

	go mux(done)
	heartbeat() // handshake is first heartbeat
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
