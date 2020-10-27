package scheduler

import (
	"fmt"

	"phenix/internal/mm"
	ifaces "phenix/types/interfaces"
)

func init() {
	schedulers["round-robin"] = new(roundRobin)
}

type roundRobin struct{}

func (roundRobin) Init(...Option) error {
	return nil
}

func (roundRobin) Name() string {
	return "round-robin"
}

func (roundRobin) Schedule(spec ifaces.ExperimentSpec) error {
	if len(spec.Topology().Nodes()) == 0 {
		return fmt.Errorf("no VMs defined for experiment")
	}

	cluster, err := mm.GetClusterHosts(true)
	if err != nil {
		return fmt.Errorf("getting cluster hosts: %w", err)
	}

	// Update cluster host VM count to account for VMs manually scheduled before
	// sorting hosts by VM count below.
	for _, name := range spec.Schedules() {
		if host := cluster.FindHostByName(name); host != nil {
			cluster.IncrHostVMs(name, 1)
		}
	}

	cluster.SortByVMs(true)

	// TODO: sort VMs by scheduled,memory (??)

	for _, node := range spec.Topology().Nodes() {
		if _, ok := spec.Schedules()[node.General().Hostname()]; !ok {
			spec.Schedules()[node.General().Hostname()] = cluster[0].Name

			cluster[0].VMs += 1
			cluster.SortByVMs(true)
		}
	}

	return nil
}
