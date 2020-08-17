package mm

type Option func(*options)

type options struct {
	ns   string
	vm   string
	cpu  int
	mem  int
	disk string

	injectPart int
	injects    []string

	connectIface int
	connectVLAN  string

	captureIface int
	captureFile  string

	screenshotSize string
}

func NewOptions(opts ...Option) options {
	var o options

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func NS(n string) Option {
	return func(o *options) {
		o.ns = n
	}
}

func VM(v string) Option {
	return func(o *options) {
		o.vm = v
	}
}

func CPU(c int) Option {
	return func(o *options) {
		o.cpu = c
	}
}

func Mem(m int) Option {
	return func(o *options) {
		o.mem = m
	}
}

func Disk(d string) Option {
	return func(o *options) {
		o.disk = d
	}
}

func InjectPartition(p int) Option {
	return func(o *options) {
		o.injectPart = p
	}
}

func Injects(i ...string) Option {
	return func(o *options) {
		o.injects = i
	}
}

func ConnectInterface(i int) Option {
	return func(o *options) {
		o.connectIface = i
	}
}

func ConnectVLAN(v string) Option {
	return func(o *options) {
		o.connectVLAN = v
	}
}

func DisonnectInterface(i int) Option {
	return func(o *options) {
		o.connectIface = i
	}
}

func CaptureInterface(i int) Option {
	return func(o *options) {
		o.captureIface = i
	}
}

func CaptureFile(f string) Option {
	return func(o *options) {
		o.captureFile = f
	}
}

func ScreenshotSize(s string) Option {
	return func(o *options) {
		o.screenshotSize = s
	}
}
