// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	log "minilog"
	"os"
	"strconv"
	"text/tabwriter"
)

var cmdSync = &Command{
	UsageLine: "sync",
	Short:     "synchronize igor data",
	Long: `
Does an internal check to verify the integrity of the data file and produces a report. Can also attempt to force a sync.
SYNOPSIS
	igor sync [-f] [-q] WHAT
OPTIONS
	-f, -force
	    If an inconsistentcy is found between the reservation and WHAT, will command WHAT to set its relevant attribute to match the reservation's value in addition to reporting
	-q, -quiet
	    Suppress report of all data, only reports inconsistencies
Possible WHATs:
arista: 	reconfigure switchports for active reservations
	`,
}

var subF bool  // -f
var force bool // -force
var subQ bool  // -q
var quiet bool // -quiet

func init() {
	// break init cycle
	cmdSync.Run = runSync

	cmdSync.Flag.BoolVar(&subF, "f", false, "")
	cmdSync.Flag.BoolVar(&force, "force", false, "")
	cmdSync.Flag.BoolVar(&subQ, "q", false, "")
	cmdSync.Flag.BoolVar(&quiet, "quiet", false, "")
}

// Gather data integrity information, report, and fix
func runSync(cmd *Command, args []string) {
	// process flags
	force = (force || subF)
	quiet = (quiet || subQ)

	if igor.Username != "root" {
		log.Fatalln("Sync access restricted. Please use as admin.")
	}

	if len(args) != 1 {
		log.Fatalln("Invalid arguments")
	}

	switch args[0] {
	case "arista":
		syncArista()
	default:
		log.Fatalln("Invalid arguments")
	}
}

func syncArista() {
	// first get ground truth
	fmt.Println("Retrieving Arista data, this may take a few moments...")
	gt, err := networkVlan()
	if err != nil {
		log.Fatal("Error gathering VLAN data from Arista")
	}
	// create writer
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	// HEADER
	// need to print column labels as variables for format/alignment consistency
	n := "NODE"
	i := "IGOR"
	a := "ARISTA"
	fmt.Println("")
	fmt.Fprintln(w, n, "\t", i, "  ", a)

	// TODO: probably shouldn't iterate over .M directly
	for _, r := range igor.Reservations.M {
		if !r.IsActive(igor.Now) {
			continue
		}
		// each reservation can have multiple hosts, but only 1 vlan
		mismatch := false
		for _, host := range r.Hosts {
			vlan := strconv.Itoa(r.Vlan)
			gtvlan := gt[host]
			// if reservation does not match arista, color the value red to print
			if gtvlan != vlan {
				vlan = FgRed + vlan + Reset
				mismatch = true
			}
			// if arista had no vlan assigned, make explicit for readability
			if gtvlan == "" {
				gtvlan = "(none)"
			}
			if !quiet {
				fmt.Fprintln(w, host, "\t", vlan, "  ", gtvlan)
			} else {
				if mismatch {
					fmt.Fprintln(w, host, "\t", vlan, "  ", gtvlan)
				}
			}
		}
		// if force flag given and a discrepancy is detected between reservation and
		// arista, then set node in arista with vlan value from the reservation
		if force && mismatch {
			if err := networkSet(r.Hosts, r.Vlan); err != nil {
				log.Fatal("unable to set up network isolation")
			}
		}
	}
	w.Flush()
}
