package main

import (
	"fmt"
	log "minilog"
)

func schedule(namespace string, names []string) map[string][]string {
	res := map[string][]string{}

	ns := namespaces[namespace]

	// Simplest scheduler -- roughly equal allocation per node
	// TODO: Shuffle hosts
	hosts := ns.Hosts
	count := len(names) / len(hosts)
	if len(names)%len(names) != 0 {
		count += 1
	}

	log.Debug("launching %d vms per host", count)
	for i, name := range names {
		log.Debug("launch vm %d on host %d", i, i/count)
		host := hosts[i/count]
		if name == "" {
			name = fmt.Sprintf("vm-%v-%v", namespace, <-ns.vmIDChan)
		}

		res[host] = append(res[host], name)
	}

	return res
}
