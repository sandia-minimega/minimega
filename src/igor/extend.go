// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"ranges"
	"time"
)

var cmdExtend = &Command{
	UsageLine: "extend -r <reservation name> -t <time>",
	Short:     "extend a reservation",
	Long: `
Extend an existing reservation.

REQUIRED FLAGS:

The -r flag specifies the name of the existing reservation.

OPTIONAL FLAGS:

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
	duration, err := parseDuration(subT)
	if err != nil {
		log.Fatal("unable to parse -t: %v", err)
	} else if duration <= 0 {
		log.Fatal("Please specify a positive value for -t")
	} else if duration%MINUTES_PER_SLICE != 0 { // Reserve at least (duration) minutes worth of slices, in increments of MINUTES_PER_SLICE
		duration = (duration/MINUTES_PER_SLICE + 1) * MINUTES_PER_SLICE
	}
	log.Debug("duration: %v minutes", duration)

	// Validate arguments
	if subR == "" {
		help([]string{"extend"})
		log.Fatalln("Missing required argument")
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	for _, r := range Reservations {
		if r.ResName != subR {
			continue
		}

		// The reservation name is unique if it exists
		if r.Owner != user.Username && user.Username != "root" {
			log.Fatal("Cannot access reservation %v: insufficient privileges", subR)
		}

		// Make sure the reservation doesn't exceed any limits
		if user.Username != "root" && igorConfig.TimeLimit > 0 {
			if float64(duration)+r.Duration > float64(igorConfig.TimeLimit) {
				log.Fatal("Only root can extend a reservation longer than %v minutes. The maximum allowable time you may extend is %v minutes.", igorConfig.TimeLimit, float64(igorConfig.TimeLimit)-r.Duration)
			}
		}

		// Make sure there's enough space in the Schedule for the reservation
		resEnd := (r.EndTime - Schedule[0].Start) / 60 // number of minutes from beginning of Schedule to end of r
		schedEnd := len(Schedule) * MINUTES_PER_SLICE  // total number of minutes in the current Schedule
		if int(resEnd)+duration >= int(schedEnd) {
			extendSchedule(int(resEnd) + duration - int(schedEnd))
		}

		// Check to see if nodes are free to extend; if so, update the Schedule
		for i := 0; i < duration/MINUTES_PER_SLICE; i++ {
			nodes, err := getNodeIndexes(r.Hosts)
			if err != nil {
				log.Fatal("Could not get host indices: %v", err)
			}

			for _, idx := range nodes {
				// Check if each node is free on the Schedule
				if !isFree(Schedule[resEnd/MINUTES_PER_SLICE+int64(i)].Nodes, idx, idx) {
					log.Fatal("Cannot extend reservation due to conflict")
				} else {
					Schedule[resEnd/MINUTES_PER_SLICE+int64(i)].Nodes[idx] = r.ID
				}
			}
		}

		// Set new end time
		r.EndTime += int64(60 * duration)
		r.Duration += float64(duration)

		Reservations[r.ID] = r

		timefmt := "Jan 2 15:04"
		rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
		fmt.Printf("Reservation %v extended to %v\n", r.ResName, time.Unix(r.EndTime, 0).Format(timefmt))
		unsplit, _ := rnge.UnsplitRange(r.Hosts)
		fmt.Printf("Nodes: %v\n", unsplit)

		emitReservationLog("EXTENDED", r)

		dirty = true

		return
	}

	// We didn't find the reservation, so error out
	log.Fatal("Reservation %v does not exist", subR)
}
