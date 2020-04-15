package v1

import "time"

type Schedule map[string]string

type ExperimentSpec struct {
	ExperimentName string       `json:"experimentName" yaml:"experimentName"`
	Topology       TopologySpec `json:"topology" yaml:"topology"`
	Scenario       ScenarioSpec `json:"scenario" yaml:"scenario"`
	Schedules      Schedule     `json:"schedules" yaml:"schedules"`
	VLANMin        int          `json:"vlanMin" yaml:"vlanMin"`
	VLANMax        int          `json:"vlanMax" yaml:"vlanMax"`
	RunLocal       bool         `json:"runLocal" yaml:"runLocal"`
}

type ExperimentStatus struct {
	StartTime time.Time `json:"startTime" yaml:"startTime"`
}
