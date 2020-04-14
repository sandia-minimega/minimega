package v1

type TopologySpec struct {
	Nodes []Node `json:"nodes" yaml:"nodes"`
	VLANs []VLAN `json:"vlans,omitempty" yaml:"vlans,omitempty"`
}
