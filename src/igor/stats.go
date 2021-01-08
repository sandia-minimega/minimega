// Copyright 2018-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"bufio"
	"bytes"
	"fmt"
	log "minilog"
	"os"
	"ranges"
	"strconv"
	"strings"
	"time"
)

var cmdStats = &Command{
	UsageLine: "stats",
	Short:     "calculate statistics given a number of days prior to today",
	Long: `
Show usage statistics for a range of days prior to today or duration of log history, whichever is shorter

REQUIRED FLAGS:

-d   Duration (in days) - specifies the number of days to be included in the report, ending with today. e.g. igor stats -d 7 will display statistics for the last seven days

OPTIONAL FLAGS:

-v   verbose

	`,
}

var statsD string // -pd flag
var statsV bool   // -v flag

func init() {
	// break init cycle
	cmdStats.Run = runStats
	cmdStats.Flag.StringVar(&statsD, "d", "", "")
	cmdStats.Flag.BoolVar(&statsV, "v", false, "")

}

// Data type for individual Reservations
type ResData struct {
	ResName        string
	ResStart       time.Time
	ResEnd         time.Time
	ActualEnd      time.Time
	ActualDuration time.Duration
	ResID          int
	Nodes          []string
	NumExtensions  int
}

// Data Structure to Store igor statistics
type Stats struct {
	NumRes               int
	NodesUsed            map[string]int
	NumNodes             int
	NumUsers             int
	TotalDurationMinutes time.Duration
	TotalEarlyCancels    int
	TotalExtensions      int
	Reservations         map[string][]*ResData
	Reservationsid       map[int]*ResData
}

// Main Stats function to output reservation calculations
func runStats(_ *Command, _ []string) {

	if statsD == "" {
		help([]string{"stats"})
		return
	}

	d, err := strconv.Atoi(statsD) // Day Paramater how many days in the past to collect data
	if err != nil {
		help([]string{"stats"})
		log.Fatalln("Invalid Duration Specified")
	}

	gs := Stats{
		Reservations:   make(map[string][]*ResData),
		NodesUsed:      make(map[string]int),
		Reservationsid: make(map[int]*ResData),
	}
	gs.readLog()
	statstartdate := igor.Now.AddDate(0, 0, -d)
	gs.calculateStats(statstartdate)
	fmt.Println(printStats(&gs))
}

// Adds reservation to a particular user. Map of user names to slices of reservations
func (stats *Stats) addReservation(rn string, ru string, ri int, start time.Time, end time.Time, nodes string) {
	rd := ResData{}
	rd.ResName = rn
	rd.ResStart = start
	rd.ResEnd = end
	list, err := ranges.SplitList(nodes)
	rd.Nodes = list
	if err != nil {
		log.Fatal("%v", err)
	}
	rd.ResID = ri
	stats.Reservations[ru] = append(stats.Reservations[ru], &rd)
	if ri != -1 { // if there was no id field do not add to the map
		stats.Reservationsid[ri] = &rd
	}
}

// Adds the end of a reservation to a particular user's reservation.
// Attempts to find a reservation if it does not find one assume the log was reset and create a reservation
func (stats *Stats) addEndRes(rn string, ru string, ri int, rs time.Time, re time.Time, ae time.Time, nodes string) {
	res := stats.findRes(ru, rn, ri, rs)
	if res != nil {
		res.ActualEnd = ae
		res.ActualDuration = ae.Sub(res.ResStart)
		return
	}
	stats.addReservation(rn, ru, ri, rs, re, nodes)
	res = stats.findRes(ru, rn, ri, rs)
	res.ActualEnd = ae
	res.ActualDuration = ae.Sub(res.ResStart)
}

// Extends a reservation
// Attempts to find a reservation if it does not find one assume the log was reset and create a reservation
func (stats *Stats) extendRes(rn string, ru string, ri int, rs time.Time, rex time.Time, nodes string) {
	res := stats.findRes(ru, rn, ri, rs)
	if res != nil {
		res.ResEnd = rex
		res.NumExtensions++
		return
	}
	stats.addReservation(rn, ru, ri, rs, rex, nodes)
	res = stats.findRes(ru, rn, ri, rs)
	res.NumExtensions++
	stats.Reservations[ru][len(stats.Reservations[ru])-1] = res

}

