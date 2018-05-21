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
	log "minilog"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// Constants

// The length of a single time-slice in the schedule.
// Must be less than 60! 1, 5, 10, or 15 are good choices
// Shorter length means less waiting for reservations to start, but bigger schedule files.
const MINUTES_PER_SLICE = 1

// Minimum schedule length in minutes, 720 = 12 hours
const MIN_SCHED_LEN = 720

// Global Variables
// This flag can be set regardless of which subcommand is executed
var configpath = flag.String("config", "/etc/igor.conf", "Path to configuration file")

// The configuration we read in from the file
var igorConfig Config

// Our most important data structures: the reservation list, and the schedule
var Reservations map[uint64]Reservation // map ID to reservations
var Schedule []TimeSlice                // The schedule

// dirty is set by the command handlers when the schedule is changed so we know
// if we need to write it out or not.
var dirty bool

// Commands lists the available commands and help topics.
// The order here is the order in which they are printed by 'go help'.
var commands = []*Command{
	cmdDel,
	cmdShow,
	cmdSub,
	cmdPower,
	cmdExtend,
	cmdNotify,
}

var exitStatus = 0
var exitMu sync.Mutex

// Represents a slice of time in the Schedule
type TimeSlice struct {
	Start int64    // UNIX time
	End   int64    // UNIX time
	Nodes []uint64 // slice of len(# of nodes), mapping to reservation IDs
}

// Sort the slice of reservations based on the start time
type StartSorter []Reservation

func (s StartSorter) Len() int {
	return len(s)
}

func (s StartSorter) Less(i, j int) bool {
	return s[i].StartTime < s[j].StartTime
}

func (s StartSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func setExitStatus(n int) {
	exitMu.Lock()
	if exitStatus < n {
		exitStatus = n
	}
	exitMu.Unlock()
}

// Runs at startup to handle automated tasks that need to happen now.
// Read the reservations, delete any that are too old.
// Copy in netboot files for any reservations that have just started
func housekeeping() {
	now := time.Now().Unix()

	backend := GetBackend()

	for _, r := range Reservations {
		// Check if $TFTPROOT/pxelinux.cfg/igor/ResName exists. This is how we verify if the reservation is installed or not
		if r.EndTime < now {
			// Reservation expired; delete it
			deleteReservation(false, []string{r.ResName})

			dirty = true
		} else if _, err := os.Stat(r.Filename()); os.IsNotExist(err) && r.StartTime < now {
			// Reservation should have started but has not yet been installed
			emitReservationLog("INSTALL", r)
			// update network config
			err := networkSet(r.Hosts, r.Vlan)
			if err != nil {
				log.Error("error setting network isolation: %v", err)
			}

			if err := backend.Install(r); err != nil {
				log.Fatal("unable to install: %v", err)
			}

			if igorConfig.AutoReboot {
				if err := backend.Power(r.Hosts, false); err != nil {
					log.Fatal("unable to power off: %v", err)
				}

				if err := backend.Power(r.Hosts, true); err != nil {
					log.Fatal("unable to power on: %v", err)
				}
			}

			dirty = true
		}
	}

	// Remove expired time slices from the schedule
	expireSchedule()
}

func init() {
	Reservations = make(map[uint64]Reservation)
}

func main() {
	var err error

	log.Init()

	flag.Usage = usage
	flag.Parse()

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

	rand.Seed(time.Now().UnixNano())

	igorConfig = readConfig(*configpath)

	// Add another logger for the logfile, if set
	if igorConfig.LogFile != "" {
		logfile, err := os.OpenFile(igorConfig.LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0660)
		if err != nil {
			log.Fatal("failed to create logfile %v: %v", igorConfig.LogFile, err)
		}
		log.AddLogger("file", logfile, log.INFO, false)
	}

	// Read in the reservations and schedule
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/data.gob")
	db, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open data file %v: %v", path, err)
	}
	defer db.Close()

	// Lock the file so we don't have simultaneous updates
	if err := syscall.Flock(int(db.Fd()), syscall.LOCK_EX); err != nil {
		log.Fatal("unable to lock schedule file -- please retry")
	}
	// unlock at the end later
	defer syscall.Flock(int(db.Fd()), syscall.LOCK_UN)
	readData(db)

	// Here, we need to go through and delete any reservations which should be
	// expired, and bring in new ones that are just starting
	housekeeping()

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

			if dirty {
				writeData(db)
				writeReservations()
			}

			return
		}
	}

	fmt.Fprintf(os.Stderr, "go: unknown subcommand %q\nRun 'go help' for usage.\n", args[0])
	setExitStatus(2)
}
