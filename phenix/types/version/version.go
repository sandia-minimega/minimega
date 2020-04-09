package version

import (
	"fmt"

	v0 "phenix/types/version/v0"
)

func GetVersionForKind(kind, version string) (interface{}, error) {
	switch kind {
	case "Topology":
		switch version {
		case "v0":
			return new(v0.TopologySpec), nil
		default:
			return nil, fmt.Errorf("unknown version %s for %s", version, kind)
		}
	case "Scenario":
		return nil, fmt.Errorf("unknown version %s for %s", version, kind)
	case "Experiment":
		switch version {
		case "v0":
			return new(v0.ExperimentSpec), nil
		default:
			return nil, fmt.Errorf("unknown version %s for %s", version, kind)
		}
	default:
		return nil, fmt.Errorf("unknown kind %s", kind)
	}
}
