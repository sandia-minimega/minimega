package v0

type TopologySpec struct {
	Nodes []Node `json:"nodes" yaml:"nodes"`
	VLANs []VLAN `json:"vlans" yaml:"vlans"`
}

type Node struct {
	Type       string      `json:"type" yaml:"type`
	General    General     `json:"general" yaml:"general"`
	Hardware   Hardware    `json:"hardware" yaml:"hardware"`
	Network    Network     `json:"network" yaml:"network"`
	Injections []Injection `json:"injections" yaml:"injections"`
	Metadata   Metadata    `json:"metadata" yaml:"metadata"`
}

type General struct {
	Hostname    string `json:"hostname" yaml:"hostname"`
	Description string `json:"description" yaml:"description"`
	Snapshot    bool   `json:"snapshot" yaml:"snapshot"`
	DoNotBoot   bool   `json:"do_not_boot" yaml:"do_not_boot"`
}

type Hardware struct {
	CPU    string  `json:"cpu" yaml:"cpu"`
	VCPU   string  `json:"vcpus" yaml:"vcpus"`
	Memory string  `json:"memory" yaml:"memory"`
	OSType string  `json:"os_type" yaml:"os_type"`
	Drives []Drive `json:"drives" yaml:"drives"`
}

type Drive struct {
	Image           string `json:"image" yaml:"image"`
	Interface       string `json:"interface" yaml:"interface"`
	CacheMode       string `json:"cache_mode" yaml:"cache_mode"`
	InjectPartition string `json:"inject_partition" yaml:"inject_partition"`
}

type Network struct {
	Interfaces []Interface `json:"interfaces" yaml:"interfaces"`
	Routes     []Route     `json:"routes" yaml:"routes"`
	OSPF       OSPF        `json:"ospf" yaml:"ospf"`
	Rulesets   []Ruleset   `json:"rulesets" yaml:"rulesets"`
}

type Interface struct {
	Name       string `json:"name" yaml:"name"`
	Vlan       string `json:"vlan" yaml:"vlan"`
	Address    string `json:"address" yaml:"address"`
	Mask       int    `json:"mask" yaml:"mask"`
	Type       string `json:"type" yaml:"type"`
	Proto      string `json:"proto" yaml:"proto"`
	Autostart  bool   `json:"autostart" yaml:"autostart"`
	MAC        string `json:"mac" yaml:"mac"`
	MTU        int    `json:"mtu" yaml:"mtu"`
	Gateway    string `json:"gateway" yaml:"gateway"`
	RulesetIn  string `json:"ruleset_in" yaml:"ruleset_in"`
	RulesetOut string `json:"rulsect_out" yaml:"rulsect_out"`
}

type Route struct {
	Destination string `json:"destination" yaml:"destination"`
	Next        string `json:"next" yaml:"next"`
	Cost        string `json:"cost" yaml:"cost"`
}

type AreaNetwork struct {
	Network string `json:"networks" yaml:"networks"`
}

type Area struct {
	AreaId       string        `json:"area_id" yaml:"area_id"`
	AreaNetworks []AreaNetwork `json:"area_networks" yaml:"area_networks"`
}

type OSPF struct {
	RouterId string `json:"router_id" yaml:"router_id"`
	Areas    []Area `json:"areas" yaml:"areas"`
}

type Ruleset struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Default     string `json:"default" yaml:"default"`
	Rules       []Rule `json:"rules" yaml:"rules"`
}

type Rule struct {
	ID          string   `json:"id" yaml:"id"`
	Description string   `json:"description" yaml:"description"`
	Action      string   `json:"action" yaml:"action"`
	Protocol    string   `json:"protocol" yaml:"protocol"`
	Source      AddrPort `json:"source" yaml:"source"`
	Destination AddrPort `json:"destination" yaml:"destination"`
}

type AddrPort struct {
	Address string `json:"address" yaml:"address"`
	Port    string `json:"port" yaml:"port"`
}

type Injection struct {
	Src         string `json:"src" yaml:"src"`
	Dst         string `json:"dst" yaml:"dst"`
	Description string `json:"description" yaml:"description"`
}

