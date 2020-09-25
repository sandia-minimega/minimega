package vm

type UpdateOption func(*updateOptions)

type iface struct {
	index int
	vlan  string
}

type updateOptions struct {
	exp   string
	vm    string
	cpu   int
	mem   int
	disk  string
	dnb   *bool
	iface *iface
	host  *string
}

func newUpdateOptions(opts ...UpdateOption) updateOptions {
	var o updateOptions

	for _, opt := range opts {
		opt(&o)
	}

	return o
}

func UpdateExperiment(e string) UpdateOption {
	return func(o *updateOptions) {
		o.exp = e
	}
}

func UpdateVM(v string) UpdateOption {
	return func(o *updateOptions) {
		o.vm = v
	}
}

func UpdateWithCPU(c int) UpdateOption {
	return func(o *updateOptions) {
		o.cpu = c
	}
}

func UpdateWithMem(m int) UpdateOption {
	return func(o *updateOptions) {
		o.mem = m
	}
}

func UpdateWithDisk(d string) UpdateOption {
	return func(o *updateOptions) {
		o.disk = d
	}
}

func UpdateWithInterface(i int, v string) UpdateOption {
	return func(o *updateOptions) {
		o.iface = &iface{index: i, vlan: v}
	}
}

func UpdateWithDNB(b bool) UpdateOption {
	return func(o *updateOptions) {
		o.dnb = &b
	}
}

func UpdateWithHost(h string) UpdateOption {
	return func(o *updateOptions) {
		o.host = &h
	}
}

// RedeployOption is a function that configures options for a VM redeployment.
// It is used in `vm.Redeploy`.
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

// CPU sets the number of CPUs to be used for the redeployed VM. It defaults to
// 0, which means the redeployed VM will have the same number of CPUs as the
// current VM.
func CPU(c int) RedeployOption {
	return func(o *redeployOptions) {
		o.cpu = c
	}
}

// Memory sets the amound of memory (in MB) to be used for the redeployed VM. It
// defaults to 0, which means the redeployed VM will have the same amount of
// memory as the current VM.
func Memory(m int) RedeployOption {
	return func(o *redeployOptions) {
		o.mem = m
	}
}

// Disk sets the path to the disk to be used for the redeployed VM. It defaults
// to an empyt string, which means the redeployed VM will use the same disk as
// the current VM.
func Disk(d string) RedeployOption {
	return func(o *redeployOptions) {
		o.disk = d
	}
}

// Inject sets whether or not files will be injected into the redeployed VM. If
// true, and `Disk` is set, and the current VM has file injections configured,
// the redeployed VM will have the same files injected.
func Inject(i bool) RedeployOption {
	return func(o *redeployOptions) {
		o.inject = i
	}
}

// InjectPartition sets the disk partition files will be injected into in the
// redeployed VM. Only used if `Inject` is set to true and `Disk` is set.
func InjectPartition(p int) RedeployOption {
	return func(o *redeployOptions) {
		o.part = p
	}
}
