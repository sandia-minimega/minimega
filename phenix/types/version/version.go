package version

import (
	"context"
	"fmt"

	v1 "phenix/types/version/v1"

	"github.com/getkin/kin-openapi/openapi3"
)

var StoredVersion = map[string]string{
	"Topology":   "v1",
	"Scenario":   "v1",
	"Experiment": "v1",
}

func GetVersionedSpecForKind(kind, version string) (interface{}, error) {
	switch kind {
	case "Topology":
		switch version {
		case "v1":
			return new(v1.TopologySpec), nil
		default:
			return nil, fmt.Errorf("unknown version %s for %s", version, kind)
		}
	case "Scenario":
		switch version {
		case "v1":
			return new(v1.ScenarioSpec), nil
		default:
			return nil, fmt.Errorf("unknown version %s for %s", version, kind)
		}
	case "Experiment":
		switch version {
		case "v1":
			return new(v1.ExperimentSpec), nil
		default:
			return nil, fmt.Errorf("unknown version %s for %s", version, kind)
		}
	default:
		return nil, fmt.Errorf("unknown kind %s", kind)
	}
}

func GetVersionedValidatorForKind(kind, version string) (*openapi3.Schema, error) {
	switch version {
	case "v1":
		s, err := openapi3.NewSwaggerLoader().LoadSwaggerFromData(v1.OpenAPI)
		if err != nil {
			return nil, fmt.Errorf("loading OpenAPI schema for version %s: %w", version, err)
		}

		if err := s.Validate(context.Background()); err != nil {
			return nil, fmt.Errorf("validating OpenAPI schema for version %s: %w", version, err)
		}

		ref, ok := s.Components.Schemas[kind]
		if !ok {
			return nil, fmt.Errorf("no schema definition found for version %s of %s", version, kind)
		}

		return ref.Value, nil
	default:
		return nil, fmt.Errorf("unknown version %s", version)
	}
}
