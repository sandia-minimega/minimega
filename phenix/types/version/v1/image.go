package v1

import (
	"strings"
)

type Format string

const (
	Format_Raw   Format = "raw"
	Format_Qcow2 Format = "qcow2"
	Format_Vmdk  Format = "vmdk"
	Format_Vdi   Format = "vdi"
	Format_Vhdx  Format = "vhdx"
)

type Image struct {
	Variant   string   `json:"variant" yaml:"variant"`
	Release   string   `json:"release" yaml:"release"`
	Format    Format   `json:"format" yaml:"format"`
	Ramdisk   bool     `json:"ramdisk" yaml:"ramdisk"`
	Compress  bool     `json:"compress" yaml:"compress"`
	Size      string   `json:"size" yaml:"size"`
	Mirror    string   `json:"mirror" yaml:"mirror"`
	DebAppend string   `json:"deb_append" yaml:"deb_append"`
	Packages  []string `json:"packages" yaml:"packages"`
	Overlays  []string `json:"overlays" yaml:"overlays"`
	Scripts   []string `json:"default_script" yaml:"default_script"`
	Verbosity string   `json:"verbosity" yaml:"verbosity"`
	Cache     bool     `json:"cache" yaml:"cache"`

	ScriptPaths []string `json:"-" yaml:"-"`
}

func (this Image) PackageList() string {
	if this.Packages == nil {
		return ""
	}

	return "--include " + strings.Join(this.Packages, ",")
}

func (this Image) PostBuild() string {
	var post []string

	for _, l := range this.Scripts {
		if l == "" {
			continue
		}

		// Add 6 spaces to script lines so YAML is formatted correctly in vmdb file.
		post = append(post, "      "+l)
	}

	return strings.Join(post, "\n")
}

func (this Image) Verbose() string {
	if this.Verbosity == "vvv" {
		return "--verbose"
	}

	return ""
}
