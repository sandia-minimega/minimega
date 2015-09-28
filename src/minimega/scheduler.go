package main

import (
	"fmt"
	log "minilog"
)

func schedule(ns *Namespace) map[string][]queuedVM {
	res := map[string][]queuedVM{}

	// Total number of VMs to launch
	var total int

	for _, queued := range ns.queuedVMs {
		total += len(queued.names)
	}

	// Simplest scheduler -- roughly equal allocation per node
	hosts := PermStrings(ns.Hosts)

	// Number of VMs per host, need to round up
	perHost := int(float32(total)/float32(len(hosts)) + 0.5)
	log.Debug("launching %d vms per host", perHost)

	// Host is an index in hosts that VMs are currently being allocated on and
	// allocated is the number of VMs that have been allocated on that host
	var host, allocated int

	for _, queued := range ns.queuedVMs {
		// Replace empty VM names with generic name
		for i, name := range queued.names {
			if name == "" {
				queued.names[i] = fmt.Sprintf("vm-%v-%v", namespace, <-ns.vmIDChan)
			}
		}

		// Process queued VMs until all names have been allocated
		for len(queued.names) > 0 {
			// Splitter for names based on how many VMs should be allocated to
			// the current host
			split := perHost - allocated
			if split > len(queued.names) {
				split = len(queued.names)
			}

			// Copy queued and partition names
			curr := queued
			curr.names = queued.names[:split]
			queued.names = queued.names[split:]

			res[hosts[host]] = append(res[hosts[host]], curr)
			allocated += len(curr.names)

			if allocated == perHost {
				host += 1
				allocated = 0
			}
		}
	}

	return res
}
