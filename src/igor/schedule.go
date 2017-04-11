package main

import (
	"errors"
	"fmt"
	"math/rand"
	log "minilog"
	"net"
	"time"
)

// Returns the indexes within the given array of all contiguous sets of '0' entries
func FindContiguousBlock(nodes []uint64, count int) ([]int, error) {
	result := []int{}
	for i := 0; i+count <= len(nodes); i++ {
		if IsFree(nodes, i, count) {
			result = append(result, i)
		}
	}
	if len(result) > 0 {
		return result, nil
	} else {
		return result, errors.New("no space available in this slice")
	}
}

// Returns true if nodes[index] through nodes[index+count-1] are free
func IsFree(nodes []uint64, index, count int) bool {
	for i := index; i < index+count; i++ {
		if nodes[index] != 0 {
			return false
		}
	}
	return true
}

func FindReservation(minutes, nodecount int) (Reservation, []TimeSlice) {
	return FindReservationAfter(minutes, nodecount, time.Now().Unix())
}

// Finds a slice of 'nodecount' nodes that's available for the specified length of time
// Returns a reservation and a slice of TimeSlices that can be used to replace
// the current Schedule if the reservation is acceptable.
// The 'after' parameter specifies a Unix timestamp that should be taken as the
// starting time for our search (this allows you to say "give me the first reservation
// after noon tomorrow")
func FindReservationAfter(minutes, nodecount int, after int64) (Reservation, []TimeSlice) {
	var res Reservation
	var newSched []TimeSlice

	slices := minutes / MINUTES_PER_SLICE
	if (minutes % MINUTES_PER_SLICE) != 0 {
		slices++
	}

	res.ID = uint64(rand.Int63())

	// We start with the *second* time slice, because the first is the current slice
	// and is partially consumed
	for i := 1; ; i++ {
		// Make sure the Schedule has enough time left in it
		if len(Schedule[i:])*MINUTES_PER_SLICE <= minutes {
			// This will guarantee we'll have enough space for the reservation
			ExtendSchedule(slices)
		}

		if Schedule[i].Start < after {
			continue
		}

		s := Schedule[i]
		// Check if there's any open blocks in this slice
		blocks, err := FindContiguousBlock(s.Nodes, nodecount)
		if err != nil {
			continue
		}

		// For each of the blocks...
		for _, b := range blocks {
			// Make a new starter schedule
			newSched = Schedule
			var nodenames []string
			for j := 0; j < slices; j++ {
				nodenames = []string{}
				// For simplicity, we'll end up re-checking the first slice, but who cares
				if !IsFree(newSched[i+j].Nodes, b, nodecount) {
					break
				} else {
					// Mark those nodes reserved
					for k := b; k < b+nodecount; k++ {
						newSched[i+j].Nodes[k] = res.ID
						nodenames = append(nodenames, fmt.Sprintf("%s%d", igorConfig.Prefix, igorConfig.Start+k))
					}
				}
			}
			// If we got this far, that means this block was free long enough
			// Now just fill out the rest of the reservation and we're all set
			var IPs []net.IP
			// First, go from node name to PXE filename
			for _, hostname := range nodenames {
				ip, err := net.LookupIP(hostname)
				if err != nil {
					log.Fatal("failure looking up %v: %v", hostname, err)
				}
				IPs = append(IPs, ip...)
			}
			// Now go IP->hex
			for _, ip := range IPs {
				res.PXENames = append(res.PXENames, toPXE(ip))
			}
			res.Hosts = nodenames
			res.StartTime = newSched[i].Start
			res.EndTime = res.StartTime + int64(minutes*60)
			res.Duration = time.Unix(res.EndTime, 0).Sub(time.Unix(res.StartTime, 0)).Minutes()
			goto Done
		}
	}
Done:
	return res, newSched
}

func InitializeSchedule() {
	// Create a 'starter'
	start := time.Now().Truncate(time.Minute * MINUTES_PER_SLICE) // round down
	end := start.Add((MINUTES_PER_SLICE-1)*time.Minute + 59*time.Second)
	size := igorConfig.End - igorConfig.Start + 1
	ts := TimeSlice{Start: start.Unix(), End: end.Unix()}
	ts.Nodes = make([]uint64, size)
	Schedule = []TimeSlice{ts}

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
	if len(Schedule)*MINUTES_PER_SLICE < MIN_SCHED_LEN {
		// Expand that schedule
		ExtendSchedule(MIN_SCHED_LEN - len(Schedule)*MINUTES_PER_SLICE)
	}
}

func ExtendSchedule(minutes int) {
	size := igorConfig.End - igorConfig.Start + 1 // size of node slice

	slices := minutes / MINUTES_PER_SLICE
	if (minutes % MINUTES_PER_SLICE) != 0 {
		// round up
		slices++
	}
	prev := Schedule[len(Schedule)-1]
	for i := 0; i < slices; i++ {
		// Everything's in Unix time, which is in units of seconds
		start := prev.End + 1 // Starts 1 second after the previous reservation ends
		end := start + (MINUTES_PER_SLICE-1)*60 + 59
		ts := TimeSlice{Start: start, End: end}
		ts.Nodes = make([]uint64, size)
		Schedule = append(Schedule, ts)
		prev = ts
	}
}
