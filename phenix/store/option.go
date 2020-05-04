package store

// Option is a function that configures options for a config store. It is used
// in `store.Init`.
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

// Endpoint sets the endpoint URI to use for the store.
func Endpoint(e string) Option {
	return func(o *Options) {
		o.Endpoint = e
	}
}
