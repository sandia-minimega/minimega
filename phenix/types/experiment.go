package types

import (
	v1 "phenix/types/version/v1"
)

type Experiment struct {
	Metadata ConfigMetadata
	Spec     *v1.ExperimentSpec
	Status   *v1.ExperimentStatus
}
