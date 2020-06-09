package v1

import (
	"strings"
)

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
	Metadata   Metadata     `json:"metadata" yaml:"metadata"`
}

type General struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	Description string `json:"description" yaml:"description"`
	VMType      VMType `json:"vm_type" yaml:"vm_type" mapstructure:"vm_type"`
	Snapshot    *bool  `json:"snapshot" yaml:"snapshot"`
	DoNotBoot   *bool  `json:"do_not_boot" yaml:"do_not_boot" structs:"do_not_boot" mapstructure:"do_not_boot"`
}

type Hardware struct {
	CPU    CPU     `json:"cpu" yaml:"cpu"`
	VCPU   int     `json:"vcpus" yaml:"vcpus"`
	Memory int     `json:"memory" yaml:"memory"`
	OSType OSType  `json:"os_type" yaml:"os_type" mapstructure:"os_type"`
	Drives []Drive `json:"drives" yaml:"drives"`
}

type Drive struct {
	Image           string `json:"image" yaml:"image"`
	Interface       string `json:"interface" yaml:"interface"`
	CacheMode       string `json:"cache_mode" yaml:"cache_mode"`
	InjectPartition *int   `json:"inject_partition" yaml:"inject_partition" mapstructure:"inject_partition"`
}

type Injection struct {
	Src         string `json:"src" yaml:"src"`
	Dst         string `json:"dst" yaml:"dst"`
	Description string `json:"description" yaml:"description"`
}

func (this *Node) SetDefaults() {
	if this.General.VMType == VMType_NotSet {
		this.General.VMType = VMType_KVM
	}

	if this.General.Snapshot == nil {
		snapshot := true
		this.General.Snapshot = &snapshot
	}

	if this.General.DoNotBoot == nil {
		dnb := false
		this.General.DoNotBoot = &dnb
	}

	if this.Hardware.CPU == CPU_NotSet {
		this.Hardware.CPU = CPU_Broadwell
	}

	if this.Hardware.VCPU == 0 {
		this.Hardware.VCPU = 1
	}

	if this.Hardware.Memory == 0 {
		this.Hardware.Memory = 512
	}

	if this.Hardware.OSType == OSType_NotSet {
		this.Hardware.OSType = OSType_Linux
	}
}

func (this Node) FileInjects(basedir string) string {
	injects := make([]string, len(this.Injections))

	for i, inject := range this.Injections {
		if strings.HasPrefix(inject.Src, "/") {
			injects[i] = inject.Src + ":" + inject.Dst
		} else {
			injects[i] = basedir + "/" + inject.Src + ":" + inject.Dst
		}
	}

	return strings.Join(injects, " ")
}

func (this Node) RouterName() string {
	if !strings.EqualFold(this.Type, "router") {
		return this.General.Hostname
	}

	name := strings.ToLower(this.General.Hostname)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")

	return name
}

func (this Hardware) DiskConfig(snapshot string) string {
	configs := make([]string, len(this.Drives))

	for i, d := range this.Drives {
		config := []string{d.Image}

		if i == 0 && snapshot != "" {
			config[0] = snapshot
		}

		if d.Interface != "" {
			config = append(config, d.Interface)
		}

		if d.CacheMode != "" {
			config = append(config, d.CacheMode)
		}

		configs[i] = strings.Join(config, ",")
	}

	return strings.Join(configs, " ")
}

func (this Drive) GetInjectPartition() int {
	if this.InjectPartition == nil {
		return 1
	}

	return *this.InjectPartition
}
