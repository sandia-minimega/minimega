// Copyright (2014) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	log "minilog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
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

	// init client
	NewClient()

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

	log.Info("starting ron client with UUID: %v", client.UUID)

	if runtime.GOOS != "windows" {
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
	}

	if err := dial(); err != nil {
		log.Fatal("unable to connect: %v", err)
	}

	go mux()
	heartbeat() // handshake is first heartbeat

	// create a listening domain socket for tag updates
	if runtime.GOOS != "windows" {
		// path for tag update
		udsPath := filepath.Join(*f_path, "miniccc")

		// clean up defunct state
		os.Remove(udsPath)

		go commandSocketStart(udsPath)
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

			// Server-side, we're now reacting to async QMP VSERPORT_CHANGE events for
			// each VM to determine when we should close and connect to the virtual
			// serial port. The call to `dialSerial` above will trigger a
			// VSERPORT_CHANGE event, at which point the server will connect to the
			// virtual serial port and wait to hear from the client. We sleep for a
			// bit here to give the server time to receive the event and connect to
			// the virtual serial port before sending he initial magic bytes message
			// below.
			time.Sleep(1 * time.Second)
		}

		// write magic bytes
		if err == nil {
			_, err = io.WriteString(client.conn, "RON")
		}

		// read until we see the magic bytes back
		var buf [3]byte
		for err == nil && string(buf[:]) != "RON" {
			// shift the buffer
			buf[0] = buf[1]
			buf[1] = buf[2]
			// read the next byte
			_, err = client.conn.Read(buf[2:])
		}

		if err == nil {
			client.enc = gob.NewEncoder(client.conn)
			client.dec = gob.NewDecoder(client.conn)
			return nil
		}

		log.Error("%v, retries = %v", err, i)

		// It's possible that we could have an error after the client connection has
		// been created. For example, when using the serial port, writing the magic
		// `RON` bytes can result in an EOF if the host has been rebooted and the
		// minimega server hasn't cleaned up and reconnected to the virtual serial
		// port yet. In such a case, the connection needs to be closed to avoid a
		// "device busy" error when trying to dial it again.
		if client.conn != nil {
			client.conn.Close()
		}

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
