package mm

var DefaultMM MM = new(Minimega)

type MM interface {
	ReadScriptFromFile(string) error
	ClearNamespace(string) error

	LaunchVMs(string) error
	GetLaunchProgress(string, int) (float64, error)

	GetVMInfo(...Option) VMs
	GetVMScreenshot(...Option) ([]byte, error)
	GetVNCEndpoint(...Option) (string, error)
	StartVM(...Option) error
	StopVM(...Option) error
	RedeployVM(...Option) error
	KillVM(...Option) error
	GetVMHost(...Option) (string, error)

	ConnectVMInterface(...Option) error
	DisconnectVMInterface(...Option) error

	StartVMCapture(...Option) error
	StopVMCapture(...Option) error
	GetExperimentCaptures(...Option) []Capture
	GetVMCaptures(...Option) []Capture

	GetClusterHosts(bool) (Hosts, error)
	IsHeadnode(string) bool
	GetVLANs(...Option) (map[string]int, error)
}
