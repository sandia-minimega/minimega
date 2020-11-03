package v0

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	ifaces "phenix/types/interfaces"
)

type Node struct {
	LabelsF     map[string]string `json:"labels" yaml:"labels" structs:"labels" mapstructure:"labels"`
	TypeF       string            `json:"type" yaml:"type" structs:"type" mapstructure:"type"`
	GeneralF    *General          `json:"general" yaml:"general" structs:"general" mapstructure:"general"`
	HardwareF   *Hardware         `json:"hardware" yaml:"hardware" structs:"hardware" mapstructure:"hardware"`
	NetworkF    *Network          `json:"network" yaml:"network" structs:"network" mapstructure:"network"`
	InjectionsF []*Injection      `json:"injections" yaml:"injections" structs:"injections" mapstructure:"injections"`
}

func (this Node) Labels() map[string]string {
	return this.LabelsF
}

func (this Node) Type() string {
	return this.TypeF
}

func (this Node) General() ifaces.NodeGeneral {
	return this.GeneralF
}

func (this Node) Hardware() ifaces.NodeHardware {
	return this.HardwareF
}

func (this Node) Network() ifaces.NodeNetwork {
	return this.NetworkF
}

func (this Node) Injections() []ifaces.NodeInjection {
	injects := make([]ifaces.NodeInjection, len(this.InjectionsF))

	for i, j := range this.InjectionsF {
		injects[i] = j
	}

	return injects
}

func (this *Node) AddInject(src, dst, perms, desc string) {
	this.InjectionsF = append(this.InjectionsF, &Injection{
		SrcF:         src,
		DstF:         dst,
		PermissionsF: perms,
		DescriptionF: desc,
	})
}

func (this *Node) SetInjections(injections []ifaces.NodeInjection) {
	injects := make([]*Injection, len(injections))

	for i, j := range injections {
		injects[i] = j.(*Injection)
	}

	this.InjectionsF = injects
}

type General struct {
	HostnameF    string `json:"hostname" yaml:"hostname" structs:"hostname" mapstructure:"hostname"`
	DescriptionF string `json:"description" yaml:"description" structs:"description" mapstructure:"description"`
	VMTypeF      string `json:"vm_type" yaml:"vm_type" structs:"vm_type" mapstructure:"vm_type"`
	SnapshotF    *bool  `json:"snapshot" yaml:"snapshot" structs:"snapshot" mapstructure:"snapshot"`
	DoNotBootF   *bool  `json:"do_not_boot" yaml:"do_not_boot" structs:"do_not_boot" mapstructure:"do_not_boot"`
}

func (this General) Hostname() string {
	return this.HostnameF
}

func (this General) Description() string {
	return this.DescriptionF
}

func (this General) VMType() string {
	return this.VMTypeF
}

func (this General) Snapshot() *bool {
	return this.SnapshotF
}

func (this General) DoNotBoot() *bool {
	return this.DoNotBootF
}

func (this *General) SetDoNotBoot(b bool) {
	this.DoNotBootF = &b
}

type Hardware struct {
	CPUF    string   `json:"cpu" yaml:"cpu" structs:"cpu" mapstructure:"cpu"`
	VCPUF   int      `json:"vcpus,string" yaml:"vcpus" structs:"vcpus" mapstructure:"vcpus"`
	MemoryF int      `json:"memory,string" yaml:"memory" structs:"memory" mapstructure:"memory"`
	OSTypeF string   `json:"os_type" yaml:"os_type" structs:"os_type" mapstructure:"os_type"`
	DrivesF []*Drive `json:"drives" yaml:"drives" structs:"drives" mapstructure:"drives"`
}

func (this Hardware) CPU() string {
	return this.CPUF
}

func (this Hardware) VCPU() int {
	return this.VCPUF
}

func (this Hardware) Memory() int {
	return this.MemoryF
}

func (this Hardware) OSType() string {
	return this.OSTypeF
}

func (this Hardware) Drives() []ifaces.NodeDrive {
	drives := make([]ifaces.NodeDrive, len(this.DrivesF))

	for i, d := range this.DrivesF {
		drives[i] = d
	}

	return drives
}

func (this *Hardware) SetVCPU(v int) {
	this.VCPUF = v
}

func (this *Hardware) SetMemory(m int) {
	this.MemoryF = m
}

