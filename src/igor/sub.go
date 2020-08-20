// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os/user"
	"regexp"
	"time"
)

var cmdSub = &Command{
	UsageLine: "sub -r <reservation name> -k <kernel path> -i <initrd path> {-n <integer> | -w <node list>} [OPTIONS]",
	Short:     "create a reservation",
	Long: `
Create a new reservation.

REQUIRED FLAGS:

The -r flag sets the name for the reservation.

The -k flag gives the location of the kernel the nodes should boot. This kernel
will be copied to a separate directory for use.

The -i flag gives the location of the initrd the nodes should boot. This file
will be copied to a separate directory for use.

The -profile flag gives the name of a Cobbler profile the nodes should boot.
This flag takes precedence over the -k and -i flags.

The -n flag indicates that the specified number of nodes should be included in
the reservation. The first available nodes will be allocated.

OPTIONAL FLAGS:

The -c flag sets any kernel command line arguments. (eg "console=tty0").

The -t flag is used to specify the reservation time. Time denominations should
be specified in days(d), hours(h), and minutes(m), in that order. Unitless
numbers are treated as minutes. Days are defined as 24*60 minutes. Example: To
make a reservation for 7 days: -t 7d. To make a reservation for 4 days, 6
hours, 30 minutes: -t 4d6h30m (default = 60m).

The -g flag sets a group owner for the reservation. Any user that is a member
of this group may modify, delete, or perform power operations on the
reservation.

The -s flag is a boolean to enable 'speculative' mode; this will print a
selection of available times for the reservation, but will not actually make
the reservation. Intended to be used with the -a flag to select a specific time
slot.

The -a flag indicates that the reservation should take place on or after the
specified time, given in the format "2017-Jan-2-15:04". Especially useful in
conjunction with the -s flag.

The -vlan flag specifies the name of an existing reservation or a VLAN number.
If a reservation name is provided, the VLAN of the new reservation is set to
the same VLAN as the specified reservation. If a VLAN number is provided, the
new reservation is set to use the specified VLAN.`,
}

var subR string       // -r flag
var subK string       // -k flag
var subI string       // -i
var subN int          // -n
var subC string       // -c
var subT string       // -t
var subS bool         // -s
var subA string       // -a
var subW string       // -w
var subG string       // -g
var subProfile string // -profile
var subVlan string    // -vlan

func init() {
	// break init cycle
	cmdSub.Run = runSub

	cmdSub.Flag.StringVar(&subR, "r", "", "")
	cmdSub.Flag.StringVar(&subK, "k", "", "")
	cmdSub.Flag.StringVar(&subI, "i", "", "")
	cmdSub.Flag.IntVar(&subN, "n", 0, "")
	cmdSub.Flag.StringVar(&subC, "c", "", "")
	cmdSub.Flag.StringVar(&subT, "t", "60m", "")
	cmdSub.Flag.BoolVar(&subS, "s", false, "")
	cmdSub.Flag.StringVar(&subA, "a", "", "")
	cmdSub.Flag.StringVar(&subW, "w", "", "")
	cmdSub.Flag.StringVar(&subG, "g", "", "")
	cmdSub.Flag.StringVar(&subProfile, "profile", "", "")
	cmdSub.Flag.StringVar(&subVlan, "vlan", "", "")
}

