package vlan

type Option func(*options)

type options struct {
	exp string

	alias string
	id    int

	min int
	max int

	force bool
}

func newOptions(opts ...Option) options {
	var o options

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func Experiment(e string) Option {
	return func(o *options) {
		o.exp = e
	}
}

func Alias(a string) Option {
	return func(o *options) {
		o.alias = a
	}
}

func ID(i int) Option {
	return func(o *options) {
		o.id = i
	}
}

func Min(m int) Option {
	return func(o *options) {
		o.min = m
	}
}

func Max(m int) Option {
	return func(o *options) {
		o.max = m
	}
}

func Force(f bool) Option {
	return func(o *options) {
		o.force = f
	}
}
