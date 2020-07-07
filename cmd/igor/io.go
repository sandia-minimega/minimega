// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// readData reads the Reservations a json-encoded file. If the file is empty
// (newly created), it initializes the Reservations to a clean slate.
func (r *Reservations) readData(f *os.File) error {
	if err := json.NewDecoder(f).Decode(&r.M); err == io.EOF {
		// init to usable defaults
		log.Warn("no previous reservations")
		r.M = make(map[uint64]*Reservation)
	} else if err != nil {
		return fmt.Errorf("unable to load data: %v", err)
	}

	return nil
}

// writeData writes the Reservations to f using an intermediate file to ensure
// that the update is all-or-nothing.
func (r *Reservations) writeData(f *os.File) error {
	// TODO: we should remove any leftover tmpdata files
	tpath := filepath.Join(igor.TFTPRoot, "igor")
	tmp, err := ioutil.TempFile(tpath, "tmpdata")
	if err != nil {
		return fmt.Errorf("unable to create tmp file: %v", err)
	}
	defer tmp.Close()

	if err := json.NewEncoder(tmp).Encode(r.M); err != nil {
		return fmt.Errorf("unable to encode data: %v", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("update failed: %v", err)
	}

	if err := os.Rename(tmp.Name(), f.Name()); err != nil {
		return fmt.Errorf("update failed: %v", err)
	}

	// make reservations file world-readable
	if err := os.Chmod(f.Name(), 0644); err != nil {
		return fmt.Errorf("update failed: %v", err)
	}

	return nil
}
