package types

import "time"

type Config struct {
	Version  string                 `json:"version" yaml:"version"`
	Kind     string                 `json:"kind" yaml:"kind"`
	Metadata ConfigMetadata         `json:"metadata" yaml:"metadata"`
	Spec     map[string]interface{} `json:"spec" yaml:"spec"`
}

type ConfigMetadata struct {
	Name    string    `json:"name" yaml:"name"`
	Created time.Time `json:"created" yaml:"created"`
}
