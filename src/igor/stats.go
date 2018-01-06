// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	log "minilog"
	"os"
	"strconv"
	"strings"
	"time"
)

var cmdStats = &Command{
	UsageLine: "stats",
	Short:     "calculate statistics given a date range",
	Long: `
Show usage statistics for the given date range. Limited to log history (~3 months)

REQUIRED FLAGS:

The -d flag specifies the number of days to be included in the report, ending with today. e.g. igor stats -d 7 will display statistics for the previous 7 days.
	`,
}

var subD string // -d flag

func init() {
	// break init cycle
	cmdStats.Run = runStats

	cmdStats.Flag.StringVar(&subD, "d", "", "")

}

type Stats struct {
	// list of stats based on a user
	NumRes int
	NumNodes int
	TotalDurationMinutes int
}

// Use nmap to scan all the nodes and then show which are up and the
// reservations they below to
func runStats(_ *Command, _ []string) {

	// Parse flags
	duration := 0
	d, err := strconv.Atoi(subD)
	if err != nil {
		log.Fatalln("Invalid duration specified")
	}
	duration = d

	format := "2006/01/02"
	now := time.Now()
	start := now.AddDate(0, 0, -duration)
	fmt.Printf("Start: %v; Now: %v\n", start, now)

	// open and read in log file
	f, err := os.Open(igorConfig.LogFile)
	if err != nil {
		log.Fatal("Unable to read in log file: %v", err)
	}
	defer f.Close()
	// parse log
	s := bufio.NewScanner(f)
	statMap := map[string]Stats{} //user->Stats
	for s.Scan() {
		line := s.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		tStamp, err := time.Parse(format, fields[0])
		if err != nil {
			fmt.Printf("Error parsing time stamp: %v\n", err)
			continue
		}
		if tStamp.Before(start) {
			continue
		}
		if fields[4] != "INSTALL" {
			continue
		}
		var st Stats
		userField := fields[5]
		user := userField[5:]
		if s, ok := statMap[user]; ok  {
			st = s
		} else {
			st = Stats{}
		}
		st.NumRes++
		statMap[user] = st
		fmt.Printf("%v\n", tStamp)
	}

	// Build database
	fmt.Printf("%v\n",statMap)
}
