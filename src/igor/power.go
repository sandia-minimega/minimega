// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	log "minilog"
)

var cmdPower = &Command{
	UsageLine: "power [-r <reservation name>] [-n <nodes>] on/off/cycle",
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

func doPower(hosts []string, action string) error {
	log.Info("POWER	user=%v	nodes=%v	action=%v", igor.Username, hosts, action)

	switch action {
	case "off":
		if igor.PowerOffCommand == "" {
			return errors.New("power configuration missing")
		}

		return runAll(igor.PowerOffCommand, hosts)
	case "cycle":
		if igor.PowerOffCommand == "" {
			return errors.New("power configuration missing")
		}

		if err := runAll(igor.PowerOffCommand, hosts); err != nil {
			return err
		}

		fallthrough
	case "on":
		if igor.PowerOnCommand == "" {
			return errors.New("power configuration missing")
		}

		return runAll(igor.PowerOnCommand, hosts)
	}

	return fmt.Errorf("invalid power operation: %v", action)
}

// Turn a node on or off
func runPower(cmd *Command, args []string) {
	if (powerR == "") == (powerN == "") {
		log.Fatalln("must specify reservation or list of nodes")
	}

	if len(args) != 1 {
		log.Fatalln(cmdPower.UsageLine)
	}

	action := args[0]
	if action != "on" && action != "off" && action != "cycle" {
		log.Fatalln("must specify on, off, or cycle")
	}

	if powerR != "" {
		r := igor.Find(powerR)
		if r == nil {
			log.Fatal("reservation does not exist: %v", powerR)
		}

		if !r.IsActive(igor.Now) {
			log.Fatal("reservation is not active: %v", powerR)
		}

		if !r.IsWritable(igor.User) {
			log.Fatal("insufficient privileges to power %v reservation: %v", action, powerR)
		}

		// Detailed logging is happening in external
		fmt.Printf("Powering %s reservation %s\n", action, powerR)
		if err := doPower(r.Hosts, action); err != nil {
			log.Error("Error running power command: %v", err)
		}
		return
	}

	// Get the array of nodes the user specified
	nodes := igor.splitRange(powerN)
	if len(nodes) == 0 {
		log.Fatal("Couldn't parse node specification %v\n", subW)
	}

	active := igor.ActiveHosts(igor.Now)

	for _, node := range nodes {
		if _, ok := active[node]; !ok {
			log.Fatal("host is not reserved: %v", node)
		}

		if !active[node].IsWritable(igor.User) {
			log.Fatal("insufficient privileges to power %v node: %v", action, node)
		}
	}

	// Detailed logging is happening in external
	fmt.Printf("Powering %s nodes %s\n", action, powerN)
	if err := doPower(nodes, action); err != nil {
		log.Error("Error running power command: %v", err)
	}
}
