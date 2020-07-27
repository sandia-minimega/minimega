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
	// collect report and print at the end
	report := make(map[string]map[string]map[string]string)

	// first get Arista vlan data
	fmt.Println("Retrieving Arista data, this may take a few moments...")
	gt, err := networkVlan()
	if err != nil {
		log.Fatal("Error gathering VLAN data from Arista")
	}

	// we need to know which nodes are up or down
	names := igor.validHosts()
	// Maps a node's index to a boolean value (up = true, down = false)
	powered := map[int]bool{}
	nodes, err := scanNodes(names)
	if err != nil {
		log.Fatal("unable to scan: %v", err)
	}
	powered = nodes

	// TODO: probably shouldn't iterate over .M directly
	for _, r := range igor.Reservations.M {
		if !r.IsActive(igor.Now) {
			continue
		}
		// each reservation can have multiple hosts, but only 1 vlan
		resnodes := make(map[string]map[string]string)
		mismatch := false
		for _, host := range r.Hosts {
			vlan := strconv.Itoa(r.Vlan)
			gtvlan := gt[host]
			aristavlan := gtvlan
			status := ""
			// if reservation does not match arista, color the value red to print
			if gtvlan != vlan {
				vlan = FgRed + vlan + Reset
				mismatch = true
			}
			// if arista had no vlan assigned, make explicit for readability
			if gtvlan == "" {
				aristavlan = "(none)"
			}
			// separate host number from prefix to compare against powered map
			hostnum, err := strconv.Atoi(host[len(igor.Prefix):])
			if err != nil {
				//that's weird
				continue
			}
			// if node is powered off, add to node status
			if !powered[hostnum] {
				status = status + " - powered off"
			}

			resnodes[host] = map[string]string{"igor": vlan, "arista": aristavlan, "status": status}

		}
		// if force flag given and a discrepancy is detected between reservation and
		// arista, then set node in arista with vlan value from the reservation
		if force && mismatch {
			if err := networkSet(r.Hosts, r.Vlan); err != nil {
				log.Fatal("unable to set up network isolation")
				for _, host := range r.Hosts {
					resnodes[host]["status"] = resnodes[host]["status"] + " - Arista set command failed!"
				}
			} else {
				for _, host := range r.Hosts {
					resnodes[host]["status"] = resnodes[host]["status"] + " - Arista set command executed"
				}
			}
		}
		report[r.Name] = resnodes
	}

	// create writer
	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 0, 1, ' ', 0)

	// HEADER
	// need to print column labels as variables for format/alignment consistency
	n := "NODE"
	i := "IGOR"
	a := "ARISTA"
	// print report
	for resname, res := range report {
		header := false
		if !quiet {
			fmt.Fprintln(w, "\nReservation: ", resname)
			fmt.Fprintln(w, n, "\t", i, "  ", a)
		}
		for nodename, node := range res {
			if !quiet {
				fmt.Fprintln(w, nodename, "\t", node["igor"], "  ", node["arista"]+node["status"])
			} else {
				if node["igor"] != node["arista"] {
					if !header {
						fmt.Fprintln(w, "\nReservation: ", resname)
						fmt.Fprintln(w, n, "\t", i, "  ", a)
						header = true
					}
					fmt.Fprintln(w, nodename, "\t", node["igor"], "  ", node["arista"]+node["status"])
				}
			}
		}

	}
	w.Flush()
}
