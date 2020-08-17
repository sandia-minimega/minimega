package types

import (
	"fmt"
)

type Experiments struct {
	Experiments []Experiment `json:"experiments"`
}

type Experiment struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Topology  string   `json:"topology"`
	Apps      []string `json:"apps"`
	StartTime string   `json:"start_time"`
	Running   bool     `json:"running"` // TODO: deprecate in lieu of `Status`
	Status    Status   `json:"status"`
	VMCount   int      `json:"vm_count"`
	VLANMin   int      `json:"vlan_min"`
	VLANMax   int      `json:"vlan_max"`
	VLANCount int      `json:"vlan_count"`
	VLANs     []VLAN   `json:"vlans"`
	VMs       []VM     `json:"vms"`
}

func (this Experiment) Validate() error {
	if this.VLANMin > this.VLANMax {
		return fmt.Errorf("vlan_min must be <= vlan_max")
	}

	return nil
}