// Reads the logfile and adds the necessary reservations and usage time
func (stats *Stats) readLog() {
	f, err := os.Open(igor.LogFile)
	if err != nil {
		log.Fatal("unable to open log")
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		// Unless the log file has action ID of util.go: skip it
		if !strings.Contains(line, "util.go:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 11 {
			continue
		}

		// Field names in the Log Line
		dateStamp := 0
		timeStamp := 1
		action := 4
		// Below calculates where the fields are in case parameters were moved
		resuser, err := stats.calculateVariable("user=", fields)
		if err {
			continue
		}
		resname, err := stats.calculateVariable("resname=", fields)
		if err {
			continue
		}
		resnodes, err := stats.calculateVariable("nodes=", fields)
		if err {
			continue
		}
		resstart, err := stats.calculateVariable("start=", fields)
		if err {
			continue
		}
		resend, err := stats.calculateVariable("end=", fields)
		if err {
			continue
		}
		resid, err := stats.calculateVariable("id=", fields)
		if err {
			resid = -1
		}
		formatDateStamp := "2006/01/02"
		formatTimeStamp := "15:04:05"
		formatLong := "2006-Jan-2-15:04"
		var nodes string

		ri := -1
		rs, er := time.Parse(formatLong, strings.TrimPrefix(fields[resstart], "start="))
		if er != nil {
			log.Fatal("%v", er)
		}
		re, er := time.Parse(formatLong, strings.TrimPrefix(fields[resend], "end="))
		if er != nil {
			log.Fatal("%v", er)
		}
		rn := strings.TrimPrefix(fields[resname], "resname=")
		ru := strings.TrimPrefix(fields[resuser], "user=")
		nodes = strings.TrimPrefix(fields[resnodes], "nodes=")
		if resid != -1 {
			ri, er = strconv.Atoi(strings.TrimPrefix(fields[resid], "id="))
			if er != nil {
				ri = -1
			}
		}

		// choose action to add new reservation metric or add new usage metric
		switch fields[action] {

		case "INSTALL":
			if !rs.After(igor.Now.AddDate(0, 0, 0)) {
				stats.addReservation(rn, ru, ri, rs, re, nodes)
			}
		case "DELETED":
			ad, er := time.Parse(formatDateStamp, fields[dateStamp])
			if er != nil {
				log.Fatal("%v", er)
			}
			at, er := time.Parse(formatTimeStamp, fields[timeStamp])
			if er != nil {
				log.Fatal("%v", er)
			}
			ae := time.Date(ad.Year(), ad.Month(), ad.Day(), at.Hour(), at.Minute(), at.Second(), 0, at.Location())
			stats.addEndRes(rn, ru, ri, rs, re, ae, nodes)
		case "EXTENDED":
			stats.extendRes(rn, ru, ri, rs, re, nodes)
		}
	}
}

// Function to handle the statistics calculation
// Walk through every reservation of thats within the search criteria
// Calculate the 5 metrics , Total Number of Unique users, Number of Unique Nodes Used,
// Number of Reservations, Number of Reservations Cancelled Early, Total Reservation Time Used
func (stats *Stats) calculateStats(statstartdate time.Time) {
	for n, rd := range stats.Reservations {
		var uResCount, uEarlyCancel, uExtension int
		var uDuration time.Duration
		var b bytes.Buffer
		userHadvalidRes := false
		if statsV {
			fmt.Fprintf(&b, "------------------User Summary for %v ------------------\n", n)
		}
		for _, d := range rd {
			var empty time.Time
			if d.ActualEnd.Before(statstartdate) && !d.ActualEnd.Equal(empty) {
				continue // ended before period we care about
			}
			if d.ActualEnd.Before(d.ResStart) && !d.ActualEnd.Equal(empty) {
				continue // deleted a queued res that had not yet started
			}
			if d.ResStart.After(igor.Now) {
				continue // reservation hasnt started yet
			}
			if d.ResEnd.Before(statstartdate) {
				continue // reservation did not have a delete in the log assume it was deleted
			}
			uExtension += d.NumExtensions
			userHadvalidRes = true
			for _, n := range d.Nodes {
				stats.NodesUsed[n]++
			}
			nodeMultiplier := time.Duration(len(d.Nodes))

			if d.ActualDuration.Minutes() == 0 { // we never saw this res get deleted
				if statstartdate.Before(d.ResStart) {
					uDuration += nodeMultiplier * igor.Now.Sub(d.ResStart)
					d.ActualDuration = igor.Now.Sub(d.ResStart)
				} else {
					uDuration += nodeMultiplier * igor.Now.Sub(statstartdate)
					d.ActualDuration = igor.Now.Sub(statstartdate)
				}
			} else {
				if statstartdate.Before(d.ActualEnd) {
					if statstartdate.Before(d.ResStart) {
						uDuration += nodeMultiplier * d.ActualDuration
					} else {
						uDuration += nodeMultiplier * d.ActualEnd.Sub(statstartdate)
					}
				}

			}
			uResCount++
			// fuzzy math here because igor takes a few seconds to delete a res
			if d.ActualEnd != empty && (d.ResEnd.Sub(d.ActualEnd).Minutes()) > 1.0 {
				uEarlyCancel++
			}
			if statsV {
				fmt.Fprintf(&b, d.String())
			}

		}
		if statsV && uResCount > 0 {
			fmt.Fprintf(&b, "Total Number of Reservations: %v\n", uResCount)
			fmt.Fprintf(&b, "Total Early Cancel: %v\n", uEarlyCancel)
			fmt.Fprintf(&b, "Number of Extensions: %v\n", uExtension)
			fmt.Fprintf(&b, "Total user duration: %v\n\n", fmtDuration(uDuration))
			fmt.Println(b.String())
		}
		if userHadvalidRes {
			stats.NumUsers++
			stats.NumRes += uResCount
			stats.TotalDurationMinutes += uDuration
			stats.TotalEarlyCancels += uEarlyCancel
			stats.TotalExtensions += uExtension
		}
	}
	for _, d := range stats.NodesUsed {
		if d > 0 {
			stats.NumNodes++
		}
	}

}

