package store

type Option func(*Options)

type Options struct {
	Endpoint string
}

func NewOptions(opts ...Option) Options {
	var o Options

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func Endpoint(e string) Option {
	return func(o *Options) {
		o.Endpoint = e
	}
}
