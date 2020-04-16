package types

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

const API_GROUP = "phenix.sandia.gov"

type Configs []Config

func NewConfig(kind, name string) *Config {
	return &Config{
		Kind: kind,
		Metadata: ConfigMetadata{
			Name: name,
		},
	}
}

func NewConfigFromFile(path string) (*Config, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config: %w", err)
	}

	var config Config

	switch filepath.Ext(path) {
	case ".json":
		if err := json.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(file, &config); err != nil {
			return nil, fmt.Errorf("unmarshaling config: %w", err)
		}
	default:
		return nil, fmt.Errorf("invalid config extension")
	}

	return &config, nil
}

type Config struct {
	Version  string                 `json:"apiVersion" yaml:"apiVersion"`
	Kind     string                 `json:"kind" yaml:"kind"`
	Metadata ConfigMetadata         `json:"metadata" yaml:"metadata"`
	Spec     map[string]interface{} `json:"spec" yaml:"spec"`
	Status   map[string]interface{} `json:"status,omitempty" yaml:"status,omitempty"`
}

type ConfigMetadata struct {
	Name        string      `json:"name" yaml:"name"`
	Created     time.Time   `json:"created" yaml:"created"`
	Updated     time.Time   `json:"updated" yaml:"updated"`
	Annotations Annotations `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type Annotations map[string]string

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
