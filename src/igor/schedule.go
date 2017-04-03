package main

import (
	"time"
	"errors"
)

// Returns the index within the given array a contiguous set of '0' entries
func FindContiguousBlock(nodes []uint64, count int) (int, error) {
	var i, j int
	for i = 0; i < len(nodes); i++ {
		if nodes[i] == 0 {
			for j = i; j < len(nodes); j++ {
				// success
				if (j - i) == count {
					return i, nil
				}
				if nodes[j] != 0 {
					break
				}
			}
		}
	}
	return 0, errors.New("no space available in this slice")
}

func FindReservationSlot(duration, nodecount int) (Reservation, []*TimeSlice) {
	return Reservation{}, nil
}

func InitializeSchedule() {
	// Create a 'starter'
	start := time.Now().Truncate(time.Minute*MINUTES_PER_SLICE) // round down
	end := start.Add((MINUTES_PER_SLICE-1)*time.Minute + 59*time.Second)
	size := igorConfig.End - igorConfig.Start
	ts := &TimeSlice{ Start: start.Unix(), End: end.Unix()}
	ts.Nodes = make([]uint64, size)
	Schedule = []*TimeSlice{ts}

	// Now expand it to fit the minimum size we want
	ExtendSchedule(MIN_SCHED_LEN - MINUTES_PER_SLICE) // we already have one slice so subtract
}

func ExpireSchedule() {
	// If the last element of the schedule is expired, or it's empty, let's start fresh
	if len(Schedule) == 0 || Schedule[len(Schedule)-1].End < time.Now().Unix() {
		InitializeSchedule()
	}

	// Get rid of any outdated TimeSlices
	for i, t := range Schedule {
		if t.End > time.Now().Unix() {
			Schedule = Schedule[i:]
			break
		}
	}

	// Now make sure we have at least the minimum length there
	if (len(Schedule)*MINUTES_PER_SLICE < MIN_SCHED_LEN) {
		// Expand that schedule
		ExtendSchedule(MIN_SCHED_LEN - len(Schedule)*MINUTES_PER_SLICE)
	}
}

func ExtendSchedule(minutes int) {
	size := igorConfig.End - igorConfig.Start // size of node slice

	slices := int(minutes / MINUTES_PER_SLICE)
	if (minutes % MINUTES_PER_SLICE) != 0 {
		// round up
		slices++
	}
	prev := Schedule[len(Schedule)-1]
	for i := 0; i < slices; i++ {
		// Everything's in Unix time, which is in units of seconds
		start := prev.End + 1 // Starts 1 second after the previous reservation ends
		end := start + (MINUTES_PER_SLICE-1)*60 + 59
		ts := &TimeSlice{ Start: start, End: end }
		ts.Nodes = make([]uint64, size)
		Schedule = append(Schedule, ts)
		prev = ts
	}
}