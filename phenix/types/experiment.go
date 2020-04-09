package types

import "time"

type ExperimentSpec struct {
	// Topology  Property         `json:"topology" yaml:"topology"`
	// Apps      Apps             `json:"apps" yaml:"apps"`
	Schedules []Schedule `json:"schedules" yaml:"schedules"`
}

type Schedule struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	ClusterNode string `json:"clusterNode" yaml:"clusterNode"`
}

type ExperimentStatus struct {
	VLANMin   int       `json:"vlanMin" yaml:"vlanMin"`
	VLANMax   int       `json:"vlanMax" yaml:"vlanMax"`
	StartTime time.Time `json:"startTime" yaml:"startTime"`
}
