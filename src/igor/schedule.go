// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	log "minilog"
	"time"
)

// scheduleHosts finds the first time the specified hosts are free for the
// requested duration.
func scheduleHosts(r *Reservations, res *Reservation) error {
	nodes := r.Nodes()

	if res.Start.IsZero() {
		res.Start = igor.Now.Round(time.Minute)
	}

Outer:
	for {
		for _, h := range res.Hosts {
			if _, ok := nodes[h]; !ok {
				return fmt.Errorf("invalid host in reservation: %v", h)
			}

			// find the next time that the node is free
			start := nextFree(nodes[h], res.Start, res.Duration)

			if start.After(res.Start) {
				// loop again to make sure that the new start time works for
				// all the other nodes
				res.Start = start
				continue Outer
			}
		}

		res.End = res.Start.Add(res.Duration)
		return nil
	}
}

// scheduleContiguous finds the first time any contiguous block of hosts are
// free for the requested duration.
func scheduleContiguous(r *Reservations, res *Reservation) error {
	nodes := r.Nodes()

	if res.Start.IsZero() {
		res.Start = igor.Now.Round(time.Minute)
	}

	// Hard mode, need to find contiguous block of nodes that are all free at
	// the same time
	validHosts := igor.validHosts()

	if len(res.Hosts) > len(validHosts) {
		return errors.New("reservation too big for cluster")
	}

	log.Debug("scheduling %v contiguous nodes across %v nodes", len(res.Hosts), len(validHosts))

	// End of time
	maxTime := time.Unix(1<<63-62135596801, 999999999)

	// block with earliest start time
	minBlock := -1

	// next free time for each of the valid hosts
	starts := make([]time.Time, len(validHosts))

	// minStart holds the time that the next reservation ends
	minStart := maxTime

	for {
		// update the valid start times for all nodes
		for i, h := range validHosts {
			// find the next time that the node is free
			starts[i] = nextFree(nodes[h], res.Start, res.Duration)
		}

	HostLoop:
		for i := range validHosts[:len(validHosts)-len(res.Hosts)+1] {
			// compute the earliest start time for this block
			var maxStart time.Time
			for j := 0; j < len(res.Hosts); j++ {
				if starts[i+j].After(maxStart) {
					maxStart = starts[i+j]
				}
			}

			// check that all the nodes are actually free from that start time
			for j := 0; j < len(res.Hosts); j++ {
				rs := nodes[validHosts[i+j]]
				if len(rs) == 0 || len(rs) == 1 {
					continue
				}

				for _, v := range rs[1:] {
					if v.IsActive(maxStart) || v.IsActive(maxStart.Add(res.Duration)) {
						log.Info("conflicts with reservation %v", v)
						continue HostLoop
					}
				}
			}

			if minBlock == -1 || maxStart.Before(starts[minBlock]) {
				minBlock = i
			}
		}

		if minBlock != -1 {
			break
		}

		// did not find a block -- try again after updating the start time
		res.Start = minStart
		minStart = maxTime
	}

	res.Start = starts[minBlock]
	res.End = res.Start.Add(res.Duration)
	for j := 0; j < len(res.Hosts); j++ {
		res.Hosts[j] = validHosts[minBlock+j]
	}
	return nil
}

// nextFree finds when a reservation of duration d can be schedule among
// existing reservations after a given time.
func nextFree(rs []*Reservation, after time.Time, d time.Duration) time.Time {
	// check if there is room before the first existing reservation
	prev := after
	for _, res := range rs {
		if res.End.Before(after) {
			continue
		}
		if res.Start.Sub(prev) >= d {
			break
		}

		prev = res.End
	}

	return prev
}
