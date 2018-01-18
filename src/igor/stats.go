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

type Resdata struct {
	User      string
	ResStart  time.Time
	ResEnd    time.Time
	ActualEnd time.Time
	Duration  int
	Nodes     []string
}

type Stats struct {
	// list of stats based on a user
	NumRes               int
	NodesUsed            map[string]bool
	NumNodes             int
	TotalDurationMinutes int
}

var (
	reservations = map[string][]Resdata{}
)

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

	formatshort := "2006/01/02"
	formatlong := "2006-Jan-2-15:04"
	now := time.Now()
	start := now.AddDate(0, 0, -duration)
	//fmt.Printf("Start: %v; Now: %v\n", start, now)

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
		tStamp, err := time.Parse(formatshort, fields[0])
		if err != nil {
			//fmt.Printf("Error parsing time stamp: %v\n", err)
			continue
		}
		if tStamp.Before(start) {
			continue
		}

		for _, a := range fields {
			if a == "INSTALL" {
				var st Stats
				var err error
				data := Resdata{}
				data.User = strings.TrimLeft(fields[5], "user=")
				resname := strings.TrimLeft(fields[6], "resname=")
				nodes := strings.TrimLeft(fields[7], "nodes=")
				data.ResStart, err = time.Parse(formatlong, strings.TrimLeft(fields[8], "start="))
				if err != nil {
					log.Fatal("%v", err)
				}
				data.ResEnd, err = time.Parse(formatlong, strings.TrimLeft(fields[9], "end="))
				if err != nil {
					log.Fatal("%v", err)
				}
				data.Duration, err = strconv.Atoi(strings.TrimLeft(fields[10], "duration="))
				if err != nil {
					log.Fatal("Error converting duration: %v", err)
				}
				if s, ok := statMap[data.User]; ok {
					st = s
				} else {
					st = Stats{}
					st.NodesUsed = make(map[string]bool)
				}
				st.NumRes++
				st.TotalDurationMinutes += duration
				nodelist, err := ranges.SplitList(nodes)
				if err != nil {
					log.Fatal("%v", err)
				}
				for _, n := range nodelist {
					if st.NodesUsed[n] != true {
						st.NodesUsed[n] = true
						st.NumNodes += 1
					}
				}
				data.Nodes = nodelist
				statMap[data.User] = st
				reservations[resname] = append(reservations[resname], data)
			}
		}
		//fmt.Printf("%v\n", tStamp)
	}

	// Build database
	for k,v := range reservations {
		fmt.Printf("\n%v\n", k)
		for _, d := range v {
			fmt.Printf("%v\n",d)
		}
	}
	fmt.Printf("\n%v\n", statMap)
}
