package main

import (
	"encoding/json"
	"os"
	"syscall"
)

var cmdDel = &Command{
	UsageLine: "del <reservation name>",
	Short:	"delete reservation",
	Long:`
Delete an existing reservation.
	`,
}

func init() {
	// break init cycle
	cmdDel.Run = runDel
}

// Remove the specified reservation.
func runDel(cmd *Command, args []string) {
	path := igorConfig.TFTPRoot + "/igor/reservations.json"
	resdb, err := os.OpenFile(path, os.O_RDWR, 664)
	if err != nil {
		fatalf("failed to open reservations file: %v", err)
	}
	defer resdb.Close()
	err = syscall.Flock(int(resdb.Fd()), syscall.LOCK_EX)
	defer syscall.Flock(int(resdb.Fd()), syscall.LOCK_UN)	// this will unlock it later
	reservations := getReservations(resdb)

	var newres []Reservation
	var deletedReservation Reservation
	found := false
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

	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(newres)
	resdb.Sync()

	// Delete all the PXE files in the reservation
	for _, pxename := range deletedReservation.PXENames {
		os.Remove(igorConfig.TFTPRoot+"/pxelinux.cfg/" + pxename)
	}
}
