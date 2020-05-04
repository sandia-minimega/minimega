package app

// Option is a function that configures options for a phenix app. It is used in
// `app.Init`.
type Option func(*Options)

// Options represents a set of options generic to all apps.
type Options struct {
	Name string // used to set the app name
}

// NewOptions returns an Options struct initialized with the given option list.
func NewOptions(opts ...Option) Options {
	o := Options{}

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

// Name sets the name for the app.
func Name(n string) Option {
	return func(o *Options) {
		o.Name = n
	}
}
