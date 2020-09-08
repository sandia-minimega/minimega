package experiment

import (
	"phenix/internal/common"
	v1 "phenix/types/version/v1"
)

type CreateOption func(*createOptions)

type createOptions struct {
	name     string
	topology string
	scenario string
	vlanMin  int
	vlanMax  int
	baseDir  string
}

func newCreateOptions(opts ...CreateOption) createOptions {
	var o createOptions

	for _, opt := range opts {
		opt(&o)
	}

	if o.baseDir == "" {
		o.baseDir = common.PhenixBase + "/experiments/" + o.name
	}

	return o
}

func CreateWithName(n string) CreateOption {
	return func(o *createOptions) {
		o.name = n
	}
}

func CreateWithTopology(t string) CreateOption {
	return func(o *createOptions) {
		o.topology = t
	}
}

func CreateWithScenario(s string) CreateOption {
	return func(o *createOptions) {
		o.scenario = s
	}
}

func CreateWithVLANMin(m int) CreateOption {
	return func(o *createOptions) {
		o.vlanMin = m
	}
}

func CreateWithVLANMax(m int) CreateOption {
	return func(o *createOptions) {
		o.vlanMax = m
	}
}

func CreateWithBaseDirectory(b string) CreateOption {
	return func(o *createOptions) {
		o.baseDir = b
	}
}

type SaveOption func(*saveOptions)

type saveOptions struct {
	name string

	spec   *v1.ExperimentSpec
	status *v1.ExperimentStatus

	saveNilSpec   bool
	saveNilStatus bool
}

func newSaveOptions(opts ...SaveOption) saveOptions {
	var o saveOptions

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func SaveWithName(n string) SaveOption {
	return func(o *saveOptions) {
		o.name = n
	}
}

func SaveWithSpec(s *v1.ExperimentSpec) SaveOption {
	return func(o *saveOptions) {
		o.spec = s
	}
}

func SaveWithStatus(s *v1.ExperimentStatus) SaveOption {
	return func(o *saveOptions) {
		o.status = s
	}
}

func SaveWithNilSpec(s bool) SaveOption {
	return func(o *saveOptions) {
		o.saveNilSpec = s
	}
}

func SaveWithNilStatus(s bool) SaveOption {
	return func(o *saveOptions) {
		o.saveNilStatus = s
	}
}

type ScheduleOption func(*scheduleOptions)

type scheduleOptions struct {
	name      string
	algorithm string
}

func newScheduleOptions(opts ...ScheduleOption) scheduleOptions {
	var o scheduleOptions

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func ScheduleForName(n string) ScheduleOption {
	return func(o *scheduleOptions) {
		o.name = n
	}
}

func ScheduleWithAlgorithm(a string) ScheduleOption {
	return func(o *scheduleOptions) {
		o.algorithm = a
	}
}

type StartOption func(*startOptions)

type startOptions struct {
	name    string
	dryrun  bool
	vlanMin int
	vlanMax int
}

func newStartOptions(opts ...StartOption) startOptions {
	var o startOptions

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func StartWithName(n string) StartOption {
	return func(o *startOptions) {
		o.name = n
	}
}

func StartWithDryRun(d bool) StartOption {
	return func(o *startOptions) {
		o.dryrun = d
	}
}

func StartWithVLANMin(m int) StartOption {
	return func(o *startOptions) {
		o.vlanMin = m
	}
}

func StartWithVLANMax(m int) StartOption {
	return func(o *startOptions) {
		o.vlanMax = m
	}
}
