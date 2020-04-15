package v1

import "strings"

type Node struct {
	Type       string      `json:"type" yaml:"type"`
	General    General     `json:"general" yaml:"general"`
	Hardware   Hardware    `json:"hardware" yaml:"hardware"`
	Network    Network     `json:"network" yaml:"network"`
	Injections []Injection `json:"injections" yaml:"injections"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
}

type General struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	Description string `json:"description" yaml:"description"`
	VMType      string `json:"vm_type" yaml:"vm_type"`
	Snapshot    bool   `json:"snapshot" yaml:"snapshot"`
	DoNotBoot   bool   `json:"do_not_boot" yaml:"do_not_boot"`
}

type Hardware struct {
	CPU    string  `json:"cpu" yaml:"cpu"`
	VCPU   int     `json:"vcpus" yaml:"vcpus"`
	Memory int     `json:"memory" yaml:"memory"`
	OSType string  `json:"os_type" yaml:"os_type"`
	Drives []Drive `json:"drives" yaml:"drives"`
}

type Drive struct {
	Image           string `json:"image" yaml:"image"`
	Interface       string `json:"interface" yaml:"interface"`
	CacheMode       string `json:"cache_mode" yaml:"cache_mode"`
	InjectPartition int    `json:"inject_partition" yaml:"inject_partition"`
}

type Injection struct {
	Src         string `json:"src" yaml:"src"`
	Dst         string `json:"dst" yaml:"dst"`
	Description string `json:"description" yaml:"description"`
}

func (this Node) FileInjects() string {
	injects := make([]string, len(this.Injections))

	for i, inject := range this.Injections {
		injects[i] = inject.Src + ":" + inject.Dst
	}

	return strings.Join(injects, " ")
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
