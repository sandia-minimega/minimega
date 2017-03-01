// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
)

var cmdDel = &Command{
	UsageLine: "del <reservation name>",
	Short:     "delete reservation",
	Long: `
Delete an existing reservation.
	`,
}

func init() {
	// break init cycle
	cmdDel.Run = runDel
}

// Remove the specified reservation.
func runDel(cmd *Command, args []string) {
	deleteReservation(true, args)
}

func deleteReservation(checkUser bool, args []string) {
	if len(args) != 1 {
		fatalf("Invalid arguments")
	}

	user, err := user.Current()
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/reservations.json")
	resdb, err := os.OpenFile(path, os.O_RDWR, 664)
	if err != nil {
		fatalf("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN) // this will unlock it later
	reservations := getReservations(resdb)

	var newres []Reservation
	var deletedReservation Reservation
	found := false
	if checkUser {
		for _, r := range reservations {
			if r.ResName == args[0] && r.Owner != user.Username {
				fatalf("You are not the owner of %v", args[0])
			}
		}
	}
	for _, r := range reservations {
		if r.ResName != args[0] {
			newres = append(newres, r)
		} else {
			deletedReservation = r
			found = true
		}
	}

	if !found {
		fatalf("Couldn't find reservation %v", args[0])
	}

	// clean up the network config
	err = networkClear(deletedReservation.Hosts)
	if err != nil {
		fatalf("error clearing network isolation: %v", err)
	}

	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(newres)
	resdb.Sync()

	// Delete all the PXE files in the reservation
	for _, pxename := range deletedReservation.PXENames {
		os.Remove(igorConfig.TFTPRoot + "/pxelinux.cfg/" + pxename)
	}
}
