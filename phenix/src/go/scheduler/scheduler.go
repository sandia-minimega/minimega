package scheduler

import (
	v1 "phenix/types/version/v1"
	"phenix/util/shell"
)

var schedulers = make(map[string]Scheduler)

// Scheduler is the interface that identifies all the required functionality for
// a phenix scheduler.
type Scheduler interface {
	// Init is used to initialize a phenix scheduler with options generic to all
	// schedulers.
	Init(...Option) error

	// Name returns the name of the phenix scheduler.
	Name() string

	// Schedule runs the phenix scheduler algorithm against the given experiment.
	Schedule(*v1.ExperimentSpec) error
}

func List() []string {
	var names []string

	for name := range schedulers {
		names = append(names, name)
	}

	for _, name := range shell.FindCommandsWithPrefix("phenix-scheduler-") {
		names = append(names, name)
	}

	return names
}

func Schedule(name string, spec *v1.ExperimentSpec) error {
	scheduler, ok := schedulers[name]
	if !ok {
		scheduler = new(userScheduler)
		scheduler.Init(Name(name))
	}

	return scheduler.Schedule(spec)
}
