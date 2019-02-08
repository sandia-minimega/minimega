// Copyright (2019) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
package main

import (
	"errors"
	"fmt"
	log "minilog"
	"os"
)

var Reservations = make(map[uint64]*Reservation) // map ID to reservations

// FindReservation by name
func FindReservation(s string) *Reservation {
	for _, r := range Reservations {
		if r.ResName == s {
			if r.InstallError != "" {
				log.Warn("reservation has install error: %v", r.InstallError)
			}

			return r
		}
	}

	return nil
}

func DeleteReservationByName(s string) error {
	for id, r := range Reservations {
		if r.ResName == s {
			return DeleteReservation(id)
		}
	}

	return fmt.Errorf("reservation does not exist: %v", s)
}

func DeleteReservation(id uint64) error {
	r, ok := Reservations[id]
	if !ok {
		// that's strange...
		return errors.New("invalid reservation ID")
	}

	// purge it from the schedule
	for i := range Schedule {
		for j := range Schedule[i].Nodes {
			if Schedule[i].Nodes[j] == r.ID {
				Schedule[i].Nodes[j] = 0
			}
		}
	}

	// clean up the network config
	if err := networkClear(r.Hosts); err != nil {
		return fmt.Errorf("error clearing network isolation: %v", err)
	}

	// unset cobbler or TFTP configuration
	if err := GetBackend().Uninstall(r); err != nil {
		return fmt.Errorf("unable to uninstall reservation: %v", err)
	}

	// We use this to indicate if a reservation has been created or not
	// It's used with Cobbler too, even though we don't manually manage PXE files.
	os.Remove(r.Filename())

	if err := r.PurgeFiles(); err != nil {
		return fmt.Errorf("unable to purge files: %v", err)
	}

	// Finally, purge it from the reservations
	delete(Reservations, r.ID)
	dirty = true

	emitReservationLog("DELETED", r)

	return nil
}
