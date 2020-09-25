package scenario

import (
	"fmt"

	"phenix/store"
	"phenix/types"
	v1 "phenix/types/version/v1"

	"github.com/mitchellh/mapstructure"
)

// AppList returns a slice of unique app names that are used in the given
// scenario.
func AppList(name string) ([]string, error) {
	if name == "" {
		return nil, fmt.Errorf("no scenario name provided")
	}

	c, _ := types.NewConfig("scenario/" + name)

	if err := store.Get(c); err != nil {
		return nil, fmt.Errorf("getting scenario %s from store: %w", name, err)
	}

	spec := new(v1.ScenarioSpec)

	if err := mapstructure.Decode(c.Spec, spec); err != nil {
		return nil, fmt.Errorf("decoding scenario spec: %w", err)
	}

	var apps []string

	if spec.Apps == nil {
		return nil, nil
	}

	for _, e := range spec.Apps.Experiment {
		apps = append(apps, e.Name)
	}

	for _, h := range spec.Apps.Host {
		apps = append(apps, h.Name)
	}

	return apps, nil
}
