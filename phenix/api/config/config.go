package config

import (
	"errors"
	"fmt"

	"phenix/api/experiment"
	"phenix/store"
	"phenix/types"
	v1 "phenix/types/version/v1"
	"phenix/util/editor"

	"github.com/mitchellh/mapstructure"
	"gopkg.in/yaml.v3"
)

func List(which string) (types.Configs, error) {
	var (
		configs types.Configs
		err     error
	)

	switch which {
	case "", "all":
		configs, err = store.List("Topology", "Scenario", "Experiment")
	case "topology":
		configs, err = store.List("Topology")
	case "scenario":
		configs, err = store.List("Scenario")
	case "experiment":
		configs, err = store.List("Experiment")
	default:
		return nil, fmt.Errorf("unknown config kind provided")
	}

	if err != nil {
		return nil, fmt.Errorf("getting list of configs from store: %w", err)
	}

	return configs, nil
}

func Get(name string) (*types.Config, error) {
	c, err := types.NewConfig(name)
	if err != nil {
		return nil, err
	}

	if err := store.Get(c); err != nil {
		return nil, fmt.Errorf("getting config from store: %w", err)
	}

	return c, nil
}

func Create(path string) (*types.Config, error) {
	if path == "" {
		return nil, fmt.Errorf("no config file provided")
	}

	c, err := types.NewConfigFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("creating new config from file: %w", err)
	}

	if c.Kind == "Experiment" && c.Spec == nil {
		if err := experiment.CreateFromConfig(c); err != nil {
			return nil, fmt.Errorf("creating experiment config spec: %w", err)
		}
	}

	if err := types.ValidateConfigSpec(*c); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	if err := store.Create(c); err != nil {
		return nil, fmt.Errorf("storing config: %w", err)
	}

	return c, nil
}

func Edit(name string) (*types.Config, error) {
	c, err := types.NewConfig(name)
	if err != nil {
		return nil, err
	}

	if err := store.Get(c); err != nil {
		return nil, fmt.Errorf("getting config from store: %w", err)
	}

	if c.Kind == "Experiment" {
		var status v1.ExperimentStatus

		if err := mapstructure.Decode(c.Status, &status); err != nil {
			return nil, fmt.Errorf("decoding experiment status: %w", err)
		}

		if status.Running() {
			return nil, fmt.Errorf("cannot edit running experiment")
		}
	}

	body, err := yaml.Marshal(c.Spec)
	if err != nil {
		return nil, fmt.Errorf("marshaling config to YAML: %w", err)
	}

	body, err = editor.EditData(body)
	if err != nil {
		return nil, fmt.Errorf("editing config: %w", err)
	}

	var spec map[string]interface{}

	if err := yaml.Unmarshal(body, &spec); err != nil {
		return nil, fmt.Errorf("unmarshaling config as YAML: %w", err)
	}

	c.Spec = spec

	if err := store.Update(c); err != nil {
		return nil, fmt.Errorf("updating config in store: %w", err)
	}

	return c, nil
}

func Delete(name string) error {
	if name == "all" {
		configs, _ := List("all")

		for _, c := range configs {
			if err := delete(&c); err != nil {
				return err
			}
		}

		return nil
	}

	c, err := Get(name)
	if err != nil {
		return fmt.Errorf("getting config '%s': %w", name, err)
	}

	return delete(c)
}

func delete(c *types.Config) error {
	if err := store.Delete(c); err != nil {
		return fmt.Errorf("deleting config in store: %w", err)
	}

	return nil
}

func IsConfigNotModified(err error) bool {
	return errors.Is(err, editor.ErrNoChange)
}