type Drive struct {
	ImageF           string `json:"image" yaml:"image" structs:"image" mapstructure:"image"`
	IfaceF           string `json:"interface" yaml:"interface" structs:"interface" mapstructure:"interface"`
	CacheModeF       string `json:"cache_mode" yaml:"cache_mode" structs:"cache_mode" mapstructure:"cache_mode"`
	InjectPartitionF *int   `json:"inject_partition,string" yaml:"inject_partition" structs:"inject_partition" mapstructure:"inject_partition"`
}

func (this Drive) Image() string {
	return this.ImageF
}

func (this Drive) Interface() string {
	return this.IfaceF
}

func (this Drive) CacheMode() string {
	return this.CacheModeF
}

func (this Drive) InjectPartition() *int {
	return this.InjectPartitionF
}

func (this *Drive) SetImage(i string) {
	this.ImageF = i
}

type Injection struct {
	SrcF         string `json:"src" yaml:"src" structs:"src" mapstructure:"src"`
	DstF         string `json:"dst" yaml:"dst" structs:"dst" mapstructure:"dst"`
	DescriptionF string `json:"description" yaml:"description" structs:"description" mapstructure:"description"`
	PermissionsF string `json:"permissions" yaml:"permissions" structs:"permissions" mapstructure:"permissions"`
}

func (this Injection) Src() string {
	return this.SrcF
}

func (this Injection) Dst() string {
	return this.DstF
}

func (this Injection) Description() string {
	return this.DescriptionF
}

func (this Injection) Permissions() string {
	return this.PermissionsF
}

func (this *Node) SetDefaults() {
	if this.GeneralF.VMTypeF == "" {
		this.GeneralF.VMTypeF = "kvm"
	}

	if this.GeneralF.SnapshotF == nil {
		snapshot := true
		this.GeneralF.SnapshotF = &snapshot
	}

	if this.GeneralF.DoNotBootF == nil {
		dnb := false
		this.GeneralF.DoNotBootF = &dnb
	}

	if this.HardwareF.CPUF == "" {
		this.HardwareF.CPUF = "Broadwell"
	}

	if this.HardwareF.VCPUF == 0 {
		this.HardwareF.VCPUF = 1
	}

	if this.HardwareF.MemoryF == 0 {
		this.HardwareF.MemoryF = 512
	}

	if this.HardwareF.OSTypeF == "" {
		this.HardwareF.OSTypeF = "linux"
	}

	this.NetworkF.SetDefaults()
}

func (this Node) FileInjects(baseDir string) string {
	injects := make([]string, len(this.InjectionsF))

	for i, inject := range this.InjectionsF {
		if strings.HasPrefix(inject.SrcF, "/") {
			injects[i] = fmt.Sprintf(`"%s":"%s"`, inject.SrcF, inject.DstF)
		} else {
			injects[i] = fmt.Sprintf(`"%s/%s":"%s"`, baseDir, inject.SrcF, inject.DstF)
		}

		if inject.PermissionsF != "" && len(inject.PermissionsF) <= 4 {
			if perms, err := strconv.ParseInt(inject.PermissionsF, 8, 64); err == nil {
				// Update file permissions on local disk before it gets injected into
				// disk image.
				os.Chmod(inject.SrcF, os.FileMode(perms))
			}
		}
	}

	return strings.Join(injects, " ")
}

func (this Node) RouterName() string {
	if !strings.EqualFold(this.TypeF, "router") {
		return this.GeneralF.HostnameF
	}

	name := strings.ToLower(this.GeneralF.HostnameF)
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "_", "-")

	return name
}

func (this Hardware) DiskConfig(snapshot string) string {
	configs := make([]string, len(this.DrivesF))

	for i, d := range this.DrivesF {
		config := []string{d.ImageF}

		if i == 0 && snapshot != "" {
			config[0] = snapshot
		}

		if d.IfaceF != "" {
			config = append(config, d.IfaceF)
		}

		if d.CacheModeF != "" {
			config = append(config, d.CacheModeF)
		}

		configs[i] = strings.Join(config, ",")
	}

	return strings.Join(configs, " ")
}

func (this Drive) GetInjectPartition() int {
	if this.InjectPartitionF == nil {
		return 1
	}

	return *this.InjectPartitionF
}
