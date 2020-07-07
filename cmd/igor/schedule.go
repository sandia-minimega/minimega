// Copyright (2013) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"errors"
	"fmt"
	"time"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// scheduleHosts finds the first time the specified hosts are free for the
// requested duration.
func scheduleHosts(r *Reservations, res *Reservation) error {
	nodes := r.Nodes()

	if res.Start.IsZero() {
		res.Start = igor.Now.Round(time.Minute)
	}

	log.Info("scheduling %v requested nodes: %v", len(res.Hosts), res.Hosts)

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

	log.Info("scheduling %v contiguous nodes", len(res.Hosts))

	// End of time
	maxTime := time.Unix(1<<63-62135596801, 999999999)

	// next free time for each of the valid hosts
	starts := make([]time.Time, len(validHosts))

	for {
		log.Debug("res start time is %v", res.Start)

		// block with earliest start time and it's start time on this pass
		minBlock := -1
		minStart := maxTime

		// update the valid start times for all nodes
		for i, h := range validHosts {
			// find the next time that the node is free
			starts[i] = nextFree(nodes[h], res.Start, res.Duration)
		}

	HostLoop:
		for i := range validHosts[:len(validHosts)-len(res.Hosts)+1] {
			// compute the latest start time for this block
			var blockStart time.Time
			for j := 0; j < len(res.Hosts); j++ {
				if starts[i+j].After(blockStart) {
					blockStart = starts[i+j]
				}
			}

			// when reservation would end for this block
			end := blockStart.Add(res.Duration)

			// ensure that the nodes are actually free for that range of time
			for j := 0; j < len(res.Hosts); j++ {
				rs := nodes[validHosts[i+j]]
				if len(rs) == 0 {
					continue
				}

				for _, v := range rs {
					if v.IsOverlap(blockStart, end) {
						log.Info("conflicts with reservation %v", v)
						continue HostLoop
					}
				}
			}

			if minBlock == -1 || blockStart.Before(minStart) {
				minBlock = i
				minStart = blockStart
			}
		}

		if minBlock == -1 {
			// we didn't find any available blocks so bump reservation start
			// time to the earliest node end time.
			var maxStart time.Time
			for _, start := range starts {
				if start.After(maxStart) {
					maxStart = start
				}
			}
			res.Start = maxStart
			continue
		}

		// found a block
		res.Start = minStart
		res.End = res.Start.Add(res.Duration)
		res.SetHosts(validHosts[minBlock : minBlock+len(res.Hosts)])
		return nil
	}
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
