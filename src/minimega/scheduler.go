package main

import log "minilog"

func schedule(queue []QueuedVMs, hosts []string) (*scheduleStat, map[string][]QueuedVMs) {
	res := map[string][]QueuedVMs{}

	// Total number of VMs to launch
	var total int

	for _, q := range queue {
		total += len(q.Names)
	}

	// Simplest scheduler -- roughly equal allocation per node
	hosts = PermStrings(hosts)

	// Number of VMs per host, need to round up
	perHost := total / len(hosts)
	if perHost*len(hosts) < total {
		perHost += 1
	}
	log.Debug("launching %d vms per host", perHost)

	// Host is an index in hosts that VMs are currently being allocated on and
	// allocated is the number of VMs that have been allocated on that host
	var host, allocated int

	for _, q := range queue {
		// Process queued VMs until all names have been allocated
		for len(q.Names) > 0 {
			// Splitter for names based on how many VMs should be allocated to
			// the current host
			split := perHost - allocated
			if split > len(q.Names) {
				split = len(q.Names)
			}

			// Copy queued and partition names
			curr := q
			curr.Names = q.Names[:split]
			q.Names = q.Names[split:]

			res[hosts[host]] = append(res[hosts[host]], curr)
			allocated += len(curr.Names)

			if allocated == perHost {
				host += 1
				allocated = 0
			}
		}
	}

	stats := &scheduleStat{
		total: total,
		hosts: len(hosts),
	}

	return stats, res
}
