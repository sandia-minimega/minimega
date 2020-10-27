package scenario

import (
	"fmt"

	"phenix/store"
	"phenix/types"
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

	spec, err := types.DecodeScenarioFromConfig(*c)
	if err != nil {
		return nil, fmt.Errorf("decoding scenario from config: %w", err)
	}

	var apps []string

	for _, app := range spec.Apps() {
		apps = append(apps, app.Name())
	}

	return apps, nil
}
