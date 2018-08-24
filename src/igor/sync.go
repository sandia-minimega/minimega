// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
)

var cmdSync = &Command{
	UsageLine: "sync",
	Short:     "synchronize igor data",
	Long: `
Does an internal check to verify the integrity of the data file.

OPTIONAL FLAGS:

	-v	Verbose - Prints additioanl information
	`,
}

var subL bool // -l
var subV bool // -v

func init() {
	// break init cycle
	cmdSync.Run = runSync

	cmdSync.Flag.BoolVar(&subL, "l", false, "")
	cmdSync.Flag.BoolVar(&subV, "v", false, "")
}

// Gather data integrity information, report, and fix
func runSync(cmd *Command, args []string) {
	user, err := getUser()
	if err != nil {
		log.Fatalln("Cannot determine current user", err)
	}
	if user.Username != "root" {
		log.Fatalln("Sync access restricted. Please use as admin.")
	}

	log.Debug("Sync called - finding orphan IDs")
	IDs := getOrphanIDs()
	if len(IDs) > 0 {
		fmt.Printf("%v orphan Reservation IDs found\n", len(IDs))
		if subV {
			for _, id := range IDs {
				fmt.Println(id)
			}
		}
	}
	// we are only listing
	if subL {
		return
	}

	// purge the orphan IDs from the shedule
	if len(IDs) > 0 {
		if !subV {
			fmt.Println("Purging Orphan IDs from Schedule...")
		}
		for _, oid := range IDs {
			purgeFromSchedule(oid)
		}
		fmt.Println("Done.")
		dirty = true
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
		resIDs[r.ID] = false
	}
	resIDs[0] = false //we don't care about 0
	// Compile a list of the remaining IDs, if any
	var orphanIDs []uint64
	for k, v := range resIDs {
		if v {
			orphanIDs = append(orphanIDs, k)
		}
	}
	log.Debug("Sync:getOrphanIDs concluded with: %v", resIDs)
	return orphanIDs
}

func purgeFromSchedule(id uint64) {
	if subV {
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
