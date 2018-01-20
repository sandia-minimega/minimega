// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"ranges"
	"strconv"
	"strings"
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

	v, err := strconv.Atoi(subT)
	if err == nil {
		duration = v
	} else {
		index := strings.Index(subT, "d")
		if index > 0 {
			days, err := strconv.Atoi(subT[:index])
			if err != nil {
				log.Fatal("unable to parse -t: %v", err)
			}
			duration = days * 24 * 60 // convert to minutes
		}

		if index+1 < len(subT) {
			v, err := time.ParseDuration(subT[index+1:])
			if err != nil {
				log.Fatal("unable to parse -t: %v", err)
			}
			duration += int(v / time.Minute)
		}
	}

	if duration < MINUTES_PER_SLICE { //1 slice minimum reservation time
		log.Fatal("Please specify an extension of at least %v minute(s) in length.", MINUTES_PER_SLICE)
		//duration = MINUTES_PER_SLICE
	}
	log.Debug("duration: %v minutes", duration)

	// validate arguments
	if subR == "" || subT == "" {
		help([]string{"extend"})
		log.Fatalln("Missing required argument")
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	// Make sure the reservation doesn't exceed any limits
	if user.Username != "root" && igorConfig.TimeLimit > 0 {
		if duration > igorConfig.TimeLimit {
			log.Fatal("Only root can make a reservation longer than %v minutes", igorConfig.TimeLimit)
		}
	}

	// Make sure there's already a reservation with this name
	for _, r := range Reservations {
		if r.ResName == subR {
			if r.Owner != user.Username {
				log.Fatal("Insufficient permissions for accessing reservation %v", subR)
			}

			// Check to see if nodes are free to extend
			hosts := r.Hosts
			for _, s := range Reservations {
				if s.ResName != r.ResName {
					for _, i := range hosts {
						for _, j := range s.Hosts {
							if i == j && s.StartTime >= r.StartTime && s.StartTime < r.EndTime + int64(60*duration) {
								log.Fatal("Cannot make reservation due to conflict with reservation %v", s.ResName)
							}
						}
					}
				}
			}

			// Set new end time
			fmt.Println(r.EndTime, duration, int64(60*duration))
			r.EndTime += int64(60*duration)
			r.Duration += float64(duration)

			Reservations[r.ID] = r

			timefmt := "Jan 2 15:04"
			rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
			fmt.Printf("Reservation %v extended to %v\n", r.ResName, time.Unix(r.EndTime, 0).Format(timefmt))
			unsplit, _ := rnge.UnsplitRange(r.Hosts)
			fmt.Printf("Nodes: %v\n", unsplit)

			emitReservationLog("EXTENDED", r)

			break
		}
	}

	//TODO: Call new Sched
	//Schedule = newSched

	timefmt := "Jan 2 15:04"
	for _, r := range Reservations {
		fmt.Printf("%v, %v\n", r.ResName, time.Unix(r.EndTime, 0).Format(timefmt))
	}

	// Update the reservation file
	putReservations()
	putSchedule()

}