// Returns Reservation data pointer. tries to find the reservation by unique ID
// Otherwise it will search by name and by user
func (stats *Stats) findRes(ru string, rn string, ri int, rs time.Time) *ResData {
	if res, found := stats.Reservationsid[ri]; found {
		return res
	}
	for i, res := range stats.Reservations[ru] {
		if res.ResStart == rs && res.ResName == rn {
			return stats.Reservations[ru][i]
		}
	}
	return nil
}

//Finds out where in the log line is a particular field of interest
func (stats *Stats) calculateVariable(param string, fields []string) (int, bool) {
	for i := 5; i < len(fields); i++ {
		if strings.Contains(fields[i], param) {
			return i, false
		}
	}
	return -1, true
}

//toString of ResData Struct
func (res *ResData) String() string {
	var b bytes.Buffer
	var empty time.Time
	formatLong := "2006-Jan-2-15:04"
	fmt.Fprintf(&b, "Reservation Name: %v\tReservation ID: %v\n", res.ResName, res.ResID)
	fmt.Fprintf(&b, "Nodes:")
	for i, n := range res.Nodes {
		fmt.Fprintf(&b, "% v ", n)
		if i%10 == 0 && i != 0 {
			fmt.Fprintln(&b, "")
		}
	}
	ae := "Currently Active"
	if !res.ActualEnd.Equal(empty) {
		ae = res.ActualEnd.Format(formatLong)
	}
	fmt.Fprintln(&b, "")
	fmt.Fprintf(&b, "Reservation Start: %v\tReservation End: %v\n", res.ResStart.Format(formatLong), res.ResEnd.Format(formatLong))
	fmt.Fprintf(&b, "Actual End: %v\tActual Duration: %v\n", ae, res.ActualDuration.String())
	fmt.Fprintf(&b, "Number of Extensions: %v\n\n", res.NumExtensions)

	return b.String()
}

func printStats(gs *Stats) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "------------------Global Statistics for all nodes------------------\n")
	fmt.Fprintf(&b, "Total Users: %v\n", gs.NumUsers)
	fmt.Fprintf(&b, "Number of Nodes used: %v\n", gs.NumNodes)
	fmt.Fprintf(&b, "Total Number of Reservations: %v\n", gs.NumRes)
	fmt.Fprintf(&b, "Total Number of Reservations Cancelled Early: %v\n", gs.TotalEarlyCancels)
	fmt.Fprintf(&b, "Total Number of Extensions: %v\n", gs.TotalExtensions)
	fmt.Fprintf(&b, "Total Reservation Time: %v\n", fmtDuration(gs.TotalDurationMinutes))
	return b.String()
}

func fmtDuration(t time.Duration) string {
	weeks := t / (time.Hour * 24 * 7)
	t -= weeks * (time.Hour * 24 * 7)
	days := t / (time.Hour * 24)
	t -= days * (time.Hour * 24)
	hours := t / time.Hour
	t -= hours * time.Hour
	minutes := t / time.Minute
	return fmt.Sprintf("%02d Weeks %02d days %02d hours %02d minutes", weeks, days, hours, minutes)
}
