package main

import (
	"time"
)

// ResTableRow represents a Reservation that igorweb.js understands an
// array of these is passed to client need to convert data to this
// structure in order to send it to client
type ResTableRow struct {
	Name           string
	Owner          string
	Group          string
	CanEdit        bool
	Kernel         string
	Initrd         string
	CobblerProfile string
	KernelArgs     string
	// display string for "Start Time"
	Start string
	// integer start time for comparisons
	StartInt int64
	// display string for "End Time"
	End string
	// integer end time for comparisons
	EndInt int64
	// list of individual nodes in reservation
	// use RangeToInts for conversion from range
	Nodes []int
}

// ResTable is a list of ResTableRows
type ResTable []ResTableRow

// ContainsExpired returns true if any ResTableRow has an End time
// that is before the current wall time.
func (r ResTable) ContainsExpired() bool {
	now := time.Now().Unix()
	for i := 0; i < len(r); i++ {
		row := r[i]
		if row.EndInt < now {
			return true
		}
	}

	return false
}
