package main

import (
	"time"
)

// reservation object that igorweb.js understands
// an array of these is passed to client
// need to convert data to this structure in order to send it to client
type ResTableRow struct {
	Name  string
	Owner string
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

type ResTable []ResTableRow

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
