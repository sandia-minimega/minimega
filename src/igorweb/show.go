package main

import (
	log "minilog"
	"ranges"
	"time"
)

// Show stores data received from "igor show -json"
type Show struct {
	LastUpdated                                 time.Time
	Prefix                                      string
	RangeStart, RangeEnd, RackWidth, RackHeight int
	Available, Down                             []string
	Reservations                                []Res
}

// Returns the range of nodes based on Show
func (s Show) Range() *ranges.Range {
	rnge, err := ranges.NewRange(s.Prefix, s.RangeStart, s.RangeEnd)
	if err != nil {
		log.Fatal("Cannot compute Range: %v", err)
	}
	return rnge
}

// DownRow returns a ResTableRow that enumerates all nodes in the "down" state
func (s Show) DownRow() ResTableRow {
	return ResTableRow{
		StartInt: time.Now().Unix(),
		EndInt:   0,
		Nodes:    s.Range().RangeToInts(s.Down),
	}
}

// ResTable returns the reservations as a ResTable. The first element
// in the ResTable enumerates the hosts that are "down"
func (s *Show) ResTable() ResTable {
	// First element is a row containing all down nodes
	resRows := ResTable{s.DownRow()}
	for _, r := range s.Reservations {
		resRows = append(resRows, r.ResTableRow(s))
	}
	return resRows
}

// Res represents a Reservation. It's for unmarshalling
// igor.Reservations from "igor show -json". However, the number of
// fields in Res is less than igor.Reservation, since we don't need
// everything there.
type Res struct {
	Name  string
	Owner string
	Group string
	Start time.Time
	End   time.Time
	Hosts []string // separate, not a range
}

// ResTableRow returns a Res as a ResTableRow.
func (r Res) ResTableRow(s *Show) ResTableRow {
	timefmt := "Jan 2 15:04"

	return ResTableRow{
		Name:     r.Name,
		Owner:    r.Owner,
		Group:    r.Group,
		Start:    r.Start.Format(timefmt),
		StartInt: r.Start.UnixNano(),
		End:      r.End.Format(timefmt),
		EndInt:   r.End.UnixNano(),
		Nodes:    s.Range().RangeToInts(r.Hosts),
	}
}
