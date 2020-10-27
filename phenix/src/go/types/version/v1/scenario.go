package v1

type ScenarioSpec struct {
	AppsF *Apps `json:"apps" yaml:"apps" structs:"apps" mapstructure:"apps"`
}

type Apps struct {
	ExperimentF []ExperimentApp `json:"experiment" yaml:"experiment" structs:"experiment" mapstructure:"experiment"`
	HostF       []HostApp       `json:"host" yaml:"host" structs:"host" mapstructure:"host"`
}

type ExperimentApp struct {
	NameF     string                 `json:"name" yaml:"name" structs:"name" mapstructure:"name"`
	AssetDirF string                 `json:"assetDir" yaml:"assetDir" structs:"assetDir" mapstructure:"assetDir"`
	MetadataF map[string]interface{} `json:"metadata" yaml:"metadata" structs:"metadata" mapstructure:"metadata"`
}

type HostApp struct {
	NameF     string `json:"name" yaml:"name" structs:"name" mapstructure:"name"`
	AssetDirF string `json:"assetDir" yaml:"assetDir" structs:"assetDir" mapstructure:"assetDir"`
	HostsF    []Host `json:"hosts" yaml:"hosts" structs:"hosts" mapstructure:"hosts"`
}

type Host struct {
	HostnameF string                 `json:"hostname" yaml:"hostname" structs:"hostname" mapstructure:"hostname"`
	MetadataF map[string]interface{} `json:"metadata" yaml:"metadata" structs:"metadata" mapstructure:"metadata"`
}
