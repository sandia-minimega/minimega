package main

import (
	"fmt"
	log "minilog"
	"sort"
	"strconv"
)

// hostSortBy defines the ordering of hosts based on some notion of load
type hostSortBy func(h1, h2 *HostStats) bool

type hostSorter struct {
	hosts []*HostStats
	by    hostSortBy
}

type ByPriority []*QueuedVMs

func (s *HostStats) IsFull() bool {
	return s.Limit != 0 && s.VMs >= s.Limit
}

func (by hostSortBy) Sort(hosts []*HostStats) {
	h := &hostSorter{
		hosts: hosts,
		by:    by,
	}

	// Floyd method -- start at the lowest levels and siftDown in each subtree.
	for i := (h.Len() - 1) / 2; i >= 0; i-- {
		h.siftDown(i)
	}
}

// Update sort order after specified host has been modified. Should only be
// used when a single host is updated.
func (by hostSortBy) Update(hosts []*HostStats, name string) {
	h := &hostSorter{
		hosts: hosts,
		by:    by,
	}

	for i, host := range hosts {
		if host.Name == name {
			h.siftDown(i)
			return
		}
	}
}

// siftDown is a slightly tweaked version of the siftDown function from the
// sort package.
func (h *hostSorter) siftDown(root int) {
	for {
		child := 2*root + 1

		if child >= h.Len() {
			return
		}

		// figure out if left or right child is bigger, proceed with the
		// smaller of the two
		if child+1 < h.Len() && !h.Less(child, child+1) {
			child++
		}

		// already meet heap requirement -- done
		if h.Less(root, child) {
			return
		}

		h.Swap(root, child)
		root = child
	}
}

func (h *hostSorter) Len() int {
	return len(h.hosts)
}

func (h *hostSorter) Swap(i, j int) {
	h.hosts[i], h.hosts[j] = h.hosts[j], h.hosts[i]
}

func (h *hostSorter) Less(i, j int) bool {
	return h.by(h.hosts[i], h.hosts[j])
}

func cpuCommit(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	return h1.CPUCommit < h2.CPUCommit
}

func memoryLoad(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	return (h1.MemTotal - h1.MemCommit) < (h2.MemTotal - h2.MemCommit)
}

func networkCommit(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	return h1.NetworkCommit < h2.NetworkCommit
}

func (q ByPriority) Less(i, j int) bool {
	return q[i].Less(q[j])
}

func (q ByPriority) Len() int {
	return len(q)
}

func (q ByPriority) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func incHostStats(stats *HostStats, config VMConfig) {
	vcpus, _ := strconv.Atoi(config.Vcpus)
	memory, _ := strconv.Atoi(config.Memory)

	stats.VMs += 1
	stats.CPUCommit += vcpus
	stats.MemCommit += memory
	stats.NetworkCommit += len(config.Networks)
}

// Less function for sorting QueuedVMs such that:
//  * host and a coschedule limit come first
//  * then those that specify a host
//  * then those that specify a coschedule limit
//  * then those that specify neither
func (q *QueuedVMs) Less(q2 *QueuedVMs) bool {
	host, host2 := q.ScheduleHost, q2.ScheduleHost

	// VMs with specified hosts should be less than those that are unspecified.
	if host != "" && host2 != "" {
		if host != host2 {
			return host < host2
		}
	} else if host != "" || host2 != "" {
		return host != ""
	}

	// VMs with specified peers should be less than those that are unspecified.
	// Within VMs that have coschedule limits, we want to process higher
	// coschedule limits first.
	limit, err := strconv.Atoi(q.SchedulePeers)
	limit2, err2 := strconv.Atoi(q2.SchedulePeers)

	if err == nil && err2 == nil {
		if limit != limit2 {
			return limit > limit2
		}
	} else if err == nil || err2 == nil {
		return err == nil
	}

	// We don't really care about the ordering of the rest but we should
	// probably schedule larger groups first.
	return len(q.Names) > len(q2.Names)
}

func schedule(queue []*QueuedVMs, hosts []*HostStats) (map[string][]*QueuedVMs, error) {
	res := map[string][]*QueuedVMs{}

	if len(hosts) == 1 {
		log.Warn("only one host in namespace, scheduling all VMs on it")
		res[hosts[0].Name] = queue
		return res, nil
	}

	// helper to find HostStats by host's name
	findHostStats := func(host string) *HostStats {
		for _, v := range hosts {
			if v.Name == host {
				return v
			}
		}

		return nil
	}

	// helper to write schedule to log
	dumpSchedule := func() {
		for host := range res {
			names := []string{}
			for _, q := range res[host] {
				names = append(names, q.Names...)
			}

			log.Info("VMs scheduled on %v: %v", host, names)
		}
	}

	// perform sanity checks to simplify scheduling loop
	for _, q := range queue {
		if host := q.ScheduleHost; host != "" {
			// host should exist
			if findHostStats(host) == nil {
				return nil, fmt.Errorf("VM scheduled on unknown host: `%v`", host)
			}
		}

		if peers := q.SchedulePeers; peers != "" {
			// coschedule should be a non-negative integer
			limit, err := strconv.Atoi(q.SchedulePeers)
			if err != nil || limit < 0 {
				return nil, fmt.Errorf("invalid coschedule value: `%v`", q.SchedulePeers)
			}

			continue
		}

		// update to "unlimited"
		q.SchedulePeers = "-1"
	}

	// perform initial sort of queued VMs and hosts
	sort.Sort(ByPriority(queue))
	hostSortBy(cpuCommit).Sort(hosts)

	for _, q := range queue {
		// no error checking required, see above
		limit, _ := strconv.Atoi(q.SchedulePeers)

		for _, name := range q.Names {
			var stats *HostStats

			if host := q.ScheduleHost; host != "" {
				// find the specified host
				stats = findHostStats(host)
			} else {
				// least loaded host is at position zero
				stats = hosts[0]
			}

			host := stats.Name

			if limit != -1 {
				// number of peers is one less than the number of VMs that we
				// should launch on the node
				limit := limit + 1
				if stats.Limit == 0 {
					// set initial limit
					stats.Limit = limit
				} else if limit < stats.Limit {
					// lower the limit for the host
					stats.Limit = limit
				}
			}

			// schedule the VMs on the host, update commit accordingly
			incHostStats(stats, q.VMConfig)

			if stats.Limit != 0 && stats.VMs > stats.Limit {
				dumpSchedule()
				return nil, fmt.Errorf("too many VMs scheduled on %v for coschedule requirement of %v", host, stats.Limit)
			}

			hostSortBy(cpuCommit).Update(hosts, host)

			q2 := *q
			q2.Names = []string{name}

			log.Debug("scheduling VMs on %v: %v", host, q2.Names)
			res[host] = append(res[host], &q2)
		}
	}

	return res, nil
}
