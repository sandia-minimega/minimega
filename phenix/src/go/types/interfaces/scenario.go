package ifaces

type ScenarioSpec interface {
	Apps() []ScenarioApp
}

type ScenarioApp interface {
	Name() string
	Hosts() []ScenarioAppHost
}

type ScenarioAppHost interface {
	Hostname() string
	Metadata() map[string]interface{}
}
