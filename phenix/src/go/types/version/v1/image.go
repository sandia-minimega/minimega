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
	Variant     string            `json:"variant" yaml:"variant"`
	Release     string            `json:"release" yaml:"release"`
	Format      Format            `json:"format" yaml:"format"`
	Ramdisk     bool              `json:"ramdisk" yaml:"ramdisk"`
	Compress    bool              `json:"compress" yaml:"compress"`
	Size        string            `json:"size" yaml:"size"`
	Mirror      string            `json:"mirror" yaml:"mirror"`
	DebAppend   string            `json:"deb_append" yaml:"deb_append" structs:"deb_append" mapstructure:"deb_append"`
	Packages    []string          `json:"packages" yaml:"packages"`
	Overlays    []string          `json:"overlays" yaml:"overlays"`
	Scripts     map[string]string `json:"scripts" yaml:"scripts"`
	ScriptOrder []string          `json:"script_order" yaml:"script_order" structs:"script_order" mapstructure:"script_order"`

	IncludeMiniccc   string `json:"include_miniccc" yaml:"include_miniccc" structs:"include_miniccc" mapstructure:"include_miniccc"`
	IncludeProtonuke string `json:"include_protonuke" yaml:"include_protonuke" structs:"include_protonuke" mapstructure:"include_protonuke"`

	Cache       bool     `json:"-" yaml:"-" structs:"-" mapstructure:"-"`
	ScriptPaths []string `json:"-" yaml:"-" structs:"-" mapstructure:"-"`
	VerboseLogs bool     `json:"-" yaml:"-" structs:"-" mapstructure:"-"`
}

func (this Image) PackageList() string {
	if this.Packages == nil {
		return ""
	}

	return "--include " + strings.Join(this.Packages, ",")
}

func (this Image) PostBuild() string {
	var post []string

	for _, o := range this.ScriptOrder {
		s := this.Scripts[o]

		for _, l := range strings.Split(s, "\n") {
			if l == "" {
				continue
			}

			// Add 6 spaces to script lines so YAML is formatted correctly in vmdb file.
			post = append(post, "      "+l)
		}
	}

	return strings.Join(post, "\n")
}

func (this Image) Verbose() string {
	if this.VerboseLogs {
		return "--verbose"
	}

	return ""
}
