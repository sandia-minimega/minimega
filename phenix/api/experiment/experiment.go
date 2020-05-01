package experiment

import (
	"fmt"
	"time"

	"phenix/app"
	"phenix/store"
	"phenix/tmpl"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

func Create(c *types.Config) error {
	topoName, ok := c.Metadata.Annotations["topology"]
	if !ok {
		panic("topology annotation missing")
	}

	scenarioName := c.Metadata.Annotations["scenario"]

	topo, _ := types.NewConfig("topology/" + topoName)

	if err := store.Get(topo); err != nil {
		panic("topology doesn't exist")
	}

	c.Spec = map[string]interface{}{
		"experimentName": c.Metadata.Name,
		"topology":       topo.Spec,
	}

	if scenarioName != "" {
		scenario, _ := types.NewConfig("scenario/" + scenarioName)

		if err := store.Get(scenario); err != nil {
			panic("scenario doesn't exist")
		}

		topo, ok := scenario.Metadata.Annotations["topology"]
		if !ok {
			panic("topology annotation missing")
		}

		if topo != topoName {
			panic("experiment/scenario topology mismatch")
		}

		c.Spec["scenario"] = scenario.Spec
	}

	exp := new(v1.ExperimentSpec)

	if err := mapstructure.Decode(c.Spec, exp); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	exp.SetDefaults()

	if err := exp.VerifyScenario(); err != nil {
		return fmt.Errorf("verifying experiment scenario: %w", err)
	}

	if err := app.ApplyApps(app.ACTIONCONFIG, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp, structs.CASESNAKE)

	return nil
}

func Start(name string) error {
	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", name, err)
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

	if err := app.ApplyApps(app.ACTIONSTART, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	filename := fmt.Sprintf("%s/mm_files/%s.mm", exp.BaseDir, exp.ExperimentName)

	if err := tmpl.CreateFileFromTemplate("minimega_script.tmpl", exp, filename); err != nil {
		return fmt.Errorf("generating minimega script: %w", err)
	}

	c.Status = map[string]interface{}{"startTime": time.Now().Format(time.RFC3339)}

	if err := app.ApplyApps(app.ACTIONPOSTSTART, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp, structs.CASESNAKE)

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}

func Stop(name string) error {
	c, _ := types.NewConfig("experiment/" + name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	exp := new(v1.ExperimentSpec)

	if err := mapstructure.Decode(c.Spec, exp); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	if err := app.ApplyApps(app.ACTIONCLEANUP, exp); err != nil {
		return fmt.Errorf("applying apps to experiment: %w", err)
	}

	c.Spec = structs.MapDefaultCase(exp, structs.CASESNAKE)
	c.Status = nil

	if err := store.Update(c); err != nil {
		return fmt.Errorf("updating experiment config: %w", err)
	}

	return nil
}
