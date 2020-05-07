package vm

type RedeployOption func(*redeployOptions)

type redeployOptions struct {
	cpu    int
	mem    int
	disk   string
	inject bool
	part   int
}

func newRedeployOptions(opts ...RedeployOption) redeployOptions {
	var o redeployOptions

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func CPU(c int) RedeployOption {
	return func(o *redeployOptions) {
		o.cpu = c
	}
}

func Memory(m int) RedeployOption {
	return func(o *redeployOptions) {
		o.mem = m
	}
}

func Disk(d string) RedeployOption {
	return func(o *redeployOptions) {
		o.disk = d
	}
}

func Inject(i bool) RedeployOption {
	return func(o *redeployOptions) {
		o.inject = i
	}
}

func InjectPartition(p int) RedeployOption {
	return func(o *redeployOptions) {
		o.part = p
	}
}
