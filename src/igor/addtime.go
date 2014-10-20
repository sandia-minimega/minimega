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

var cmdAddtime = &Command{
	UsageLine: "addtime <reservation name> <hours>",
	Short:     "Add time to a reservation",
	Long: `
Add time to a reservation.

-r specifies reservation

-t specifies how many hours should be added
	`,
}

var addtimeR string
var addtimeT int

func init() {
	// break init cycle
	cmdAddtime.Run = runAddtime

	cmdAddtime.Flag.StringVar(&addtimeR, "r", "", "")
	cmdAddtime.Flag.IntVar(&addtimeT, "t", 0, "")
}

// Remove the specified reservation.
func runAddtime(cmd *Command, args []string) {
	// validate arguments
	if addtimeR == "" || addtimeT <= 0 {
		errorf("Missing required argument!")
		help([]string{"addtime"})
		exit()
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

	found := false
	for _, r := range reservations {
		if r.ResName == addtimeR && r.Owner != user.Username {
			fatalf("You are not the owner of %v", addtimeR)
		}
	}
	for i, r := range reservations {
		if r.ResName == addtimeR {
			reservations[i].Expiration += int64(addtimeT * 60 * 60)
			found = true
		}
	}

	if !found {
		fatalf("Couldn't find reservation %v", addtimeR)
	}

	// Truncate the existing reservation file
	resdb.Truncate(0)
	resdb.Seek(0, 0)
	// Write out the new reservations
	enc := json.NewEncoder(resdb)
	enc.Encode(reservations)
	resdb.Sync()
}
