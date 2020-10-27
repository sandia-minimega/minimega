package experiment

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"phenix/app"
	"phenix/internal/common"
	"phenix/internal/file"
	"phenix/internal/mm"
	"phenix/scheduler"
	"phenix/store"
	"phenix/tmpl"
	"phenix/types"
	"phenix/types/version"

	"github.com/activeshadow/structs"
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
		exp, err := types.DecodeExperimentFromConfig(c)
		if err != nil {
			return nil, fmt.Errorf("decoding experiment from config: %w", err)
		}

		experiments = append(experiments, *exp)
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

	c, err := types.NewConfig("experiment/" + name)
	if err != nil {
		return nil, fmt.Errorf("getting experiment: %w", err)
	}

	if err := store.Get(c); err != nil {
		return nil, fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return nil, fmt.Errorf("decoding experiment from config: %w", err)
	}

	return exp, nil
}

// Create uses the provided arguments to create a new experiment. The
// `scenarioName` argument can be an empty string, in which case no scenario is
// used for the experiment. The `baseDir` argument can be an empty string, in
// which case the default value of `/phenix/experiments/{name}` is used for the
// experiment base directory. It returns any errors encountered while creating
// the experiment.
func Create(ctx context.Context, opts ...CreateOption) error {
	o := newCreateOptions(opts...)

	if o.name == "" {
		return fmt.Errorf("no experiment name provided")
	}

	if strings.ToLower(o.name) == "all" {
		return fmt.Errorf("cannot use 'all' for experiment name")
	}

	if o.topology == "" {
		return fmt.Errorf("no topology name provided")
	}

	var (
		kind       = "Experiment"
		apiVersion = version.StoredVersion[kind]
	)

	topoC, _ := types.NewConfig("topology/" + o.topology)

	if err := store.Get(topoC); err != nil {
		return fmt.Errorf("topology doesn't exist")
	}

	// This will upgrade the toplogy to the latest known version if needed.
	topo, err := types.DecodeTopologyFromConfig(*topoC)
	if err != nil {
		return fmt.Errorf("decoding topology from config: %w", err)
	}

	meta := types.ConfigMetadata{
		Name: o.name,
		Annotations: map[string]string{
			"topology": o.topology,
		},
	}

	specMap := map[string]interface{}{
		"experimentName": o.name,
		"baseDir":        o.baseDir,
		"topology":       topo,
	}

	if o.scenario != "" {
		scenarioC, _ := types.NewConfig("scenario/" + o.scenario)

		if err := store.Get(scenarioC); err != nil {
			return fmt.Errorf("scenario doesn't exist")
		}

		topo, ok := scenarioC.Metadata.Annotations["topology"]
		if !ok {
			return fmt.Errorf("topology annotation missing from scenario")
		}

		if topo != o.topology {
			return fmt.Errorf("experiment/scenario topology mismatch")
		}

		// This will upgrade the scenario to the latest known version if needed.
		scenario, err := types.DecodeScenarioFromConfig(*scenarioC)
		if err != nil {
			return fmt.Errorf("decoding scenario from config: %w", err)
		}

		meta.Annotations["scenario"] = o.scenario
		specMap["scenario"] = scenario
	}

	c := &types.Config{
		Version:  types.API_GROUP + "/" + apiVersion,
		Kind:     kind,
		Metadata: meta,
		Spec:     specMap,
	}

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return fmt.Errorf("decoding experiment from config: %w", err)
	}

	exp.Spec.SetVLANRange(o.vlanMin, o.vlanMax, false)

	exp.Spec.SetDefaults()

	if err := exp.Spec.VerifyScenario(ctx); err != nil {
		return fmt.Errorf("verifying experiment scenario: %w", err)
	}

	if err := app.ApplyApps(app.ACTIONCONFIG, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp.Spec, structs.CASESNAKE)

	if err := types.ValidateConfigSpec(*c); err != nil {
		return fmt.Errorf("validating experiment config: %w", err)
	}

	if err := store.Create(c); err != nil {
		return fmt.Errorf("storing experiment config: %w", err)
	}

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

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return fmt.Errorf("decoding experiment from config: %w", err)
	}

	if exp.Running() {
		return fmt.Errorf("experiment already running (started at: %s)", exp.Status.StartTime())
	}

	if err := scheduler.Schedule(o.algorithm, exp.Spec); err != nil {
		return fmt.Errorf("running scheduler algorithm: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp.Spec, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}

// Start starts the experiment with the given name. It returns any errors
// encountered while starting the experiment.
func Start(opts ...StartOption) error {
	o := newStartOptions(opts...)

	c, _ := types.NewConfig("experiment/" + o.name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", o.name, err)
	}

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return fmt.Errorf("decoding experiment from config: %w", err)
	}

	if exp.Running() {
		if !strings.HasSuffix(exp.Status.StartTime(), "-DRYRUN") {
			return fmt.Errorf("experiment already running (started at: %s)", exp.Status.StartTime())
		}
	}

	if o.vlanMin != 0 {
		exp.Spec.VLANs().SetMin(o.vlanMin)
	}

	if o.vlanMax != 0 {
		exp.Spec.VLANs().SetMax(o.vlanMax)
	}

	if err := app.ApplyApps(app.ACTIONPRESTART, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	filename := fmt.Sprintf("%s/mm_files/%s.mm", exp.Spec.BaseDir(), exp.Spec.ExperimentName())

	if err := tmpl.CreateFileFromTemplate("minimega_script.tmpl", exp.Spec, filename); err != nil {
		return fmt.Errorf("generating minimega script: %w", err)
	}

	if o.dryrun {
		exp.Status.SetVLANs(exp.Spec.VLANs().Aliases())
	} else {
		if err := mm.ReadScriptFromFile(filename); err != nil {
			mm.ClearNamespace(exp.Spec.ExperimentName())
			return fmt.Errorf("reading minimega script: %w", err)
		}

		if err := mm.LaunchVMs(exp.Spec.ExperimentName()); err != nil {
			mm.ClearNamespace(exp.Spec.ExperimentName())
			return fmt.Errorf("launching experiment VMs: %w", err)
		}

		schedule := make(map[string]string)

		for _, vm := range mm.GetVMInfo(mm.NS(exp.Spec.ExperimentName())) {
			schedule[vm.Name] = vm.Host
		}

		exp.Status.SetSchedule(schedule)

		vlans, err := mm.GetVLANs(mm.NS(exp.Spec.ExperimentName()))
		if err != nil {
			return fmt.Errorf("processing experiment VLANs: %w", err)
		}

		exp.Status.SetVLANs(vlans)
	}

	if o.dryrun {
		exp.Status.SetStartTime(time.Now().Format(time.RFC3339) + "-DRYRUN")
	} else {
		exp.Status.SetStartTime(time.Now().Format(time.RFC3339))
	}

	if err := app.ApplyApps(app.ACTIONPOSTSTART, exp); err != nil {
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

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return fmt.Errorf("decoding experiment from config: %w", err)
	}

	if !exp.Running() {
		return fmt.Errorf("experiment isn't running")
	}

	dryrun := strings.HasSuffix(exp.Status.StartTime(), "-DRYRUN")

	if err := app.ApplyApps(app.ACTIONCLEANUP, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	if !dryrun {
		if err := mm.ClearNamespace(exp.Spec.ExperimentName()); err != nil {
			return fmt.Errorf("killing experiment VMs: %w", err)
		}
	}

	exp.Status.SetStartTime("")

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

	exp, err := types.DecodeExperimentFromConfig(*c)
	if err != nil {
		return false
	}

	return exp.Running()
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
	if Running(name) {
		return fmt.Errorf("cannot delete a running experiment")
	}

	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s: %w", name, err)
	}

	if err := store.Delete(c); err != nil {
		return fmt.Errorf("deleting experiment %s: %w", name, err)
	}

	return nil
}

func Files(name string) ([]string, error) {
	return file.GetExperimentFileNames(name)
}

func File(name, fileName string) ([]byte, error) {
	files, err := file.GetExperimentFileNames(name)
	if err != nil {
		return nil, fmt.Errorf("getting list of experiment files: %w", err)
	}

	for _, f := range files {
		if fileName == f {
			path := fmt.Sprintf("%s/%s/files/%s", common.PhenixBase, name, f)

			data, err := ioutil.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading contents of file: %w", err)
			}

			return data, nil
		}
	}

	return nil, fmt.Errorf("file not found")
}
