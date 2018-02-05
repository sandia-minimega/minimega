// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"ranges"
	"strconv"
//	"strings"
	"time"
)

var cmdExtend = &Command{
	UsageLine: "extend -r <reservation name> -t <time>",
	Short:     "extend a reservation",
	Long: `
Extend an existing reservation.

REQUIRED FLAGS:

The -r flag specifies the name of the existing reservation.

The -t flag is used to specify the reservation extension time. Time denominations should
be specified in days(d), hours(h), and minutes(m), in that order. Unitless
numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To
make a reservation for 7 days: -t 7d. To make a reservation for 4 days, 6
hours, 30 minutes: -t 4d6h30m (default = 60m).`,
}

func init() {
	// break init cycle
	cmdExtend.Run = runExtend

	cmdExtend.Flag.StringVar(&subR, "r", "", "")
	cmdExtend.Flag.StringVar(&subT, "t", "60m", "")
}

func runExtend(cmd *Command, args []string) {
	// duration is in minutes
	duration := 0

	duration, err := parseDuration(subT)
	if err != nil {
		log.Fatal("unable to parse -t: %v", err)
	} else if duration < MINUTES_PER_SLICE { //1 slice minimum reservation time
		log.Fatal("Please specify an extension of at least %v minute(s) in length.", MINUTES_PER_SLICE)
		//duration = MINUTES_PER_SLICE
	}
	log.Debug("duration: %v minutes", duration)

	// Validate arguments
	if subR == "" || subT == "" {
		help([]string{"extend"})
		log.Fatalln("Missing required argument")
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	// Make sure there's already a reservation with this name
	exists := false

	for _, r := range Reservations {
		if r.ResName == subR { // The reservation name is unique
			if r.Owner != user.Username {
				log.Fatal("You are not the owner of reservation %v", subR)
			}

			// Make sure the reservation doesn't exceed any limits
			if user.Username != "root" && igorConfig.TimeLimit > 0 {
				if float64(duration) + r.Duration > float64(igorConfig.TimeLimit) {
					log.Fatal("Only root can extend a reservation longer than %v minutes", igorConfig.TimeLimit)
				}
			}

			// Check to see if nodes are free to extend; if so, update the Schedule
			for i := 0; i < duration; i++ {
				for _, host := range r.Hosts {
					// Allow for rune-encoded cluster prefix names
					preflen := len([]rune(igorConfig.Prefix))
					// TODO: Needs a better check; will reserve nodes 1-5 if ccc[11-15] are reserved and someone dynamically
					//	 changes the prefix from "ccc" (len 3) to "cccc" (len 4) in igor.conf
					if preflen >= len([]rune(host)) {
						log.Fatal("Could not parse cluster prefix name (Did you change your config file?)")
					}
					idx, err := strconv.Atoi(host[preflen:]) // Get node index, e.g. for "ccc[4-6],ccc8" idx iterates over 4,5,6,8
					if err != nil {
						//should not see this unless cluster node naming convention changes
						log.Fatal("could not get host indices")
					}
					if !isFree(Schedule[(r.EndTime-Schedule[0].Start)/60*MINUTES_PER_SLICE + int64(i)].Nodes, idx, idx) {
						log.Fatal("Cannot extend reservation due to conflict")
					} else {
						Schedule[(r.EndTime-Schedule[0].Start)/60*MINUTES_PER_SLICE + int64(i)].Nodes[idx] = r.ID
					}
				}
			}

			// Set new end time
			r.EndTime += int64(60*duration)
			r.Duration += float64(duration)

			Reservations[r.ID] = r

			timefmt := "Jan 2 15:04"
			rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
			fmt.Printf("Reservation %v extended to %v\n", r.ResName, time.Unix(r.EndTime, 0).Format(timefmt))
			unsplit, _ := rnge.UnsplitRange(r.Hosts)
			fmt.Printf("Nodes: %v\n", unsplit)

			emitReservationLog("EXTENDED", r)

			exists = true

			break
		}
	}

	// If the reservation does not exist then we error out
	if !exists {
		log.Fatal("Reservation %v does not exist", subR)
	}

	// Update the reservation file
	putReservations()
	putSchedule()

}
