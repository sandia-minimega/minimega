package app

import (
	v1 "phenix/types/version/v1"
)

type Serial struct{}

func (Serial) Init(...Option) error {
	return nil
}

func (Serial) Name() string {
	return "serial"
}

func (Serial) Configure(spec *v1.ExperimentSpec) error {
	return nil
}

func (Serial) Start(spec *v1.ExperimentSpec) error {
	return nil
}

func (Serial) PostStart(spec *v1.ExperimentSpec) error {
	return nil
}

func (Serial) Cleanup(spec *v1.ExperimentSpec) error {
	return nil
}
