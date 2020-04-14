package v1

import "time"

type ExperimentSpec struct {
	Topology  TopologySpec `json:"topology" yaml:"topology"`
	Scenario  ScenarioSpec `json:"scenario" yaml:"scenario"`
	Schedules []Schedule   `json:"schedules" yaml:"schedules"`
	VLANMin   int          `json:"vlanMin" yaml:"vlanMin"`
	VLANMax   int          `json:"vlanMax" yaml:"vlanMax"`
}

type Schedule struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	ClusterNode string `json:"clusterNode" yaml:"clusterNode"`
}

type ExperimentStatus struct {
	StartTime time.Time `json:"startTime" yaml:"startTime"`
}