func runSub(cmd *Command, args []string) {
	r := new(Reservation) // the new reservation

	format := "2006-Jan-2-15:04"

	// duration is in minutes
	duration, err := parseDuration(subT)
	if err != nil {
		log.Fatal("unable to parse -t: %v", err)
	} else if duration <= 0 {
		log.Fatal("Please specify a positive value for -t")
	}
	log.Debug("duration: %v", duration)

	r.Duration = duration

	// validate arguments
	if subR == "" || (subN == 0 && subW == "") {
		help([]string{"sub"})
		log.Fatalln("Missing required argument")
	}

	if subW != "" && subN > 0 {
		log.Fatalln("Must specify either number of nodes or list of nodes")
	}

	// make sure there's no weird characters in the reservation name
	if matched, err := regexp.MatchString("^[a-zA-Z0-9-_]+$", subR); !matched {
		log.Fatalln("reservation name contains invalid characters, must only contain letters, numbers, hyphen and underscores")
	} else if err != nil {
		// ???
		log.Fatalln(err)
	}

	if (subK == "" || subI == "") && subProfile == "" {
		help([]string{"sub"})
		log.Fatalln("Must specify either a kernel & initrd, or a Cobbler profile")
	}

	if subProfile != "" && !igor.UseCobbler {
		log.Fatalln("igor is not configured to use Cobbler, cannot specify a Cobbler profile")
	}

	// Validate the cobbler profile
	if subProfile != "" {
		cobblerProfiles := CobblerProfiles()
		if !cobblerProfiles[subProfile] {
			log.Fatal("Cobbler profile does not exist: %v", subProfile)
		}
	}

	// Check VLAN Availablility
	if subVlan != "" {
		vlan, err := parseVLAN(subVlan)
		if err != nil {
			log.Fatalln(err)
		}

		r.Vlan = vlan
	}

	// Make sure there's not already a reservation with this name
	if igor.Find(subR) != nil {
		log.Fatal("A reservation named %v already exists.", subR)
	}

	// figure out which nodes to reserve
	if subW != "" {
		r.SetHosts(igor.splitRange(subW))
		if len(r.Hosts) == 0 {
			log.Fatal("Couldn't parse node specification %v", subW)
		}
	} else {
		r.Hosts = make([]string, subN)
	}

	// Make sure the reservation doesn't exceed any limits
	if igor.Username != "root" && igor.NodeLimit > 0 {
		if subN > igor.NodeLimit || len(r.Hosts) > igor.NodeLimit {
			log.Fatal("Only root can make a reservation of more than %v nodes", igor.NodeLimit)
		}
	}
	if igor.Username != "root" {
		if err := igor.checkTimeLimit(len(r.Hosts), duration); err != nil {
			log.Fatalln(err)
		}
	}

	// set group if specified
	if subG != "" {
		g, err := user.LookupGroup(subG)
		if err != nil {
			log.Fatalln(err)
		}

		r.Group = subG
		r.GroupID = g.Gid
	}

	r.Start = igor.Now.Round(time.Minute).Add(-time.Minute * 1) //keep from putting the reservation 1 minute into future
	if subA != "" {
		loc, _ := time.LoadLocation("Local")
		t, _ := time.Parse(format, subA)
		r.Start = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}

	// If this is a speculative call, run findReservationAfter a few times,
	// print, and exit
	if subS {
		fmt.Println("AVAILABLE RESERVATIONS")
		fmt.Println("START\t\t\tEND")
		for i := 0; i < 10; i++ {
			r.Start = r.Start.Add(10 * time.Minute)

			if subN > 0 {
				r.Hosts = make([]string, subN)

				if err := igor.Schedule(r, true); err != nil {
					log.Fatalln(err)
				}
			} else if subW != "" {
				if err := igor.Schedule(r, true); err != nil {
					log.Fatalln(err)
				}
			}
			fmt.Printf("%v\t%v\n", r.Start.Format(format), r.End.Format(format))
		}
		return
	}

	if err := igor.Schedule(r, false); err != nil {
		log.Fatalln(err)
	}

	r.Owner = igor.Username
	r.Name = subR
	r.KernelArgs = subC
	r.CobblerProfile = subProfile // safe to do even if unset

	// If we're not doing a Cobbler profile...
	if subProfile == "" {
		if err := r.SetKernel(subK); err != nil {
			log.Fatalln(err)
		}
		if err := r.SetInitrd(subI); err != nil {
			if err := igor.PurgeFiles(r); err != nil {
				log.Error("leaked kernel: %v", subK)
			}
			log.Fatalln(err)
		}
	}

	timefmt := "Jan 2 15:04"
	fmt.Printf("Reservation created for %v - %v\n", r.Start.Format(timefmt), r.End.Format(timefmt))
	unsplit := igor.unsplitRange(r.Hosts)
	fmt.Printf("Nodes: %v\n", unsplit)

	emitReservationLog("CREATED", r)
}
