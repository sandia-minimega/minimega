package experiment

import (
	"fmt"
	"os"
	"phenix/store"
	"phenix/tmpl"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/mitchellh/mapstructure"
)

func CreateConfigSpec(config *types.Config) error {
	topoName, ok := config.Metadata.Annotations["topology"]
	if !ok {
		panic("topology annotation missing")
	}

	scenarioName := config.Metadata.Annotations["scenario"]

	topo := types.NewConfig("Topology", topoName)

	if err := store.Get(topo); err != nil {
		panic("topology doesn't exist")
	}

	config.Spec = map[string]interface{}{
		"experimentName": config.Metadata.Name,
		"topology":       topo.Spec,
	}
	if scenarioName != "" {
		scenario := types.NewConfig("Scenario", scenarioName)

		if err := store.Get(scenario); err != nil {
			panic("scenario doesn't exist")
		}

		topo, ok := scenario.Spec["topology"].(string)
		if !ok || topo != topoName {
			panic("experiment/scenario topology mismatch")
		}

		config.Spec["scenario"] = scenario.Spec
	}

	return nil
}

func Start(name string) error {
	c := types.NewConfig("Experiment", name)

	if err := store.Get(c); err != nil {
		return fmt.Errorf("getting experiment %s from store: %w", name, err)
	}

	var exp v1.ExperimentSpec

	if err := mapstructure.Decode(c.Spec, &exp); err != nil {
		return fmt.Errorf("decoding experiment spec: %w", err)
	}

	exp.SetDefaults()

	if err := exp.VerifyScenario(); err != nil {
		return fmt.Errorf("verifying experiment scenario: %w", err)
	}

	if err := tmpl.GenerateFromTemplate("minimega_script.tmpl", exp, os.Stdout); err != nil {
		panic(err)
	}

	return nil
}
