// Copyright (2018) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.
package main

import (
	"path/filepath"
	"time"
)

// Represents a single reservation
type Reservation struct {
	ResName        string
	CobblerProfile string   // Optional; if set, use this Cobbler profile instead of a kernel+initrd
	Hosts          []string // separate, not a range
	PXENames       []string // eg C000025B
	StartTime      int64    // UNIX time
	EndTime        int64    // UNIX time
	Duration       float64  // minutes
	Owner          string
	ID             uint64
	KernelArgs     string
	Vlan           int
	Kernel         string
	Initrd         string
	KernelHash     string
	InitrdHash     string

	// Installed is set when the reservation is first installed
	Installed bool
}

// Filename returns the filename that stores the reservation configuration
func (r Reservation) Filename() string {
	return filepath.Join(igorConfig.TFTPRoot, "pxelinux.cfg", "igor", r.ResName)
}

// Active returns true if the reservation is active at the given time
func (r Reservation) Active(t time.Time) bool {
	start := time.Unix(r.StartTime, 0)
	end := time.Unix(r.EndTime, 0)

	return start.Before(t) && end.After(t)
}
