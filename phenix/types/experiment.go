package types

import (
	v1 "phenix/types/version/v1"
)

type Experiment struct {
	Metadata ConfigMetadata       `json:"metadata" yaml:"metadata"` // experiment configuration metadata
	Spec     *v1.ExperimentSpec   `json:"spec" yaml:"spec"`         // reference to latest versioned experiment spec
	Status   *v1.ExperimentStatus `json:"status" yaml:"status"`     // reference to latest versioned experiment status
}

func (this Experiment) GetHostApps() []v1.HostApp {
	if this.Spec.Scenario != nil && this.Spec.Scenario.Apps != nil {
		return this.Spec.Scenario.Apps.Host
	}

	return nil
}
