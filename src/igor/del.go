// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	log "minilog"
	"os"
	"path/filepath"
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

// The checkUser argument specifies whether or not we should compare the current
// username to the username of the deleted reservation. It is set to 'true' when
// a reservation is deleted at the command line, and 'false' when the reservation
// is deleted because it has expired.
func deleteReservation(checkUser bool, args []string) {
	var deletedReservation Reservation

	if len(args) != 1 {
		log.Fatalln("Invalid arguments")
	}

	user, err := getUser()
	if err != nil {
		log.Fatal("can't get current user: %v\n", err)
	}

	if checkUser {
		for _, r := range Reservations {
			if r.ResName == args[0] && r.Owner != user.Username {
				log.Fatal("You are not the owner of %v", args[0])
			}
		}
	}

	// Remove the reservation
	found := false
	for _, r := range Reservations {
		if r.ResName == args[0] {
			deletedReservation = r
			delete(Reservations, r.ID)
			found = true
		}
	}
	if !found {
		log.Fatal("Couldn't find reservation %v", args[0])
	}

	// Now purge it from the schedule
	for i, _ := range Schedule {
		for j, _ := range Schedule[i].Nodes {
			if Schedule[i].Nodes[j] == deletedReservation.ID {
				Schedule[i].Nodes[j] = 0
			}
		}
	}

	// clean up the network config
	if err := networkClear(deletedReservation.Hosts); err != nil {
		log.Fatal("error clearing network isolation: %v", err)
	}

	if err := GetBackend().Uninstall(deletedReservation); err != nil {
		log.Fatal("unable to uninstall reservation: %v", err)
	}

	// We use this to indicate if a reservation has been created or not
	// It's used with Cobbler too, even though we don't manually manage PXE files.
	os.Remove(filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", deletedReservation.ResName))

	// If no other reservations are using them, delete the kernel and/or initrd
	var ifound, kfound bool
	for _, r := range Reservations {
		if r.InitrdHash == deletedReservation.InitrdHash {
			ifound = true
		}
		if r.KernelHash == deletedReservation.KernelHash {
			kfound = true
		}
	}
	if !ifound {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", deletedReservation.InitrdHash+"-initrd")
		os.Remove(fname)
	}
	if !kfound {
		fname := filepath.Join(igorConfig.TFTPRoot, "igor", deletedReservation.KernelHash+"-kernel")
		os.Remove(fname)
	}

	emitReservationLog("DELETED", deletedReservation)

	dirty = true
}
