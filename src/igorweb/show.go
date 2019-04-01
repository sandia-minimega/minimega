package main

import (
	"ranges"
	"time"
)

type Show struct {
	Prefix                                      string
	RangeStart, RangeEnd, RackWidth, RackHeight int
	Available, Down                             []string
	Reservations                                []Res
}

func (s Show) DownRow() ResTableRow {
	rnge, _ := ranges.NewRange(s.Prefix, s.RangeStart, s.RangeEnd)
	return ResTableRow{
		"",
		"",
		"",
		time.Now().Unix(),
		"",
		0,
		rnge.RangeToInts(s.Down),
	}
}

func (s Show) ResTable() ResTable {
	// First element is a row containing all down nodes
	resRows := ResTable{s.DownRow()}
	for _, r := range s.Reservations {
		resRows = append(resRows, r.ResTableRow(s.Prefix, s.RangeStart, s.RangeEnd))
	}
	return resRows
}

type Res struct {
	Name  string
	Owner string
	Start time.Time
	End   time.Time
	Hosts []string // separate, not a range
}

func (r Res) ResTableRow(prefix string, start, end int) ResTableRow {
	timefmt := "Jan 2 15:04"
	rnge, _ := ranges.NewRange(prefix, start, end)

	return ResTableRow{
		Name:     r.Name,
		Owner:    r.Owner,
		Start:    r.Start.Format(timefmt),
		StartInt: r.Start.UnixNano(),
		End:      r.End.Format(timefmt),
		EndInt:   r.End.UnixNano(),
		Nodes:    rnge.RangeToInts(r.Hosts),
	}
}
