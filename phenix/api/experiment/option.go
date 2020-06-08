package experiment

import v1 "phenix/types/version/v1"

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

func Name(n string) SaveOption {
	return func(o *saveOptions) {
		o.name = n
	}
}

func Spec(s *v1.ExperimentSpec) SaveOption {
	return func(o *saveOptions) {
		o.spec = s
	}
}

func Status(s *v1.ExperimentStatus) SaveOption {
	return func(o *saveOptions) {
		o.status = s
	}
}

func SaveNilSpec(s bool) SaveOption {
	return func(o *saveOptions) {
		o.saveNilSpec = s
	}
}

func SaveNilStatus(s bool) SaveOption {
	return func(o *saveOptions) {
		o.saveNilStatus = s
	}
}
