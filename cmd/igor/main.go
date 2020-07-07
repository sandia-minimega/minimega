// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

// igor is a simple command line tool for managing reservations of nodes in a
// cluster. It also will configure the pxeboot environment for booting kernels
// and initrds for cluster nodes.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Global Variables
// This flag can be set regardless of which subcommand is executed
var configpath = flag.String("config", "/etc/igor.conf", "Path to configuration file")

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'go help'.
var commands = []*Command{
	cmdDel,
	cmdShow,
	cmdStats,
	cmdSub,
	cmdPower,
	cmdExtend,
	cmdNotify,
	cmdSync,
	cmdEdit,
}

var exitStatus = 0
var exitMu sync.Mutex

func setExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

func main() {
	var err error

	flag.Usage = usage
	flag.Parse()

	log.Init()

	args := flag.Args()
	if len(args) < 1 {
		usage()
	}

	if args[0] == "help" {
		help(args[1:])
		return
	}

	if args[0] == "version" {
		printVersion()
		return
	}

	igor.Config = readConfig(*configpath)

	// Quit immediately if igor is paused
	if igor.Pause != "" {
		log.Fatal(igor.Pause)
	}

	// Add another logger for the logfile, if set
	if igor.LogFile != "" {
		logfile, err := os.OpenFile(igor.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			log.Fatal("failed to create logfile %v: %v", igor.LogFile, err)
		}
		log.AddLogger("file", logfile, log.INFO, false)
	}

	// Make sure that we are running with effective UID of igor. This ensures
	// that we write files that we can read later.
	u, err := user.LookupId(strconv.Itoa(os.Geteuid()))
	if err != nil {
		log.Fatal("unable to get effective uid: %v", err)
	} else if u.Username != "igor" {
		log.Fatal("effective uid must be igor and not %v", u.Username)
	}

	// Look up the user so that we can attribute actions.
	igor.User, err = getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	if igor.UseCobbler {
		igor.Backend = NewCobblerBackend()
	} else {
		igor.Backend = NewTFTPBackend()
	}

	igor.Now = time.Now()

	// Seed the random number based on the current time
	rand.Seed(igor.Now.UnixNano())

	// We open and lock the lock file before trying to open the data file
	// because the data file may have been changed by the instance of igor that
	// holds the lock.
	lockPath := filepath.Join(igor.TFTPRoot, "/igor/lock")
	dataPath := filepath.Join(igor.TFTPRoot, "/igor/reservations.json")

	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open data file %v: %v", lockPath, err)
	}
	defer lock.Close()

	// Lock the file so we don't have simultaneous updates
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		log.Fatal("unable to lock file -- please retry")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	db, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open data file %v: %v", dataPath, err)
	}
	defer db.Close()

	if err := igor.readData(db); err != nil {
		log.Fatalln(err)
	}

	// Here, we need to go through and delete any reservations which should be
	// expired, and bring in new ones that are just starting
	if err := igor.Housekeeping(); err != nil {
		log.Fatalln(err)
	}

	// Now process the command
	for _, cmd := range commands {
		if cmd.Name() == args[0] && cmd.Run != nil {
			cmd.Flag.Usage = func() { cmd.Usage() }
			if cmd.CustomFlags {
				args = args[1:]
			} else {
				cmd.Flag.Parse(args[1:])
				args = cmd.Flag.Args()
			}
			cmd.Run(cmd, args)

			if igor.dirty {
				log.Info("writing data file")
				if err := igor.writeData(db); err != nil {
					log.Fatalln(err)
				}
			}

			return
		}
	}

	fmt.Fprintf(os.Stderr, "igor: unknown subcommand %q\nRun 'igor help' for usage.\n", args[0])
	setExitStatus(2)
}
