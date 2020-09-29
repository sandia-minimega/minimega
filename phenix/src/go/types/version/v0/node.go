package v0

type VMType string

const (
	VMType_NotSet    VMType = ""
	VMType_KVM       VMType = "kvm"
	VMType_Container VMType = "container"
)

type CPU string

const (
	CPU_NotSet    CPU = ""
	CPU_Broadwell CPU = "Broadwell"
	CPU_Haswell   CPU = "Haswell"
	CPU_Core2Duo  CPU = "core2duo"
	CPU_Pentium3  CPU = "pentium3"
)

type OSType string

const (
	OSType_NotSet  OSType = ""
	OSType_Windows OSType = "windows"
	OSType_Linux   OSType = "linux"
	OSType_RHEL    OSType = "rhel"
	OSType_CentOS  OSType = "centos"
)

type Labels map[string]string

type Node struct {
	Labels     Labels       `json:"labels" yaml:"labels"`
	Type       string       `json:"type" yaml:"type"`
	General    General      `json:"general" yaml:"general"`
	Hardware   Hardware     `json:"hardware" yaml:"hardware"`
	Network    Network      `json:"network" yaml:"network"`
	Injections []*Injection `json:"injections" yaml:"injections"`
}

type General struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	Description string `json:"description" yaml:"description"`
	VMType      VMType `json:"vm_type" yaml:"vm_type" structs:"vm_type" mapstructure:"vm_type"`
	Snapshot    *bool  `json:"snapshot" yaml:"snapshot"`
	DoNotBoot   *bool  `json:"do_not_boot" yaml:"do_not_boot" structs:"do_not_boot" mapstructure:"do_not_boot"`
}

type Hardware struct {
	CPU    CPU     `json:"cpu" yaml:"cpu"`
	VCPU   int     `json:"vcpus,string" yaml:"vcpus"`
	Memory int     `json:"memory,string" yaml:"memory"`
	OSType OSType  `json:"os_type" yaml:"os_type" structs:"os_type" mapstructure:"os_type"`
	Drives []Drive `json:"drives" yaml:"drives"`
}

type Drive struct {
	Image           string `json:"image" yaml:"image"`
	Interface       string `json:"interface" yaml:"interface"`
	CacheMode       string `json:"cache_mode" yaml:"cache_mode"`
	InjectPartition *int   `json:"inject_partition,string" yaml:"inject_partition" structs:"inject_partition" mapstructure:"inject_partition"`
}

type Injection struct {
	Src         string `json:"src" yaml:"src"`
	Dst         string `json:"dst" yaml:"dst"`
	Description string `json:"description" yaml:"description"`
}
