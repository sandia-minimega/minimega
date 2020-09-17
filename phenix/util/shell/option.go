package shell

import "os"

type Option func(*options)

type options struct {
	cmd   string
	env   []string
	args  []string
	stdin []byte
}

func newOptions(opts ...Option) options {
	o := options{
		env: os.Environ(),
	}

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

func Env(e ...string) Option {
	return func(o *options) {
		o.env = append(o.env, e...)
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
