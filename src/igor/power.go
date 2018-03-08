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

var cmdPower = &Command{
	UsageLine: "power [-r <reservation name>] [-n <nodes>] on/off",
	Short:     "power-cycle nodes or full reservation",
	Long: `
Power-cycle either a full reservation, or some nodes within a reservation owned by you.

Specify on or off to determine which power action should be taken.

Specify -r <reservation name> to indicate that the action should affect all nodes within the reservation.

Specify -n <nodes> to indicate that the action should affect the nodes listed. Only nodes in reservations belonging to you will be affected.
	`,
}

var powerR string
var powerN string

func init() {
	// break init cycle
	cmdPower.Run = runPower

	cmdPower.Flag.StringVar(&powerR, "r", "", "")
	cmdPower.Flag.StringVar(&powerN, "n", "", "")

}

// Turn a node off and on again.
func powerCycle(Hosts []string) {
	if igorConfig.AutoReboot {
		doPower(Hosts, "off")
		doPower(Hosts, "on")
	}
}

func doPower(hosts []string, action string) {
	user, err := getUser()
	if err != nil {
		log.Fatal("can't get current user: %v\n", err)
	}
	log.Info("POWER	user=%v	nodes=%v	action=%v", user.Username, hosts, action)
	var offcommand, oncommand string
	if igorConfig.PowerOffCommand != "" && igorConfig.PowerOnCommand != "" {
		// Use non-cobbler commands
		offcommand = igorConfig.PowerOffCommand
		oncommand = igorConfig.PowerOnCommand
	} else if igorConfig.UseCobbler {
		offcommand = "cobbler system poweroff --name %s"
		oncommand = "cobbler system poweron --name %s"
	} else {
		log.Fatal("no valid method of rebooting nodes available")
	}

	// Do it in parallel because this can take a while
	done := make(chan bool)
	f := func(h, commandformat string) {
		command := strings.Split(fmt.Sprintf(commandformat, h), " ")
		_, err := processWrapper(command...)
		if err != nil {
			log.Error("power command %v returned %v", command, err)
		}
		done <- true
	}
	for _, h := range hosts {
		if action == "off" {
			go f(h, offcommand)
		} else if action == "on" {
			go f(h, oncommand)
		}
	}
	for _, _ = range hosts {
		<-done
	}
}

// Turn a node on or off
func runPower(cmd *Command, args []string) {
	if len(args) != 1 {
		log.Fatalln(cmdPower.UsageLine)
	}
	action := args[0]
	if action != "on" && action != "off" {
		log.Fatalln("must specify on or off")
	}

	user, err := getUser()
	if err != nil {
		log.Fatal("can't get current user: %v\n", err)
	}

	if powerR != "" {
		// The user specified a reservation name
		found := false
		for _, r := range Reservations {
			if r.ResName == powerR && r.StartTime < time.Now().Unix() {
				found = true
				if r.Owner != user.Username {
					log.Fatal("You are not the owner of %v", powerR)
				} else if r.ResName == powerR {
					fmt.Printf("Powering %s reservation %s\n", action, powerR)
					doPower(r.Hosts, action)
				}
			}
		}
		if !found {
			log.Fatal("Cannot find an active reservation %v", powerR)
		}
	} else if powerN != "" {
		// The user specified some nodes. We need to verify they 'own' all those nodes.
		// Instead of looking through the reservations, we'll look at the current slice of the Schedule
		currentSched := Schedule[0]
		// Get the array of nodes the user specified
		rnge, _ := ranges.NewRange(igorConfig.Prefix, igorConfig.Start, igorConfig.End)
		nodes, _ := rnge.SplitRange(powerN)
		if len(nodes) == 0 {
			log.Fatal("Couldn't parse node specification %v\n", subW)
		}
		// make sure the range is valid
		if !checkValidNodeRange(nodes) {
			log.Fatalln("Invalid node range.")
		}

		// This will be the list of nodes to actually power on/off (in a user-owned reservation)
		var validatedNodes []string
		for _, n := range nodes {
			// Get rid of the prefix
			numstring := strings.TrimPrefix(n, igorConfig.Prefix)
			index, err := strconv.Atoi(numstring)
			if err != nil {
				log.Fatal("choked on a node named %v", n)
			}

			resID := currentSched.Nodes[index-1]
			for _, r := range Reservations {
				if r.ID == resID && r.Owner == user.Username {
					// Success! This node is in a reservation owned by the user
					validatedNodes = append(validatedNodes, n)
				}
			}
		}
		if len(validatedNodes) > 0 {
			unsplit, _ := rnge.UnsplitRange(validatedNodes)
			fmt.Printf("Powering %s nodes %s\n", action, unsplit)
			doPower(validatedNodes, action)
		} else {
			fmt.Printf("No nodes specified are in a reservation owned by the user\n")
		}
	}
}
