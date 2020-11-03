package store

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"phenix/types/version"

	"gopkg.in/yaml.v3"
)

const API_GROUP = "phenix.sandia.gov"

type (
	Configs     []Config
	Annotations map[string]string
)

type Config struct {
	Version  string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind     string                 `json:"kind" yaml:"kind"`
	Metadata ConfigMetadata         `json:"metadata" yaml:"metadata"`
	Spec     map[string]interface{} `json:"spec" yaml:"spec"`
	Status   map[string]interface{} `json:"status,omitempty" yaml:"status,omitempty"`
}

type ConfigMetadata struct {
	Name        string      `json:"name" yaml:"name"`
	Created     string      `json:"created" yaml:"created"`
	Updated     string      `json:"updated" yaml:"updated"`
	Annotations Annotations `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

func NewConfig(name string) (*Config, error) {
	n := strings.Split(name, "/")

	if len(n) != 2 {
		return nil, fmt.Errorf("invalid config name provided: %s", name)
	}

	kind, name := n[0], n[1]
	kind = strings.Title(kind)

	version := version.StoredVersion[kind]
	version = API_GROUP + "/" + version

	c := Config{
		Version: version,
		Kind:    kind,
		Metadata: ConfigMetadata{
			Name: name,
		},
	}

	return &c, nil
}

func NewConfigFromFile(path string) (*Config, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var c Config

	switch filepath.Ext(path) {
	case ".json":
		if err := json.Unmarshal(file, &c); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(file, &c); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid config extension")
	}

	return &c, nil
}

func (this Config) APIGroup() string {
	s := strings.Split(this.Version, "/")

	if len(s) < 2 {
		return ""
	}

	return s[0]
}

func (this Config) APIVersion() string {
	s := strings.Split(this.Version, "/")

	if len(s) == 0 {
		return ""
	} else if len(s) == 1 {
		return s[0]
	} else {
		return s[1]
	}
}
