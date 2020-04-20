package app

type Option func(*Options)

type Options struct {
	Name string
}

func NewOptions(opts ...Option) Options {
	o := Options{}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func Name(n string) Option {
	return func(o *Options) {
		o.Name = n
	}
}