type Metadata struct {
	Infrastructure       string       `json:"infrastructure" yaml:"infrastructure"`
	Provider             string       `json:"provider" yaml:"provider"`
	Simulator            string       `json:"simulator" yaml:"simulator"`
	PublishEndpoint      string       `json:"publish_endpoint" yaml:"publish_endpoint"`
	CycleTime            string       `json:"cycle_time" yaml:"cycle_time"`
	DNP3                 []DNP3       `json:"dnp3" yaml:"dnp3"`
	DNP3Serial           []DNP3Serial `json:"dnp3-serial" yaml:"dnp3-serial"`
	Modbus               []Modbus     `json:"modbus" yaml:"modbus"`
	Logic                string       `json:"logic" yaml:"logic"`
	ConnectedRTU         []string     `json:"connected_rtus" yaml:"connected_rtus"`
	ConnectToScada       bool         `json:"connect_to_scada" yaml:"connect_to_scada"`
	ManualRegisterConfig string       `json:"manual_register_config" yaml:"manual_register_config"`
}

type DNP3 struct {
	Type            string   `json:"type" yaml:"type"`
	Name            string   `json:"name" yaml:"name"`
	AnalogRead      []string `json:"analog_read" yaml:"analog_read"`
	BinaryRead      []string `json:"binary_read" yaml:"binary_read"`
	BinaryReadWrite []string `json:"binary_read_write" yaml:"binary_read_write"`
}

type DNP3Serial struct {
	Type            string      `json:"type" yaml:"type"`
	Name            string      `json:"name" yaml:"name"`
	AnalogRead      []ReadWrite `json:"analog_read" yaml:"analog_read"`
	BinaryRead      []ReadWrite `json:"binary_read" yaml:"binary_read"`
	BinaryReadWrite []ReadWrite `json:"binary_read_write" yaml:"binary_read_write"`
}

type Modbus struct {
	Type            string      `json:"type" yaml:"type"`
	Name            string      `json:"name" yaml:"name"`
	AnalogRead      []ReadWrite `json:"analog_read" yaml:"analog_read"`
	BinaryRead      []ReadWrite `json:"binary_read" yaml:"binary_read"`
	BinaryReadWrite []ReadWrite `json:"binary_read_write" yaml:"binary_read_write"`
}

type ReadWrite struct {
	Field          string `json:"field" yaml:"field"`
	RegisterNumber int    `json:"register_number" yaml:"register_number"`
	RegisterType   string `json:"register_number" yaml:"register_number"`
}

type VLAN struct {
	Name string `json:"name" yaml:"name"`
	ID   string `json:"id" yaml:"id"`
}

type Definition struct {
	Ifaces         []Iface        `json:"iface" yaml:"iface"`
	IfaceAddresses []IfaceAddress `json:"iface_address" yaml:"iface_address"`
	IfaceRulesets  []IfaceRuleset `json:"iface_rulesets" yaml:"iface_rulesets"`
	StaticIfaces   []DefIface     `json:"static_iface" yaml:"static_iface"`
	DHCPIfaces     []DefIface     `json:"dhcp_iface" yaml:"dhcp_iface"`
	SerialIfaces   []DefIface     `json:"serial_iface" yaml:"serial_iface"`
}

type Iface struct {
	Name      string `json:"name" yaml:"name"`
	VLAN      string `json:"vlan" yaml:"vlan"`
	Autostart bool   `json:"autostart" yaml:"autostart"`
	MAC       string `json:"mac" yaml:"mac"`
	MTU       int    `json:"mtu" yaml:"mtu"`
}

type IfaceAddress struct {
	Addres  string `json:"address" yaml:"address"`
	Mask    int    `json:"mask" yaml:"mask"`
	Gateway string `json:"gateway" yaml:"gateway"`
}

type IfaceRuleset struct {
	RulesetOut string `json:"ruleset_out" yaml:"ruleset_out"`
	RulesetIn  string `json:"ruleset_in" yaml:"ruleset_in"`
}

type DefIface struct {
	Type     string `json:"type" yaml:"type"`
	Proto    string `json:"proto" yaml:"proto"`
	UDPPort  int    `json:"udp_port" yaml:"udp_port"`
	BaudRate int    `json:"baud_rate" yaml:"baud_rate"`
	Device   string `json:"device" yaml:"device"`
}
