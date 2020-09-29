package upgrade

import (
	"fmt"
	"path/filepath"

	"phenix/types"
	v0 "phenix/types/version/v0"
	v1 "phenix/types/version/v1"

	"github.com/mitchellh/mapstructure"
)

func init() {
	RegisterUpgrader("topology/v1", new(topology))
}

type topology struct{}

func (topology) Upgrade(version string, spec map[string]interface{}, md types.ConfigMetadata) ([]interface{}, error) {
	// This is a dummy topology upgrader to provide an exmaple of how an upgrader
	// might be coded up. The specs in v0 simply assume that some integer values
	// might be represented as strings when in JSON format.

	if version == "v0" {
		var (
			topoV0 v0.TopologySpec
			topoV1 v1.TopologySpec
		)

		// Using WeakDecode here since v0 schema uses strings for some integer
		// values.
		if err := mapstructure.WeakDecode(spec, &topoV0); err != nil {
			return nil, fmt.Errorf("decoding topology into v0 spec: %w", err)
		}

		// Using WeakDecode here since v0 schema uses strings for some integer
		// values.
		if err := mapstructure.WeakDecode(spec, &topoV1); err != nil {
			return nil, fmt.Errorf("decoding topology into v1 spec: %w", err)
		}

		// Previous versions of phenix assumed topologies were stored at
		// /phenix/topologies/<name>, and typically configured injections to use an
		// injections subdirectory. Given this, if an injection source path isn't
		// absolute then assume injections are based in the old topologies
		// directory.
		for _, n := range topoV1.Nodes {
			for _, i := range n.Injections {
				if !filepath.IsAbs(i.Src) {
					i.Src = fmt.Sprintf("/phenix/topologies/%s/%s", md.Name, i.Src)
				}
			}
		}

		results := []interface{}{topoV1}

		return results, nil
	}

	return nil, fmt.Errorf("unknown version %s to upgrade from", version)
}
