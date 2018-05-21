// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/gob"
	"encoding/json"
	"io"
	"io/ioutil"
	log "minilog"
	"os"
	"path/filepath"
)

// readData reads the Reservations and Schedule from a gob-encoded file. If the
// file is empty (newly created), it initializes the Reservations and Schedule
// to a clean slate.
func readData(f *os.File) {
	var data struct {
		Reservations map[uint64]Reservation
		Schedule     []TimeSlice
	}

	if err := gob.NewDecoder(f).Decode(&data); err == nil {
		// copy to globals
		Reservations = data.Reservations
		Schedule = data.Schedule
	} else if err == io.EOF {
		// init to usable defaults
		log.Warn("no previous reservations")
		Reservations = make(map[uint64]Reservation)
	} else {
		log.Fatal("unable to load data: %v", err)
	}
}

// writeData writes the Reservations and Schedule to f using an intermediate
// file to ensure that the update is all-or-nothing.
func writeData(f *os.File) {
	data := struct {
		Reservations map[uint64]Reservation
		Schedule     []TimeSlice
	}{
		Reservations,
		Schedule,
	}

	tmp, err := ioutil.TempFile("/tmp", "igor")
	if err != nil {
		log.Fatal("unable to create tmp file: %v", err)
	}
	defer tmp.Close()

	if err := gob.NewEncoder(tmp).Encode(data); err != nil {
		log.Fatal("unable to encode data: %v", err)
	}

	if err := tmp.Close(); err != nil {
		log.Fatal("update failed: %v", err)
	}

	if err := os.Rename(tmp.Name(), f.Name()); err != nil {
		log.Fatal("update failed: %v", err)
	}
}

// writeReservations writes just the JSON-encoded Reservations to the
// reservation file based on igorConfig. Since this file is just a mirror for
// the UI, we don't need to make it all-or-nothing.
func writeReservations() {
	path := filepath.Join(igorConfig.TFTPRoot, "/igor/reservations.json")

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0664)
	if err != nil {
		log.Fatal("failed to open file %v: %v", path, err)
	}
	defer f.Close()

	if err := json.NewEncoder(f).Encode(Reservations); err != nil {
		log.Fatal("unable to encode reservations: %v", err)
	}
}
