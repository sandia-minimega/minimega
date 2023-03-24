package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type Scheduler struct {
	queue      []*QueuedVMs
	hosts      []*HostStats
	hostSortBy // embed

	// output from scheduler map of queued vms, indexed by host that they
	// should be scheduled on
	res map[string][]*QueuedVMs

	// colocated are VMs that need to be scheduled with another VM, indexed by
	// the name of the VM to be colocated with
	colocated map[string][]*QueuedVMs
}

// hostSortBy defines the ordering of hosts based on some notion of load
type hostSortBy func(h1, h2 *HostStats) bool

// hostSorter acts a minheap so that the least-loaded host is in position 0.
type hostSorter struct {
	hosts []*HostStats
	by    hostSortBy
}

var hostSortByFns = map[string]hostSortBy{
	"netcommit": networkCommit,
	"cpucommit": cpuCommit,
	"memcommit": memoryCommit,
}

func (s *Scheduler) Schedule() (map[string][]*QueuedVMs, error) {
	if len(s.hosts) == 0 {
		return nil, errors.New("no hosts to schedule VMs on")
	}

	if len(s.hosts) == 1 {
		log.Warn("only one host in namespace, scheduling all VMs on it")
		res := map[string][]*QueuedVMs{
			s.hosts[0].Name: s.queue,
		}
		return res, nil
	}

	s.res = map[string][]*QueuedVMs{}
	s.colocated = map[string][]*QueuedVMs{}

	for _, q := range s.queue {
		// resolve `localhost` to actual hostname
		if q.Schedule == Localhost {
			q.Schedule = hostname
		}

		// ensure we can get host stats to simplify scheduling loop
		if host := q.Schedule; host != "" {
			// host should exist
			if s.findHostStats(host) == nil {
				return nil, fmt.Errorf("VM scheduled on unknown host: `%v`", host)
			}
		}

		// found "floating" VM
		if q.Colocate != "" && q.Schedule == "" {
			s.colocated[q.Colocate] = append(s.colocated[q.Colocate], q)
		}
	}

	// update colocatedCounts so that we can include it in the sorting
	for _, q := range s.queue {
		q.colocatedCounts = map[string]int{}
		q.colocatedCount = 0

		for _, name := range q.Names {
			if _, ok := s.colocated[name]; !ok {
				continue
			}

			v := len(s.colocated[name])
			q.colocatedCounts[name] = v
			q.colocatedCount += v
		}
	}

	// perform initial sort of queued VMs and hosts
	sort.Slice(s.queue, func(i, j int) bool {
		return s.queue[i].Less(s.queue[j])
	})
	s.hostSortBy.Sort(s.hosts)

	for _, q := range s.queue {
		if q.Colocate != "" && q.Schedule == "" {
			// floating, ignore
			continue
		}

		for _, name := range q.Names {
			// least loaded host is at position zero
			host := s.hosts[0]

			if v := q.Schedule; v != "" {
				// find the specified host
				host = s.findHostStats(v)
			}

			if err := s.add(host, name, q); err != nil {
				s.dumpSchedule()
				return nil, err
			}

			s.hostSortBy.Update(s.hosts, host.Name)
		}
	}

	// floating VMs never found a home... should probably be an error, right?
	if len(s.colocated) != 0 {
		names := []string{}
		for k := range s.colocated {
			names = append(names, k)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("nonexistent colocate VMs: [%v]", strings.Join(names, ", "))
	}

	return s.res, nil
}

// helper to write schedule to log
func (s *Scheduler) dumpSchedule() {
	log.Info("partial schedule:")
	for host := range s.res {
		names := []string{}
		for _, q := range s.res[host] {
			names = append(names, q.Names...)
		}

		log.Info("VMs scheduled on %v: %v", host, names)
	}
}

// helper to find host stats by name
func (s *Scheduler) findHostStats(host string) *HostStats {
	for _, v := range s.hosts {
		if v.Name == host {
			return v
		}
	}

	return nil
}

// add a VM to the given host, checking and adjusting limits if necessary
func (s *Scheduler) add(host *HostStats, name string, q *QueuedVMs) error {
	limit := int(q.Coschedule)

	if limit != -1 {
		// number of peers is one less than the number of VMs that we
		// should launch on the node
		limit := limit + 1
		if host.Limit == -1 {
			// set initial limit
			log.Debug("set initial limit on %v to %v", host.Name, limit)
			host.Limit = limit
		} else if limit < host.Limit {
			// lower the limit for the host
			log.Debug("lower limit on %v from %v to %v", host.Name, host.Limit, limit)
			host.Limit = limit
		}
	}

	// update commit based on this VM's specs
	host.increment(q.VMConfig)

	// schedule all floating VMs on this host as well
	for _, q := range s.colocated[name] {
		for _, name2 := range q.Names {
			log.Debug("colocating %v with %v", name2, name)
			if err := s.add(host, name2, q); err != nil {
				return err
			}
		}
	}

	// we no longer need to track these floating VMs
	delete(s.colocated, name)

	if host.Limit != -1 && host.VMs > host.Limit {
		return fmt.Errorf("too many VMs scheduled on %v for coschedule requirement of %v", host.Name, host.Limit)
	}

	// create copy of q
	q2 := *q
	q2.Names = []string{name}

	//log.Debug("scheduling VM on %v: %v", host.Name, name)
	s.res[host.Name] = append(s.res[host.Name], &q2)

	return nil
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

// Less function for sorting QueuedVMs such that:
//   - host and a coschedule limit come first
//   - then those that specify a host
//   - then those that specify a coschedule limit
//   - then those that have more colocated VMs
//   - then those that specify neither
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

	// VMs with more colocated VMs are next
	if q.colocatedCount != q2.colocatedCount {
		return q.colocatedCount > q2.colocatedCount
	}

	// We don't really care about the ordering of the rest but we should
	// probably schedule larger groups first.
	return len(q.Names) > len(q2.Names)
}

func (s *HostStats) increment(config VMConfig) {
	s.VMs += 1
	s.CPUCommit += config.VCPUs
	s.MemCommit += config.Memory
	s.NetworkCommit += len(config.Networks)
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

func schedule(q []*QueuedVMs, hosts []*HostStats, by hostSortBy) (map[string][]*QueuedVMs, error) {
	s := &Scheduler{
		queue:      q,
		hosts:      hosts,
		hostSortBy: by,
	}

	return s.Schedule()
}
