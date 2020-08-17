package types

import (
	v1 "phenix/types/version/v1"
)

type Experiment struct {
	Metadata ConfigMetadata       `json:"metadata" yaml:"metadata"` // experiment configuration metadata
	Spec     *v1.ExperimentSpec   `json:"spec" yaml:"spec"`         // reference to latest versioned experiment spec
	Status   *v1.ExperimentStatus `json:"status" yaml:"status"`     // reference to latest versioned experiment status
}

type Status string

const (
	StatusStopping     = "stopping"
	StatusStopped      = "stopped"
	StatusStarting     = "starting"
	StatusStarted      = "started"
	StatusCreating     = "creating"
	StatusDeleting     = "deleting"
	StatusRedeploying  = "redeploying"
	StatusSnapshotting = "snapshotting"
	StatusRestoring    = "restoring"
	StatusCommitting   = "committing"
)

func (this Experiment) ToUI(status Status, vms []VM) map[string]interface{} {
	data := map[string]interface{}{
		"name":       this.Spec.ExperimentName,
		"topology":   this.Metadata.Annotations["topology"],
		"start_time": this.Status.StartTime,
		"running":    this.Status.Running(),
		"status":     status,
		"vlan_max":   this.Spec.VLANs.Max,
		"vlan_min":   this.Spec.VLANs.Min,
		"vlan_count": len(this.Spec.VLANs.Aliases),
		"vms":        vms,
		"vm_count":   len(vms),
	}

	var apps []string

	for _, app := range this.Spec.Scenario.Apps.Experiment {
		apps = append(apps, app.Name)
	}

	for _, app := range this.Spec.Scenario.Apps.Host {
		apps = append(apps, app.Name)
	}

	data["apps"] = apps

	var vlans []map[string]interface{}

	for alias := range this.Spec.VLANs.Aliases {
		vlan := map[string]interface{}{
			"vlan":  this.Spec.VLANs.Aliases[alias],
			"alias": alias,
		}

		vlans = append(vlans, vlan)
	}

	data["vlans"] = vlans

	return data
}

func (this *Experiment) FromUI(data map[string]interface{}) {
	meta := ConfigMetadata{
		Name: data["name"].(string),
		Annotations: map[string]string{
			"topology": data["topology"].(string),
			"scenario": data["scenario"].(string),
		},
	}

	spec := &v1.ExperimentSpec{
		ExperimentName: data["name"].(string),
		VLANs: &v1.VLANSpec{
			Min: int(data["vlan_min"].(float64)),
			Max: int(data["vlan_max"].(float64)),
		},
	}

	this.Metadata = meta
	this.Spec = spec
}
