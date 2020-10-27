package types

import (
	"fmt"
	"strings"

	ifaces "phenix/types/interfaces"
	"phenix/types/version"

	"github.com/mitchellh/mapstructure"
)

type Experiment struct {
	Metadata ConfigMetadata          `json:"metadata" yaml:"metadata"` // experiment configuration metadata
	Spec     ifaces.ExperimentSpec   `json:"spec" yaml:"spec"`         // reference to latest versioned experiment spec
	Status   ifaces.ExperimentStatus `json:"status" yaml:"status"`     // reference to latest versioned experiment status
}

func NewExperiment(md ConfigMetadata) *Experiment {
	ver := version.StoredVersion["Experiment"]

	spec, _ := version.GetVersionedSpecForKind("Experiment", ver)
	status, _ := version.GetVersionedStatusForKind("Experiment", ver)

	return &Experiment{
		Metadata: md,
		Spec:     spec.(ifaces.ExperimentSpec),
		Status:   status.(ifaces.ExperimentStatus),
	}
}

func (this *Experiment) SetSpec(spec ifaces.ExperimentSpec) {
	this.Spec = spec
}

func (this Experiment) Apps() []ifaces.ScenarioApp {
	if this.Spec.Scenario() != nil {
		return this.Spec.Scenario().Apps()
	}

	return nil
}

func (this Experiment) Running() bool {
	if this.Status == nil {
		return false
	}

	if this.Status.StartTime() == "" {
		return false
	}

	return true
}

func DecodeExperimentFromConfig(c Config) (*Experiment, error) {
	iface, err := version.GetVersionedSpecForKind(c.Kind, c.APIVersion())
	if err != nil {
		return nil, fmt.Errorf("getting versioned spec for config: %w", err)
	}

	if err := mapstructure.Decode(c.Spec, &iface); err != nil {
		// Known issue when starting an existing experiment that contains an older
		// version of the scenario config.
		if strings.Contains(err.Error(), `'scenario.apps': source data must be an array or slice, got map`) {
			kbArticle := "EX-SC-UPG-01"
			kbLink := "https://phenix.sceptre.dev/kb/#article-ex-sc-upg-01"

			return nil, fmt.Errorf("decoding versioned spec for experiment %s: %w\n\nPlease see KB article %s at %s", c.Metadata.Name, err, kbArticle, kbLink)
		}

		return nil, fmt.Errorf("decoding versioned spec: %w", err)
	}

	spec, ok := iface.(ifaces.ExperimentSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec in config")
	}

	iface, err = version.GetVersionedStatusForKind(c.Kind, c.APIVersion())
	if err != nil {
		return nil, fmt.Errorf("getting versioned status for config: %w", err)
	}

	if err := mapstructure.Decode(c.Status, &iface); err != nil {
		return nil, fmt.Errorf("decoding versioned status: %w", err)
	}

	status, ok := iface.(ifaces.ExperimentStatus)
	if !ok {
		return nil, fmt.Errorf("invalid status in config")
	}

	exp := &Experiment{
		Metadata: c.Metadata,
		Spec:     spec,
		Status:   status,
	}

	return exp, nil
}
