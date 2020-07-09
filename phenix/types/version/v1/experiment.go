package v1

import (
	"fmt"
	"os"
	"path/filepath"
)

type Schedule map[string]string
type VLANAliases map[string]int

type VLANSpec struct {
	Aliases VLANAliases `json:"aliases" yaml:"aliases" structs:"aliases" mapstructure:"aliases"`
	Min     int         `json:"min" yaml:"min" structs:"min" mapstructure:"min"`
	Max     int         `json:"max" yaml:"max" structs:"max" mapstructure:"max"`
}

type ExperimentSpec struct {
	ExperimentName string        `json:"experimentName" yaml:"experimentName" structs:"experimentName"`
	BaseDir        string        `json:"baseDir" yaml:"baseDir" structs:"baseDir"`
	Topology       *TopologySpec `json:"topology" yaml:"topology"`
	Scenario       *ScenarioSpec `json:"scenario" yaml:"scenario"`
	VLANs          *VLANSpec     `json:"vlans" yaml:"vlans" structs:"vlans" mapstructure:"vlans"`
	Schedules      Schedule      `json:"schedules" yaml:"schedules"`
	RunLocal       bool          `json:"runLocal" yaml:"runLocal" structs:"runLocal"`
}

type ExperimentStatus struct {
	StartTime string                 `json:"startTime" yaml:"startTime" structs:"startTime" mapstructure:"startTime"`
	Schedules Schedule               `json:"schedules" yaml:"schedules"`
	Apps      map[string]interface{} `json:"apps" yaml:"apps"`
	VLANs     VLANAliases            `json:"vlans" yaml:"vlans" structs:"vlans" mapstructure:"vlans"`
}

func (this *ExperimentSpec) SetDefaults() {
	if this.BaseDir == "" {
		this.BaseDir = "/phenix/experiments/" + this.ExperimentName
	}

	if !filepath.IsAbs(this.BaseDir) {
		if absPath, err := filepath.Abs(this.BaseDir); err == nil {
			this.BaseDir = absPath
		}
	}

	if this.VLANs == nil {
		this.VLANs = new(VLANSpec)
	}

	if this.VLANs.Aliases == nil {
		this.VLANs.Aliases = make(VLANAliases)
	}

	if this.Schedules == nil {
		this.Schedules = make(Schedule)
	}

	this.Topology.SetDefaults()

	for _, n := range this.Topology.Nodes {
		for _, i := range n.Network.Interfaces {
			if _, ok := this.VLANs.Aliases[i.VLAN]; !ok {
				this.VLANs.Aliases[i.VLAN] = 0
			}
		}
	}
}

func (this VLANSpec) Validate() error {
	for k, v := range this.Aliases {
		if this.Min != 0 && v < this.Min {
			return fmt.Errorf("topology VLAN %s (VLAN ID %d) is less than experiment min VLAN ID of %d", k, v, this.Min)
		}

		if this.Max != 0 && v > this.Max {
			return fmt.Errorf("topology VLAN %s (VLAN ID %d) is greater than experiment min VLAN ID of %d", k, v, this.Max)
		}
	}

	return nil
}

func (this ExperimentSpec) VerifyScenario() error {
	if this.Scenario == nil {
		return nil
	}

	hosts := make(map[string]struct{})

	for _, node := range this.Topology.Nodes {
		hosts[node.General.Hostname] = struct{}{}
	}

	for _, app := range this.Scenario.Apps.Host {
		for _, host := range app.Hosts {
			if _, ok := hosts[host.Hostname]; !ok {
				return fmt.Errorf("host %s in app %s not in topology", host.Hostname, app.Name)
			}
		}
	}

	return nil
}

func (this ExperimentSpec) SnapshotName(node string) string {
	hostname, _ := os.Hostname()

	return fmt.Sprintf("%s_%s_%s_snapshot", hostname, this.ExperimentName, node)
}

func (this ExperimentStatus) Running() bool {
	return this.StartTime != ""
}
