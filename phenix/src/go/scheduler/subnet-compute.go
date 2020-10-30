package scheduler

import (
	"fmt"

	"phenix/internal/mm"
	v1 "phenix/types/version/v1"
)

func init() {
	schedulers["subnet-compute"] = new(subnetCompute)
}

type subnetCompute struct{}

func (subnetCompute) Init(...Option) error {
	return nil
}

func (subnetCompute) Name() string {
	return "subnet-compute"
}

func (subnetCompute) Schedule(spec *v1.ExperimentSpec) error {
	if len(spec.Topology.Nodes) == 0 {
		return fmt.Errorf("no VMs defined for experiment")
	}

	cluster, err := mm.GetClusterHosts(true)
	if err != nil {
		return fmt.Errorf("getting cluster hosts: %w", err)
	}

	// map VLAN aliases (key) to one (or more) cluster hosts
	vlans := make(map[string][]string)

	// Update VLAN alias map and cluster host VM count to account for VMs manually
	// scheduled before sorting hosts by VM count below.
	for node, host := range spec.Schedules {
		if h := cluster.FindHostByName(host); h != nil {
			if n := spec.Topology.FindNodeByName(node); n != nil {
				if len(n.Network.Interfaces) == 0 {
					return fmt.Errorf("node %s doesn't have any network interfaces", n.General.Hostname)
				}

				// cluster.IncrHostVMs(host, 1)
				// cluster.IncrHostCPUCommit(host, n.Hardware.VCPU)
				cluster.IncrHostMemCommit(host, n.Hardware.Memory)

				vlan := n.Network.Interfaces[0].VLAN

				hosts := vlans[vlan]
				hosts = append(hosts, h.Name)
				vlans[vlan] = hosts
			}
		}
	}

	cluster.SortByCommittedMem(true)

	for _, node := range spec.Topology.Nodes {
		if _, ok := spec.Schedules[node.General.Hostname]; ok {
			continue
		}

		var scheduled *mm.Host

		if len(node.Network.Interfaces) == 0 {
			return fmt.Errorf("node %s doesn't have any network interfaces", node.General.Hostname)
		}

		vlan := node.Network.Interfaces[0].VLAN

		if hosts, ok := vlans[vlan]; ok {
			for _, name := range hosts {
				if host := cluster.FindHostByName(name); host != nil {
					if (host.MemCommit + node.Hardware.Memory) < host.MemTotal {
						scheduled = host
						break
					}
				}
			}
		}

		if scheduled == nil {
			// VM's VLAN alias not mapped to a cluster host yet, so use round-robin
			// approach to find a cluster host to map it to.
			scheduled = &cluster[0]

			hosts := vlans[vlan]
			hosts = append(hosts, scheduled.Name)
			vlans[vlan] = hosts
		}

		spec.Schedules[node.General.Hostname] = scheduled.Name

		// cluster.IncrHostVMs(scheduled.Name, 1)
		// cluster.IncrHostCPUCommit(scheduled.Name, node.Hardware.VCPU)
		cluster.IncrHostMemCommit(scheduled.Name, node.Hardware.Memory)

		cluster.SortByCommittedMem(true)
	}

	return nil
}
