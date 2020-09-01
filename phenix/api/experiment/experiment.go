package experiment

import (
	"fmt"
	"strings"
	"time"

	"phenix/app"
	"phenix/internal/mm"
	"phenix/scheduler"
	"phenix/store"
	"phenix/tmpl"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

// List collects experiments, each in a struct that references the latest
// versioned experiment spec and status. It returns a slice of experiments and
// any errors encountered while gathering and decoding them.
func List() ([]types.Experiment, error) {
	configs, err := store.List("Experiment")
	if err != nil {
		return nil, fmt.Errorf("getting list of experiment configs from store: %w", err)
	}

	var experiments []types.Experiment

	for _, c := range configs {
		spec := new(v1.ExperimentSpec)

		if err := mapstructure.Decode(c.Spec, spec); err != nil {
			return nil, fmt.Errorf("decoding experiment spec: %w", err)
		}

		status := new(v1.ExperimentStatus)

		if err := mapstructure.Decode(c.Status, status); err != nil {
			return nil, fmt.Errorf("decoding experiment status: %w", err)
		}

		exp := types.Experiment{Metadata: c.Metadata, Spec: spec, Status: status}

		experiments = append(experiments, exp)
	}

	return experiments, nil
}

// Get retrieves the experiment with the given name. It returns a pointer to a
// struct that references the latest versioned experiment spec and status for
// the given experiment, and any errors encountered while retrieving the
// experiment.
func Get(name string) (*types.Experiment, error) {
	if name == "" {
		return nil, fmt.Errorf("no experiment name provided")
	}

	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return nil, fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	spec := new(v1.ExperimentSpec)

	if err := mapstructure.Decode(c.Spec, spec); err != nil {
		return nil, fmt.Errorf("decoding experiment spec: %w", err)
	}

	status := new(v1.ExperimentStatus)

	if err := mapstructure.Decode(c.Status, status); err != nil {
		return nil, fmt.Errorf("decoding experiment status: %w", err)
	}

	exp := &types.Experiment{Metadata: c.Metadata, Spec: spec, Status: status}

	return exp, nil
}

// Create uses the provided arguments to create a new experiment. The
// `scenarioName` argument can be an empty string, in which case no scenario is
// used for the experiment. The `baseDir` argument can be an empty string, in
// which case the default value of `/phenix/experiments/{name}` is used for the
// experiment base directory. It returns any errors encountered while creating
// the experiment.
func Create(opts ...CreateOption) error {
	o := newCreateOptions(opts...)

	if o.name == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if o.topology == "" {
		return fmt.Errorf("no topology name provided")
	}

	topo, _ := types.NewConfig("topology/" + o.topology)

	if err := store.Get(topo); err != nil {
		return fmt.Errorf("topology doesn't exist")
	}

	meta := types.ConfigMetadata{
		Name: o.name,
		Annotations: map[string]string{
			"topology": o.topology,
		},
	}

	spec := map[string]interface{}{
		"experimentName": o.name,
		"baseDir":        o.baseDir,
		"topology":       topo.Spec,
	}

	var scenario *types.Config

	if o.scenario != "" {
		scenario, _ = types.NewConfig("scenario/" + o.scenario)

		if err := store.Get(scenario); err != nil {
			return fmt.Errorf("scenario doesn't exist")
		}

		meta.Annotations["scenario"] = o.scenario
		spec["scenario"] = scenario.Spec
	}

	c := &types.Config{
		Version:  "phenix.sandia.gov/v1",
		Kind:     "Experiment",
		Metadata: meta,
		Spec:     spec,
	}

	if err := create(c); err != nil {
		return fmt.Errorf("creating experiment config: %w", err)
	}

	if err := types.ValidateConfigSpec(*c); err != nil {
		return fmt.Errorf("validating experiment config: %w", err)
	}

	if err := store.Create(c); err != nil {
		return fmt.Errorf("storing experiment config: %w", err)
	}

	return nil
}

// CreateFromConfig uses the provided config argument to create a new
// experiment. The provided config must be of kind `Experiment`, and must
// contain an annotation in its metadata identifying the topology to use for the
// experiment. A scenario annotation may also be provided, but is not required.
// It returns any errors encountered while creating the experiment.
func CreateFromConfig(c *types.Config) error {
	topoName, ok := c.Metadata.Annotations["topology"]
	if !ok {
		return fmt.Errorf("topology annotation missing from experiment")
	}

	scenarioName := c.Metadata.Annotations["scenario"]

	topo, _ := types.NewConfig("topology/" + topoName)

	if err := store.Get(topo); err != nil {
		return fmt.Errorf("topology doesn't exist")
	}

	if c.Spec == nil {
		c.Spec = make(map[string]interface{})
	}

	if _, ok := c.Spec["experimentName"]; !ok {
		c.Spec["experimentName"] = c.Metadata.Name
	}

	c.Spec["topology"] = topo.Spec

	if scenarioName != "" {
		scenario, _ := types.NewConfig("scenario/" + scenarioName)

		if err := store.Get(scenario); err != nil {
			return fmt.Errorf("scenario doesn't exist")
		}

		topo, ok := scenario.Metadata.Annotations["topology"]
		if !ok {
			return fmt.Errorf("topology annotation missing from scenario")
		}

		if topo != topoName {
			return fmt.Errorf("experiment/scenario topology mismatch")
		}

		c.Spec["scenario"] = scenario.Spec
	}

	return create(c)
}

func create(c *types.Config) error {
	var spec v1.ExperimentSpec

	if err := mapstructure.Decode(c.Spec, &spec); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	spec.SetDefaults()

	if err := spec.VerifyScenario(); err != nil {
		return fmt.Errorf("verifying experiment scenario: %w", err)
	}

	exp := types.Experiment{Metadata: c.Metadata, Spec: &spec}

	if err := app.ApplyApps(app.ACTIONCONFIG, &exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp.Spec, structs.CASESNAKE)

	return nil
}

// Schedule applies the given scheduling algorithm to the experiment with the
// given name. It returns any errors encountered while scheduling the
// experiment.
func Schedule(opts ...ScheduleOption) error {
	o := newScheduleOptions(opts...)

	c, _ := types.NewConfig("experiment/" + o.name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", o.name, err)
	}

	status := new(v1.ExperimentStatus)

	if err := mapstructure.Decode(c.Status, status); err != nil {
		return fmt.Errorf("decoding experiment status: %w", err)
	}

	if status.StartTime != "" {
		return fmt.Errorf("experiment already running (started at: %s)", status.StartTime)
	}

	exp := new(v1.ExperimentSpec)

	if err := mapstructure.Decode(c.Spec, exp); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	if err := scheduler.Schedule(o.algorithm, exp); err != nil {
		return fmt.Errorf("running scheduler algorithm: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}

// Start starts the experiment with the given name. It returns any errors
// encountered while starting the experiment.
func Start(name string, dryrun bool) error {
	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	var status v1.ExperimentStatus

	if err := mapstructure.Decode(c.Status, &status); err != nil {
		return fmt.Errorf("decoding experiment status: %w", err)
	}

	if status.StartTime != "" && !strings.HasSuffix(status.StartTime, "-DRYRUN") {
		return fmt.Errorf("experiment already running (started at: %s)", status.StartTime)
	}

	var spec v1.ExperimentSpec

	if err := mapstructure.Decode(c.Spec, &spec); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	exp := types.Experiment{Metadata: c.Metadata, Spec: &spec, Status: &status}

	if err := app.ApplyApps(app.ACTIONPRESTART, &exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	filename := fmt.Sprintf("%s/mm_files/%s.mm", exp.Spec.BaseDir, exp.Spec.ExperimentName)

	if err := tmpl.CreateFileFromTemplate("minimega_script.tmpl", exp.Spec, filename); err != nil {
		return fmt.Errorf("generating minimega script: %w", err)
	}

	if dryrun {
		status.VLANs = spec.VLANs.Aliases
	} else {
		if err := mm.ReadScriptFromFile(filename); err != nil {
			return fmt.Errorf("reading minimega script: %w", err)
		}

		if err := mm.LaunchVMs(exp.Spec.ExperimentName); err != nil {
			return fmt.Errorf("launching experiment VMs: %w", err)
		}

		schedule := make(v1.Schedule)

		for _, vm := range mm.GetVMInfo(mm.NS(exp.Spec.ExperimentName)) {
			schedule[vm.Name] = vm.Host
		}

		status.Schedules = schedule

		vlans, err := mm.GetVLANs(mm.NS(exp.Spec.ExperimentName))
		if err != nil {
			return fmt.Errorf("processing experiment VLANs: %w", err)
		}

		status.VLANs = vlans
	}

	if dryrun {
		status.StartTime = time.Now().Format(time.RFC3339) + "-DRYRUN"
	} else {
		status.StartTime = time.Now().Format(time.RFC3339)
	}

	if err := app.ApplyApps(app.ACTIONPOSTSTART, &exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp.Spec, structs.CASESNAKE)
	c.Status = structs.MapDefaultCase(exp.Status, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}

// Stop stops the experiment with the given name. It returns any errors
// encountered while stopping the experiment.
func Stop(name string) error {
	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	var status v1.ExperimentStatus

	if err := mapstructure.Decode(c.Status, &status); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	if status.StartTime == "" {
		return fmt.Errorf("experiment isn't running")
	}

	dryrun := strings.HasSuffix(status.StartTime, "-DRYRUN")

	var spec v1.ExperimentSpec

	if err := mapstructure.Decode(c.Spec, &spec); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	exp := types.Experiment{Metadata: c.Metadata, Spec: &spec, Status: &status}

	if err := app.ApplyApps(app.ACTIONCLEANUP, &exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	if !dryrun {
		if err := mm.ClearNamespace(exp.Spec.ExperimentName); err != nil {
			return fmt.Errorf("killing experiment VMs: %w", err)
		}
	}

	exp.Status.StartTime = ""

	c.Spec = structs.MapDefaultCase(exp.Spec, structs.CASESNAKE)
	c.Status = structs.MapDefaultCase(exp.Status, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}

func Running(name string) bool {
	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return false
	}

	var status v1.ExperimentStatus

	if err := mapstructure.Decode(c.Status, &status); err != nil {
		return false
	}

	return status.Running()
}

func Save(opts ...SaveOption) error {
	o := newSaveOptions(opts...)

	if o.name == "" {
		return fmt.Errorf("experiment name required")
	}

	c, _ := types.NewConfig("experiment/" + o.name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", o.name, err)
	}

	if o.spec == nil {
		if o.saveNilSpec {
			c.Spec = nil
		}
	} else {
		c.Spec = structs.MapDefaultCase(o.spec, structs.CASESNAKE)
	}

	if o.status == nil {
		if o.saveNilStatus {
			c.Status = nil
		}
	} else {
		c.Status = structs.MapDefaultCase(o.status, structs.CASESNAKE)
	}

	if err := store.Update(c); err != nil {
		return fmt.Errorf("saving experiment config: %w", err)
	}

	return nil
}

func Delete(name string) error {
	// TODO

	return nil
}

func Files(name string) ([]string, error) {
	// TODO

	return nil, nil
}

func File(name, file string) ([]byte, error) {
	// TODO

	return nil, nil
}
