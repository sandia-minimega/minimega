package ifaces

import "context"

type VLANSpec interface {
	Aliases() map[string]int
	Min() int
	Max() int

	SetAliases(map[string]int)
	SetMin(int)
	SetMax(int)
}

type ExperimentSpec interface {
	ExperimentName() string
	BaseDir() string
	Topology() TopologySpec
	Scenario() ScenarioSpec
	VLANs() VLANSpec
	Schedules() map[string]string
	RunLocal() bool

	SetDefaults()
	SetVLANAlias(string, int, bool) error
	SetVLANRange(int, int, bool) error
	SetSchedule(map[string]string)

	VerifyScenario(context.Context) error
	ScheduleNode(string, string) error
}

type ExperimentStatus interface {
	StartTime() string
	AppStatus() map[string]interface{}
	VLANs() map[string]int

	SetStartTime(string)
	SetAppStatus(string, interface{})
	SetVLANs(map[string]int)
	SetSchedule(map[string]string)
}
