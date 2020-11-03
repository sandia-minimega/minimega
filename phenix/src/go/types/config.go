package types

import (
	"fmt"

	"phenix/store"
	v1 "phenix/types/version/v1"
	v2 "phenix/types/version/v2"

	"github.com/activeshadow/structs"
)

func NewConfigFromSpec(name string, spec interface{}) (*store.Config, error) {
	// TODO: add more case statements to this as more upgraders are added.
	switch spec := spec.(type) {
	case store.Config:
		return &spec, nil
	case *store.Config:
		return spec, nil
	case v1.TopologySpec, *v1.TopologySpec:
		c, err := store.NewConfig("topology/" + name)
		if err != nil {
			return nil, fmt.Errorf("creating new v1 scenario config: %w", err)
		}

		c.Version = "phenix.sandia.gov/v1"
		c.Spec = structs.MapWithOptions(spec, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty())

		return c, nil
	case v1.ScenarioSpec, *v1.ScenarioSpec:
		c, err := store.NewConfig("scenario/" + name)
		if err != nil {
			return nil, fmt.Errorf("creating new v1 scenario config: %w", err)
		}

		c.Version = "phenix.sandia.gov/v1"
		c.Spec = structs.MapWithOptions(spec, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty())

		return c, nil
	case v2.ScenarioSpec, *v2.ScenarioSpec:
		c, err := store.NewConfig("scenario/" + name)
		if err != nil {
			return nil, fmt.Errorf("creating new v2 scenario config: %w", err)
		}

		c.Version = "phenix.sandia.gov/v2"
		c.Spec = structs.MapWithOptions(spec, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty())

		return c, nil
	}

	return nil, fmt.Errorf("unknown spec provided")
}
