package shell

type Option func(*options)

type options struct {
	cmd   string
	args  []string
	stdin []byte
}

func newOptions(opts ...Option) options {
	var o options

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func Command(c string) Option {
	return func(o *options) {
		o.cmd = c
	}
}

func Args(a ...string) Option {
	return func(o *options) {
		o.args = a
	}
}

func Stdin(s []byte) Option {
	return func(o *options) {
		o.stdin = s
	}
}
