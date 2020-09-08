package upgrade

import (
	"fmt"
	"path/filepath"

	"phenix/types"
	v0 "phenix/types/version/v0"
	v1 "phenix/types/version/v1"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

func init() {
	RegisterUpgrader("topology/v1", new(topology))
}

type topology struct{}

func (topology) Upgrade(version string, spec map[string]interface{}, md types.ConfigMetadata) ([]interface{}, error) {
	// This is a fairly simple upgrade path. The only difference between v0 and v1
	// is the lack of topology metadata in v1. Key names and such stayed the same.
	// The idea here is to decode the spec into both v0 and v1 (v1 should ignore
	// v0 metadata stuff) and use v0 metadata to create a v1 scenario (assuming
	// all v0 metadata is associated w/ the SCEPTRE app).

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

		app := v1.HostApp{Name: "sceptre"}

		for _, node := range topoV0.Nodes {
			if node.Metadata != nil {
				host := v1.Host{
					Hostname: node.General.Hostname,
					Metadata: structs.MapWithOptions(node.Metadata, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty()),
				}

				app.Hosts = append(app.Hosts, host)
			}
		}

		if len(app.Hosts) > 0 {
			scenario := v1.ScenarioSpec{
				Apps: &v1.Apps{Host: []v1.HostApp{app}},
			}

			results = append(results, scenario)
		}

		return results, nil
	}

	return nil, fmt.Errorf("unknown version %s to upgrade from", version)
}
