package version

import (
	"fmt"

	v1 "phenix/types/version/v1"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/spec"
	"github.com/go-openapi/strfmt"
	"github.com/go-openapi/validate"
)

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

func GetVersionedValidatorForKind(kind, version string) (*spec.Schema, error) {
	switch version {
	case "v1":
		schema, err := loads.Analyzed(v1.OpenAPI, "")
		if err != nil {
			return nil, fmt.Errorf("loading OpenAPI schema for version %s: %w", version, err)
		}

		if schema, err = schema.Expanded(); err != nil {
			return nil, fmt.Errorf("expanding OpenAPI schema for version %s: %w", version, err)
		}

		if err := validate.Spec(schema, strfmt.Default); err != nil {
			return nil, fmt.Errorf("validating OpenAPI schema for version %s: %w", version, err)
		}

		for _, d := range schema.Analyzer.AllDefinitions() {
			if d.Name == kind {
				return d.Schema, nil
			}
		}

		return nil, fmt.Errorf("no schema definition found for version %s of %s", version, kind)
	default:
		return nil, fmt.Errorf("unknown version %s", version)
	}
}
