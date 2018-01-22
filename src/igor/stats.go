// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
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

The -d flag specifies the number of days to be included in the report, ending with today. e.g. igor stats -d 7 will display statistics for the previous 7 days (provided igor log goes back that far.)
	`,
}

var subD string // -d flag

func init() {
	// break init cycle
	cmdStats.Run = runStats
	cmdStats.Flag.StringVar(&subD, "d", "", "")

}

type ResData struct {
	ResName        string
	ResStart       time.Time
	ResEnd         time.Time
	ActualEnd      time.Time
	ActualDuration time.Duration
	Nodes          []string
}

type Stats struct { //global statistics
	NumRes               int
	NodesUsed            map[string]int
	NumNodes             int
	NumUsers             int
	TotalDurationMinutes time.Duration
	TotalEarlyCancels    int
}

var (
	formatDateStamp string = "2006/01/02"
	formatTimeStamp string = "15:04:05"
	formatLong      string = "2006-Jan-2-15:04"
	statStartDate   time.Time
	reservations    = map[string][]ResData{} //username->ResData
)

// Helper function to capture reservation data from logs and store for processing
func gatherInstallData(fields []string, s time.Time, e time.Time) ResData {
	var err error
	rd := ResData{}

	rd.ResStart = s
	rd.ResEnd = e
	// ActualEnd and ActualDuration fields are updated after
	// DELETED log entries are processed
	// NOTE: If we never encounter a DELETED entry for a corresponding INSTALL entry,
	// we calculate the reservation time during stats calculation (see RunStats)
	rd.ResName = strings.TrimLeft(fields[6], "resname=")

	// get nodes used in reservation TODO: Plot histogram of nodes used?
	nodes := strings.TrimLeft(fields[7], "nodes=")
	nodelist, err := ranges.SplitList(nodes)
	if err != nil {
		log.Fatal("%v", err)
	}
	rd.Nodes = nodelist
	user := strings.TrimLeft(fields[5], "user=")
	reservations[user] = append(reservations[user], rd)
	return rd
}

func gatherDeleteData(fields []string, s time.Time, e time.Time) {
	// The timestamp for the log indicates when this delete happened
	ad, err := time.Parse(formatDateStamp, fields[0])
	if err != nil {
		log.Fatal("%v", err)
	}
	at, err := time.Parse(formatTimeStamp, fields[1])
	if err != nil {
		log.Fatal("%v", err)
	}
	ae := time.Date(ad.Year(), ad.Month(), ad.Day(), at.Hour(), at.Minute(), at.Second(), 0, at.Location())
	fmt.Printf("ae: %v\nstatStartDate: %v\n", ae, statStartDate)
	// if it was deleted before our stat range, we don't care about this
	if ae.Before(statStartDate) {
		return
	}

	resName := strings.TrimLeft(fields[6], "resname=")
	user := strings.TrimLeft(fields[5], "user=")
	notFound := true
	for i, r := range reservations[user] {
		if r.ResStart == s && r.ResName == resName {
			// this is a delete for a res we know about
			r.ActualEnd = ae
			reservations[user][i] = r
			notFound = false
		}
	}
	if notFound {
		// We did not know about this reservation
		// the log was likely reset after it started
		rd := gatherInstallData(fields, s, e)
		rd.ActualEnd = ae
		rd.ActualDuration = ae.Sub(statStartDate)
		for i, r := range reservations[user] {
			if r.ResStart == s && r.ResName == resName {
				reservations[user][i] = rd
			}
		}
	}
}

// Parses igor log for reservation data and calculates usage statistic
// for the time duration specified. Output stats to standard out
func runStats(_ *Command, _ []string) {

	// Parse flags
	d, err := strconv.Atoi(subD)
	if err != nil {
		log.Fatalln("Invalid duration specified")
	}
	statStartDate = time.Now().AddDate(0, 0, -d)

	// open and read in log file
	f, err := os.Open(igorConfig.LogFile)
	if err != nil {
		log.Fatal("Unable to read in log file: %v", err)
	}
	defer f.Close()
	// parse log and build data structs
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		fields := strings.Fields(line)
		if len(fields) < 9 {
			continue
		}

		resStart, err := time.Parse(formatLong, strings.TrimLeft(fields[8], "start="))
		if err != nil {
			continue //not a log entry we care about
		}

		resEnd, err := time.Parse(formatLong, strings.TrimLeft(fields[9], "end="))
		if err != nil {
			log.Fatal("%v", err) //if this failed, something is wrong
		}

		if resEnd.Before(statStartDate) {
			continue // This ended before the range we care about
		}

		for _, a := range fields {
			if a == "INSTALL" {
				gatherInstallData(fields, resStart, resEnd)
			}
			if a == "DELETED" {
				gatherDeleteData(fields, resStart, resEnd)
			}
		}
	}

	// Calculate Stats
	globalStats := Stats{}
	globalStats.NodesUsed = make(map[string]int)
	for n, rd := range reservations {
		var uResCount, uEarlyCancel int
		var uDuration time.Duration

		globalStats.NumUsers += 1
		fmt.Printf("%v\n", n)
		for _, d := range rd {
			fmt.Printf("Res: %v\n", d.ResName)
			for _, n := range d.Nodes {
				fmt.Printf("%v\n", n)
				globalStats.NodesUsed[n] += 1
			}
			fmt.Printf("ResStart: %v, ResEnd: %v\n", d.ResStart, d.ResEnd)
			if d.ActualDuration.Minutes() == 0 { // we never saw this res get deleted
				if statStartDate.Before(d.ResStart) {
					uDuration += time.Now().Sub(d.ResStart)
				} else {
					uDuration += time.Now().Sub(statStartDate)
				}
			} else {
				uDuration += d.ActualDuration
			}
			uResCount += 1
			earlyCancel := false
			// fuzzy math here because igor log uses 2 different granularities of timestamps
			// so direct comparisons won't work
			//TODO: Fix timestamp inconsistencies in igor logs
			if (d.ResEnd.Sub(d.ActualEnd).Minutes()) < 1.0 {
				earlyCancel = true
				uEarlyCancel += 1
			}
			fmt.Printf("Actual End: %v, Actual Duration: %v\n", d.ActualEnd, uDuration)
			fmt.Printf("Early Cancel: %v\n\n", earlyCancel)
		}
		globalStats.NumRes += uResCount
		globalStats.TotalDurationMinutes += uDuration
		globalStats.TotalEarlyCancels += uEarlyCancel
	}
	for _, d := range globalStats.NodesUsed {
		if d > 0 {
			globalStats.NumNodes += 1
		}
	}

	//Repoort
	fmt.Printf("Total Users: %v\n", globalStats.NumUsers)
	fmt.Printf("Total Number of Reservations: %v\n", globalStats.NumRes)
	fmt.Printf("Total Number of Nodes Used: %v\n", globalStats.NumNodes)
	fmt.Printf("Total Number of Reservations Canceled Early: %v\n", globalStats.TotalEarlyCancels)
	fmt.Printf("Total Reservations Time: %v\n", globalStats.TotalDurationMinutes)
}
