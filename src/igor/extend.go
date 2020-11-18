// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
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
	}
	log.Debug("duration: %v", duration)

	// Validate arguments
	if subR == "" {
		help([]string{"extend"})
		log.Fatalln("Missing required argument")
	}

	r := igor.Find(subR)
	if r == nil {
		log.Fatal("reservation does not exist: %v", subR)
	}

	if !r.IsWritable(igor.User) {
		log.Fatal("insufficient privileges to edit reservation: %v", subR)
	}

	if igor.Username != "root" {
		// Make sure the reservation doesn't exceed any limits
		remainingResDuration := r.Remaining(igor.Now)
		if err := igor.checkTimeLimit(len(r.Hosts), remainingResDuration+duration); err != nil {
			log.Fatalln(err, " (%v + %v minutes remaining in your reservation.)", duration, remainingResDuration)
		}

		// Make sure that the user is extending a reservation that is near its
		// completion based on the ExtendWithin config.
		if igor.ExtendWithin > 0 {
			remaining := r.End.Sub(igor.Now)
			if int(remaining.Minutes()) > igor.ExtendWithin {
				log.Fatal("reservations can only be extended if they are within %v minutes of ending", igor.ExtendWithin)
			}
		}
	}

	if err := igor.Extend(r, duration); err != nil {
		log.Fatalln(err)
	}

	timefmt := "Jan 2 15:04"
	fmt.Printf("Reservation %v extended to %v\n", r.Name, r.End.Format(timefmt))
	unsplit := igor.unsplitRange(r.Hosts)
	fmt.Printf("Nodes: %v\n", unsplit)

	emitReservationLog("EXTENDED", r)
}
