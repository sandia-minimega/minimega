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
	"os/user"
	"path/filepath"
	"strconv"
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
	cmdSync,
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
		if r.EndTime < now {
			// Reservation expired; delete it
			deleteReservation(false, []string{r.ResName})

			dirty = true
		} else if r.StartTime >= now {
			// Reservation is in the future, ignore for now
			continue
		}

		// already installed
		if r.Installed {
			continue
		}

		// check to see if we need to install the reservation
		if _, err := os.Stat(r.Filename()); err == nil {
			// also already installed
			log.Info("%v is already installed", r.ResName)

			r.Installed = true
			// TODO: make Reservations map[int]*Reservation
			Reservations[r.ID] = r
			dirty = true

			continue
		}

		// Reservation has started but has not yet been installed
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

		r.Installed = true
		// TODO: make Reservations map[int]*Reservation
		Reservations[r.ID] = r

		dirty = true
	}

	// Remove expired time slices from the schedule
	expireSchedule()
}

func init() {
	Reservations = make(map[uint64]Reservation)
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

	// Make sure that we are running with effective UID of igor. This ensures
	// that we write files that we can read later.
	u, err := user.LookupId(strconv.Itoa(os.Geteuid()))
	if err != nil {
		log.Fatal("unable to get effective uid: %v", err)
	} else if u.Username != "igor" {
		log.Fatal("effective uid must be igor and not %v", u.Username)
	}

	// We open and lock the lock file before trying to open the data file
	// because the data file may have been changed by the instance of igor that
	// holds the lock.
	lockPath := filepath.Join(igorConfig.TFTPRoot, "/igor/lock")
	dataPath := filepath.Join(igorConfig.TFTPRoot, "/igor/data.gob")

	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open data file %v: %v", lockPath, err)
	}
	defer lock.Close()

	// Lock the file so we don't have simultaneous updates
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		log.Fatal("unable to lock file -- please retry")
	}
	// unlock at the end later
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	db, err := os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open data file %v: %v", dataPath, err)
	}
	defer db.Close()

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
				log.Info("writing data file")
				writeData(db)
				log.Info("writing reservations file")
				writeReservations()
			}

			return
		}
	}

	fmt.Fprintf(os.Stderr, "go: unknown subcommand %q\nRun 'go help' for usage.\n", args[0])
	setExitStatus(2)
}
