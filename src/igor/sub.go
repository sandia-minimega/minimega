// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	log "minilog"
	"os"
	"path/filepath"
	"ranges"
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

The -s flag is a boolean to enable 'speculative' mode; this will print a
selection of available times for the reservation, but will not actually make
the reservation. Intended to be used with the -a flag to select a specific time
slot.

The -a flag indicates that the reservation should take place on or after the
specified time, given in the format "2017-Jan-2-15:04". Especially useful in
conjunction with the -s flag.`,
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
var subProfile string // -profile

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
	cmdSub.Flag.StringVar(&subProfile, "profile", "", "")
}

// install src into dir, using the hash as the file name. Returns the hash or
// an error.
func install(src, dir, suffix string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// hash the file
	hash := sha1.New()
	if _, err := io.Copy(hash, f); err != nil {
		return "", fmt.Errorf("unable to hash file %v: %v", src, err)
	}

	fname := hex.EncodeToString(hash.Sum(nil))

	dst := filepath.Join(dir, fname+suffix)

	// copy the file if it doesn't already exist
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		// need to go back to the beginning of the file since we already read
		// it once to do the hashing
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}

		f2, err := os.Create(dst)
		if err != nil {
			return "", err
		}
		defer f2.Close()

		if _, err := io.Copy(f2, f); err != nil {
			return "", fmt.Errorf("unable to install %v: %v", src, err)
		}
	} else if err != nil {
		// strange...
		return "", err
	} else {
		log.Info("file with identical hash %v already exists, skipping install of %v.", fname, src)
	}

	return fname, nil
}

func runSub(cmd *Command, args []string) {
	var nodes []string          // if the user has requested specific nodes
	var reservation Reservation // the new reservation
	var newSched []TimeSlice    // the new schedule
	format := "2006-Jan-2-15:04"

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

	// validate arguments
	if subR == "" || (subN == 0 && subW == "") {
		help([]string{"sub"})
		log.Fatalln("Missing required argument")
	}

	if (subK == "" || subI == "") && subProfile == "" {
		help([]string{"sub"})
		log.Fatalln("Must specify either a kernel & initrd, or a Cobbler profile")
	}

	if subProfile != "" && !igorConfig.UseCobbler {
		log.Fatalln("igor is not configured to use Cobbler, cannot specify a Cobbler profile")
	}

	// Validate the cobbler profile
	if subProfile != "" {
		cobblerProfiles := CobblerProfiles()
		if !cobblerProfiles[subProfile] {
			log.Fatal("Cobbler profile does not exist: %v", subProfile)
		}
	}

	user, err := getUser()
	if err != nil {
		log.Fatalln("cannot determine current user", err)
	}

	// Make sure there's not already a reservation with this name
	for _, r := range Reservations {
		if r.ResName == subR {
			log.Fatal("A reservation named %v already exists.", subR)
		}
	}

	// figure out which nodes to reserve
	if subW != "" {
		rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
		nodes, _ = rnge.SplitRange(subW)
		if len(nodes) == 0 {
			log.Fatal("Couldn't parse node specification %v", subW)
		}
		if !checkValidNodeRange(nodes) {
			log.Fatalln("Invalid node range")
		}
	}

	// Make sure the reservation doesn't exceed any limits
	if user.Username != "root" && igorConfig.NodeLimit > 0 {
		if subN > igorConfig.NodeLimit || len(nodes) > igorConfig.NodeLimit {
			log.Fatal("Only root can make a reservation of more than %v nodes", igorConfig.NodeLimit)
		}
	}
	if user.Username != "root" && igorConfig.TimeLimit > 0 {
		if duration > igorConfig.TimeLimit {
			log.Fatal("Only root can make a reservation longer than %v minutes", igorConfig.TimeLimit)
		}
	}

	when := time.Now().Add(-time.Minute * MINUTES_PER_SLICE) //keep from putting the reservation 1 minute into future
	if subA != "" {
		loc, _ := time.LoadLocation("Local")
		t, _ := time.Parse(format, subA)
		when = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	}

	// If this is a speculative call, run findReservationAfter a few times,
	// print, and exit
	if subS {
		fmt.Println("AVAILABLE RESERVATIONS")
		fmt.Println("START\t\t\tEND")
		for i := 0; i < 10; i++ {
			var r Reservation
			if subN > 0 {
				r, _, err = findReservationAfter(duration, subN, when.Add(time.Duration(i*10)*time.Minute).Unix())
				if err != nil {
					log.Fatalln(err)
				}
			} else if subW != "" {
				r, _, err = findReservationGeneric(duration, 0, nodes, true, when.Add(time.Duration(i*10)*time.Minute).Unix())
				if err != nil {
					log.Fatalln(err)
				}
			}
			fmt.Printf("%v\t%v\n", time.Unix(r.StartTime, 0).Format(format), time.Unix(r.EndTime, 0).Format(format))
		}
		return
	}

	if subW != "" {
		if subN > 0 {
			log.Fatalln("Both -n and -w options used. Operation canceled.")
		}
		reservation, newSched, err = findReservationGeneric(duration, 0, nodes, true, when.Unix())
	} else if subN > 0 {
		reservation, newSched, err = findReservationAfter(duration, subN, when.Unix())
	}
	if err != nil {
		log.Fatalln(err)
	}

	// pick a network segment
	var vlan int
VlanLoop:
	for vlan = igorConfig.VLANMin; vlan <= igorConfig.VLANMax; vlan++ {
		for _, r := range Reservations {
			if vlan == r.Vlan {
				continue VlanLoop
			}
		}
		break
	}
	if vlan > igorConfig.VLANMax {
		log.Fatal("couldn't assign a vlan!")
	}
	reservation.Vlan = vlan

	reservation.Owner = user.Username
	reservation.ResName = subR
	reservation.KernelArgs = subC
	reservation.CobblerProfile = subProfile // safe to do even if unset

	// If we're not doing a Cobbler profile...
	if subProfile == "" {
		reservation.Kernel = subK
		reservation.Initrd = subI

		dir := filepath.Join(igorConfig.TFTPRoot, "igor")

		if hash, err := install(subK, dir, "-kernel"); err != nil {
			log.Fatal("reservation failed: %v", err)
		} else {
			reservation.KernelHash = hash
		}

		if hash, err := install(subI, dir, "-initrd"); err != nil {
			// TODO: we may leak a kernel here
			log.Fatal("reservation failed: %v", err)
		} else {
			reservation.InitrdHash = hash
		}
	}

	// Add it to the list of reservations
	Reservations[reservation.ID] = reservation

	timefmt := "Jan 2 15:04"
	rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
	fmt.Printf("Reservation created for %v - %v\n", time.Unix(reservation.StartTime, 0).Format(timefmt), time.Unix(reservation.EndTime, 0).Format(timefmt))
	unsplit, _ := rnge.UnsplitRange(reservation.Hosts)
	fmt.Printf("Nodes: %v\n", unsplit)

	Schedule = newSched

	// update the network config
	err = networkSet(reservation.Hosts, vlan)
	if err != nil {
		// TODO: we may leak a kernel and initrd here
		log.Fatal("unable to set up network isolation")
	}

	emitReservationLog("CREATED", reservation)

	dirty = true
}
