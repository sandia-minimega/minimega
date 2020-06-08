package v1

type TopologySpec struct {
	Nodes []*Node `json:"nodes" yaml:"nodes"`
}

func (this *TopologySpec) SetDefaults() {
	for _, n := range this.Nodes {
		n.SetDefaults()
	}
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
