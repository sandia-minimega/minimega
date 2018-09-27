package main

import (
	"errors"
	"fmt"
	log "minilog"
	"sort"
)

// hostSortBy defines the ordering of hosts based on some notion of load
type hostSortBy func(h1, h2 *HostStats) bool

type hostSorter struct {
	hosts []*HostStats
	by    hostSortBy
}

type ByPriority []*QueuedVMs

var hostSortByFns = map[string]hostSortBy{
	"netcommit": networkCommit,
	"cpucommit": cpuCommit,
	"memcommit": memoryCommit,
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
func (q ByPriority) Less(i, j int) bool {
	return q[i].Less(q[j])
}

func (q ByPriority) Len() int {
	return len(q)
}

func (q ByPriority) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

// Less function for sorting QueuedVMs such that:
//  * host and a coschedule limit come first
//  * then those that specify a host
//  * then those that specify a coschedule limit
//  * then those that specify neither
func (q *QueuedVMs) Less(q2 *QueuedVMs) bool {
	host, host2 := q.Schedule, q2.Schedule

	// VMs with specified hosts should be less than those that are unspecified.
	if host != host2 {
		// check if only one specified
		if host != "" || host2 != "" {
			return host != ""
		}

		return host < host2
	}

	// VMs with specified peers should be less than those that are unspecified.
	// Within VMs that have coschedule limits, we want to process the lower
	// coschedule limits first.
	if q.Coschedule != q2.Coschedule {
		// check if only one specified
		if q.Coschedule == -1 || q2.Coschedule == -1 {
			return q.Coschedule != -1
		}

		return q.Coschedule < q2.Coschedule
	}

	// We don't really care about the ordering of the rest but we should
	// probably schedule larger groups first.
	return len(q.Names) > len(q2.Names)
}

// cpuCommit tests whether h1 < h2.
func cpuCommit(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	r1 := float64(h1.CPUCommit) / float64(h1.CPUs)
	r2 := float64(h2.CPUCommit) / float64(h2.CPUs)

	return r1 < r2
}

// memoryCommit tests whether h1 < h2.
func memoryCommit(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	r1 := float64(h1.MemCommit) / float64(h1.MemTotal)
	r2 := float64(h2.MemCommit) / float64(h2.MemTotal)

	return r1 < r2
}

// networkCommit tests whether h1 < h2.
func networkCommit(h1, h2 *HostStats) bool {
	// fully loaded host is always greater
	if full := h1.IsFull(); full != h2.IsFull() {
		return !full
	}

	return h1.NetworkCommit < h2.NetworkCommit
}

func incHostStats(stats *HostStats, config VMConfig) {
	stats.VMs += 1
	stats.CPUCommit += config.VCPUs
	stats.MemCommit += config.Memory
	stats.NetworkCommit += len(config.Networks)
}

func schedule(queue []*QueuedVMs, hosts []*HostStats, hostSorter hostSortBy) (map[string][]*QueuedVMs, error) {
	res := map[string][]*QueuedVMs{}

	if len(hosts) == 0 {
		return nil, errors.New("no hosts to schedule VMs on")
	}

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
		log.Info("partial schedule:")
		for host := range res {
			names := []string{}
			for _, q := range res[host] {
				names = append(names, q.Names...)
			}

			log.Info("VMs scheduled on %v: %v", host, names)
		}
	}

	for _, q := range queue {
		// resolve `localhost` to actual hostname
		if q.Schedule == Localhost {
			q.Schedule = hostname
		}

		// ensure we can get host stats to simplify scheduling loop
		if host := q.Schedule; host != "" {
			// host should exist
			if findHostStats(host) == nil {
				return nil, fmt.Errorf("VM scheduled on unknown host: `%v`", host)
			}
		}
	}

	// perform initial sort of queued VMs and hosts
	sort.Sort(ByPriority(queue))
	hostSorter.Sort(hosts)

	for _, q := range queue {
		// no error checking required, see above
		limit := int(q.Coschedule)

		for _, name := range q.Names {
			var stats *HostStats

			if host := q.Schedule; host != "" {
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
				if stats.Limit == -1 {
					// set initial limit
					log.Debug("set initial limit on %v to %v", stats.Name, limit)
					stats.Limit = limit
				} else if limit < stats.Limit {
					// lower the limit for the host
					log.Debug("lower limit on %v from %v to %v", stats.Name, stats.Limit, limit)
					stats.Limit = limit
				}
			}

			// schedule the VMs on the host, update commit accordingly
			incHostStats(stats, q.VMConfig)

			if stats.Limit != -1 && stats.VMs > stats.Limit {
				dumpSchedule()
				return nil, fmt.Errorf("too many VMs scheduled on %v for coschedule requirement of %v", host, stats.Limit)
			}

			hostSorter.Update(hosts, host)

			q2 := *q
			q2.Names = []string{name}

			//log.Debug("scheduling VM on %v: %v", host, name)
			res[host] = append(res[host], &q2)
		}
	}

	return res, nil
}
