package v1

import (
	"fmt"
	"phenix/types/upgrade"
	v0 "phenix/types/version/v0"

	"github.com/activeshadow/structs"
	"github.com/mitchellh/mapstructure"
)

type TopologySpec struct {
	Nodes []*Node `json:"nodes" yaml:"nodes"`
}

func (this *TopologySpec) SetDefaults() {
	for _, n := range this.Nodes {
		n.SetDefaults()
	}
}

func (this TopologySpec) FindNodeByName(name string) *Node {
	for _, node := range this.Nodes {
		if node.General.Hostname == name {
			return node
		}
	}

	return nil
}

// FindNodesWithLabels finds all nodes in the topology containing at least one
// of the labels provided. Take note that the node does not have to have all the
// labels provided, just one.
func (this TopologySpec) FindNodesWithLabels(labels ...string) []*Node {
	var nodes []*Node

	for _, n := range this.Nodes {
		for _, l := range labels {
			if _, ok := n.Labels[l]; ok {
				nodes = append(nodes, n)
				break
			}
		}
	}

	return nodes
}

func (TopologySpec) Upgrade(version string, spec map[string]interface{}) ([]interface{}, error) {
	// This is a fairly simple upgrade path. The only difference between v0 and v1
	// is the lack of topology metadata in v1. Key names and such stayed the same.
	// The idea here is to decode the spec into both v0 and v1 (v1 should ignore
	// v0 metadata stuff) and use v0 metadata to create a v1 scenario (assuming
	// all v0 metadata is associated w/ the SCEPTRE app).

	if version == "v0" {
		var (
			topoV0 v0.TopologySpec
			topoV1 TopologySpec
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

		results := []interface{}{topoV1}

		app := HostApp{Name: "sceptre"}

		for _, node := range topoV0.Nodes {
			if node.Metadata != nil {
				host := Host{
					Hostname: node.General.Hostname,
					Metadata: structs.MapWithOptions(node.Metadata, structs.DefaultCase(structs.CASE_SNAKE), structs.DefaultOmitEmpty()),
				}

				app.Hosts = append(app.Hosts, host)
			}
		}

		if len(app.Hosts) > 0 {
			scenario := ScenarioSpec{
				Apps: &Apps{Host: []HostApp{app}},
			}

			results = append(results, scenario)
		}

		return results, nil
	}

	return nil, fmt.Errorf("unknown version %s to upgrade from", version)
}

func init() {
	upgrade.RegisterUpgrader("topology/v1", new(TopologySpec))
}
