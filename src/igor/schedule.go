package main

import (
	"errors"
	"fmt"
	"math/rand"
	log "minilog"
	"net"
	"strconv"
	"strings"
	"time"
)

// Checks if node numbers are consistent with the range specified in an igor configuration file.
// Returns true if the range is valid and false if invalid.
// Note that an empty list is considered to have a valid node range since all nodes specified
// (in this case none) fall within the proper range.
func checkValidNodeRange(nodes []string) bool {
	indexes, err := getNodeIndexes(nodes)
	if err != nil {
		log.Fatal("Unable to get node indexes: %v", err)
	} else if len(indexes) == 0 {
		// An empty list has a valid node range.
		return true
	}
	return !(indexes[len(indexes)-1] > igorConfig.End-1 || indexes[0] < igorConfig.Start-1)
}

// Returns the node numbers within the given array of nodes of all contiguous sets of '0' entries
// Basically: figures out where in the list of nodes there are 'count' unallocated nodes.
func findContiguousBlock(nodes []uint64, count int) ([][]int, error) {
	result := [][]int{}
	for i := 0; i+count <= len(nodes); i++ {
		if isFree(nodes, i, count) {
			inner := []int{}
			for j := 0; j < count; j++ {
				inner = append(inner, i+j)
			}
			result = append(result, inner)
		}
	}
	if len(result) > 0 {
		return result, nil
	} else {
		return result, errors.New("no space available in this slice")
	}
}

// Checks if the nodes at requestedindexes within the clusternodes array are all set to 0,
// meaning nothing has reserved them.
func areNodesFree(clusternodes []uint64, requestedindexes []int) bool {
	for _, idx := range requestedindexes {
		if !isFree(clusternodes, idx, 1) {
			return false
		}
	}
	return true
}

// Returns true if nodes[index] through nodes[index+count-1] are free (set to 0)
func isFree(nodes []uint64, index, count int) bool {
	for i := index; i < index+count; i++ {
		if nodes[i] != 0 {
			return false
		}
	}
	return true
}

// Find the first available reservation of 'minutes' length and 'nodecount' nodes.
func findReservation(minutes, nodecount int) (Reservation, []TimeSlice, error) {
	return findReservationAfter(minutes, nodecount, time.Now().Unix())
}

// Finds a slice of 'nodecount' nodes that's available for the specified length of time
// Returns a reservation and a slice of TimeSlices that can be used to replace
// the current Schedule if the reservation is acceptable.
// The 'after' parameter specifies a Unix timestamp that should be taken as the
// starting time for our search (this allows you to say "give me the first reservation
// after noon tomorrow")
func findReservationAfter(minutes, nodecount int, after int64) (Reservation, []TimeSlice, error) {
	return findReservationGeneric(minutes, nodecount, []string{}, false, after)
}

// Helper function to convert string list of nodes into ints
func getNodeIndexes(requestedNodes []string) ([]int, error) {
	var requestedindexes []int
	for _, hostname := range requestedNodes {
		ns := strings.TrimPrefix(hostname, igorConfig.Prefix)
		n, err := strconv.Atoi(ns)
		if err != nil {
			return requestedindexes, errors.New("invalid hostname " + hostname)
		}
		requestedindexes = append(requestedindexes, n-igorConfig.Start)
	}
	return requestedindexes, nil
}

// Finds a slice of 'nodecount' nodes that's available for the specified length of time
// Returns a reservation and a slice of TimeSlices that can be used to replace
// the current Schedule if the reservation is acceptable.
// The 'after' parameter specifies a Unix timestamp that should be taken as the
// starting time for our search (this allows you to say "give me the first reservation
// after noon tomorrow")
// 'requestednodes' = a list of node names
// 'specific' = true if we want the nodes in requestednodes rather than a range
func findReservationGeneric(minutes, nodecount int, requestednodes []string, specific bool, after int64) (Reservation, []TimeSlice, error) {
	var res Reservation
	var err error
	var newSched []TimeSlice

	slices := minutes / MINUTES_PER_SLICE
	if (minutes % MINUTES_PER_SLICE) != 0 {
		slices++
	}

	// convert hostnames to indexes
	requestedindexes, err := getNodeIndexes(requestednodes)

	res.ID = uint64(rand.Int63())

	// We start with the current time slice, even though it is partially consumed
	// This is to keep the reservation from starting 1 minute into the future
	for i := 0; ; i++ {
		// Make sure the Schedule has enough time left in it
		if len(Schedule[i:])*MINUTES_PER_SLICE <= minutes {
			// This will guarantee we'll have enough space for the reservation
			extendSchedule(minutes)
		}

		if Schedule[i].Start < after {
			continue
		}

		s := Schedule[i]
		// 'blocks' is a slice of cluster segments which are not allocated yet; in other words, potential sets of nodes to allocate
		var blocks [][]int
		// Check if there's any open blocks in this slice
		if specific {
			if areNodesFree(s.Nodes, requestedindexes) {
				blocks = [][]int{requestedindexes}
			} else {
				continue
			}
		} else {
			blocks, err = findContiguousBlock(s.Nodes, nodecount)
			if err != nil {
				continue
			}
		}

		// For each of the blocks...
	BlockLoop:
		for _, b := range blocks {
			// Make a new starter schedule
			newSched = Schedule
			var nodenames []string
			// Make sure the block we're checking is free for as long as we need it.
			for j := 0; j < slices; j++ {
				nodenames = []string{}
				// For simplicity, we'll end up re-checking the first slice, but who cares
				// If the block of nodes we're looking at are available, mark them as 'ours', otherwise
				// move on to the next block
				if !areNodesFree(newSched[i+j].Nodes, b) {
					continue BlockLoop
				} else {
					// Mark those nodes reserved
					//for k := b; k < b+nodecount; k++ {
					for _, idx := range b {
						newSched[i+j].Nodes[idx] = res.ID
						fmtstring := "%s%0" + strconv.Itoa(igorConfig.Padlen) + "d"
						nodenames = append(nodenames, fmt.Sprintf(fmtstring, igorConfig.Prefix, igorConfig.Start+idx))
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
	return res, newSched, nil
}

// Create an empty schedule, in case we don't have one or the old one completely expired.
func initializeSchedule() {
	// Create a 'starter'
	start := time.Now().Truncate(time.Minute * MINUTES_PER_SLICE) // round down
	end := start.Add((MINUTES_PER_SLICE-1)*time.Minute + 59*time.Second)
	size := igorConfig.End - igorConfig.Start + 1
	ts := TimeSlice{Start: start.Unix(), End: end.Unix()}
	ts.Nodes = make([]uint64, size)
	Schedule = []TimeSlice{ts}

	// Now expand it to fit the minimum size we want
	extendSchedule(MIN_SCHED_LEN - MINUTES_PER_SLICE) // we already have one slice so subtract
}

// Clear out outdated time slices from the schedule and extend if needed
func expireSchedule() {
	// If the last element of the schedule is expired, or it's empty, let's start fresh
	if len(Schedule) == 0 || Schedule[len(Schedule)-1].End < time.Now().Unix() {
		if len(Schedule) == 0 {
			log.Warn("Schedule is empty, initializing new schedule.")
		} else {
			log.Info("Schedule file is expired, initializing new schedule.")
		}
		initializeSchedule()
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
		extendSchedule(MIN_SCHED_LEN - len(Schedule)*MINUTES_PER_SLICE)
	}
}

// Extend the schedule by 'minutes'.
func extendSchedule(minutes int) {
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
