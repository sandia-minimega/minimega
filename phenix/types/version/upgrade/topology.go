package upgrade

import (
	"fmt"
	"path/filepath"
	"strings"

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

		nodeTypes := []string{
			"client",
			"data-concentrator",
			"elk",
			"engineer-workstation",
			"fep",
			"historian",
			"hmi",
			"opc",
			"provider",
			"scada-server",
		}

		elkNodes := []string{
			"fep",
			"plc",
			"provier",
			"relay",
			"rtu",
		}

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
				md := structs.MapWithOptions(node.Metadata, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty())

				// Metadata might be empty if v0 topology contained metadata keys not
				// recognized by v0.Metadata struct.
				if len(md) > 0 {
					// Default type if no other node types defined above match.
					md["type"] = "field-device"
					var labels []string

					for _, t := range nodeTypes {
						if strings.Contains(node.General.Hostname, t) {
							md["type"] = t
							break
						}
					}

					for _, e := range elkNodes {
						if strings.Contains(node.General.Hostname, e) {
							labels = append(labels, "elk")
							break
						}
					}

					if strings.Contains(node.General.Hostname, "ignition") {
						labels = append(labels, "ignition")
					}

					if labels != nil {
						md["labels"] = labels
					}

					host := v1.Host{
						Hostname: node.General.Hostname,
						Metadata: md,
					}

					app.Hosts = append(app.Hosts, host)
				}
			}
		}

		if len(app.Hosts) > 0 {
			scenario := v1.ScenarioSpec{
				Apps: &v1.Apps{Host: []v1.HostApp{app}},
			}

			config, _ := types.NewConfig("scenario/" + md.Name)
			config.Version = "phenix.sandia.gov/v1"
			config.Metadata.Annotations = map[string]string{"topology": "mosaics"}
			config.Spec = structs.MapWithOptions(scenario, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty())

			results = append(results, config)
		}

		return results, nil
	}

	return nil, fmt.Errorf("unknown version %s to upgrade from", version)
}
