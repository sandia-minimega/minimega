package v2

import (
	ifaces "phenix/types/interfaces"
)

type ScenarioSpec struct {
	AppsF []ScenarioApp `json:"apps" yaml:"apps" structs:"apps" mapstructure:"apps"`
}

func (this *ScenarioSpec) Apps() []ifaces.ScenarioApp {
	if this == nil {
		return nil
	}

	apps := make([]ifaces.ScenarioApp, len(this.AppsF))

	for i, a := range this.AppsF {
		apps[i] = a
	}

	return apps
}

type ScenarioApp struct {
	NameF     string                 `json:"name" yaml:"name" structs:"name" mapstructure:"name"`
	AssetDirF string                 `json:"assetDir" yaml:"assetDir" structs:"assetDir" mapstructure:"assetDir"`
	MetadataF map[string]interface{} `json:"metadata" yaml:"metadata" structs:"metadata" mapstructure:"metadata"`
	HostsF    []ScenarioAppHost      `json:"hosts" yaml:"hosts" structs:"hosts" mapstructure:"hosts"`
}

func (this ScenarioApp) Name() string {
	return this.NameF
}

func (this ScenarioApp) Hosts() []ifaces.ScenarioAppHost {
	hosts := make([]ifaces.ScenarioAppHost, len(this.HostsF))

	for i, h := range this.HostsF {
		hosts[i] = h
	}

	return hosts
}

type ScenarioAppHost struct {
	HostnameF string                 `json:"hostname" yaml:"hostname" structs:"hostname" mapstructure:"hostname"`
	MetadataF map[string]interface{} `json:"metadata" yaml:"metadata" structs:"metadata" mapstructure:"metadata"`
}

func (this ScenarioAppHost) Hostname() string {
	return this.HostnameF
}

func (this ScenarioAppHost) Metadata() map[string]interface{} {
	return this.MetadataF
}
