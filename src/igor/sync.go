// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"time"
)

var cmdSync = &Command{
	UsageLine: "sync",
	Short:     "synchronize igor data",
	Long: `
Does an internal check to verify the integrity of the data file. Can report and attempt to clean.

SYNOPSIS
	igor sync <[-d] [-f]> [-q] WHAT

OPTIONS

	-f, -force
	    Will force sync to fix inconsistencies found in addition to reporting

	-d, -dry-run
	    Does not attempt to make any corrections, only reports inconsistencies

	-q, -quiet
	    Suppress reports, only report errors

Possible WHATs:

arista: 	reconfigure switchports for active reservations
schedule:	purge orphan reservation IDs from the schedule
	`,
}

var subF bool   // -f
var force bool  // -force
var subD bool   // -d
var dryRun bool // -dry-run
var subQ bool   // -q
var quiet bool  // -quiet

func init() {
	// break init cycle
	cmdSync.Run = runSync

	cmdSync.Flag.BoolVar(&subF, "f", false, "")
	cmdSync.Flag.BoolVar(&force, "force", false, "")
	cmdSync.Flag.BoolVar(&subD, "d", false, "")
	cmdSync.Flag.BoolVar(&dryRun, "dry-run", false, "")
	cmdSync.Flag.BoolVar(&subQ, "q", false, "")
	cmdSync.Flag.BoolVar(&quiet, "quiet", false, "")
}

// Gather data integrity information, report, and fix
func runSync(cmd *Command, args []string) {
	// process flags
	dryRun = (dryRun || subD)
	force = (force || subF)
	quiet = (quiet || subQ)

	if dryRun == force {
		log.Fatal("Missing or invalid flags. Please see igor sync -h, --help")
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("Cannot determine current user", err)
	}
	if user.Username != "root" {
		log.Fatalln("Sync access restricted. Please use as admin.")
	}

	if len(args) != 1 {
		log.Fatalln("Invalid arguments")
	}

	switch args[0] {
	case "schedule":
		syncSchedule()
	case "arista":
		syncArista()
	default:
		log.Fatalln("Invalid arguments")
	}
}

func syncSchedule() {
	log.Debug("Sync called - finding orphan IDs")
	IDs := getOrphanIDs()
	if len(IDs) > 0 && !quiet {
		fmt.Printf("%v orphan Reservation IDs found:\n", len(IDs))
		for _, id := range IDs {
			fmt.Println(id)
		}
	}

	// purge the orphan IDs from the schedule
	if len(IDs) > 0 && force {
		if !quiet {
			fmt.Println("Purging Orphan IDs from Schedule...")
		}
		for _, oid := range IDs {
			purgeFromSchedule(oid)
		}
		if !quiet {
			fmt.Println("Done.")
		}
		dirty = true
	}

}

func syncArista() {
	now := time.Now()

	for _, r := range Reservations {
		if !r.Active(now) {
			continue
		}

		if !quiet {
			fmt.Printf("Set switchports for %v to %v\n", r.Hosts, r.Vlan)
		}
		if !dryRun {
			if err := networkSet(r.Hosts, r.Vlan); err != nil {
				log.Fatal("unable to set up network isolation")
			}
		}
	}
}

func getOrphanIDs() []uint64 {
	resIDs := make(map[uint64]bool)
	// make a list of all reseration IDs that appear in the schedule
	for _, s := range Schedule {
		for _, n := range s.Nodes {
			resIDs[n] = true
		}
	}
	// Go through the reservations and turn off IDs we know about
	for _, r := range Reservations {
		delete(resIDs, r.ID)
	}
	delete(resIDs, 0) //we don't care about 0
	// Compile a list of the remaining IDs, if any
	orphanIDs := make([]uint64, 0, len(resIDs))
	for k, _ := range resIDs {
		orphanIDs = append(orphanIDs, k)
	}
	log.Debug("Sync:getOrphanIDs concluded with: %v", resIDs)
	return orphanIDs
}

func purgeFromSchedule(id uint64) {
	if !quiet {
		fmt.Printf("Purging orphan ID %v from schedule...\n", id)
	}
	newSched := Schedule
	for i := 0; i < len(newSched); i++ {
		for j := 0; j < len(newSched[i].Nodes); j++ {
			if newSched[i].Nodes[j] == id {
				newSched[i].Nodes[j] = 0
			}
		}
	}
	Schedule = newSched
}
