package schedule

import (
	"fmt"

	v1 "phenix/types/version/v1"
)

var schedulers = make(map[string]Scheduler)

type Scheduler interface {
	Schedule(*v1.ExperimentSpec) error
}

func List() []string {
	var names []string

	for name := range schedulers {
		names = append(names, name)
	}

	return names
}

func Schedule(name string, spec *v1.ExperimentSpec) error {
	scheduler, ok := schedulers[name]
	if !ok {
		return fmt.Errorf("could not find scheduler with name %s", name)
	}

	return scheduler.Schedule(spec)
}
