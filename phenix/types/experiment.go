package types

import (
	v1 "phenix/types/version/v1"
)

type Experiment struct {
	Metadata ConfigMetadata       // experiment configuration metadata
	Spec     *v1.ExperimentSpec   // reference to latest versioned experiment spec
	Status   *v1.ExperimentStatus // reference to latest versioned experiment status
}
