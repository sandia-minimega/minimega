package v0

type Network struct {
	Interfaces []Interface `json:"interfaces" yaml:"interfaces"`
	Routes     []Route     `json:"routes" yaml:"routes"`
	OSPF       *OSPF       `json:"ospf" yaml:"ospf" mapstructure:"ospf"`
	Rulesets   []Ruleset   `json:"rulesets" yaml:"rulesets"`
}

type Interface struct {
	Name       string `json:"name" yaml:"name"`
	Type       string `json:"type" yaml:"type"`
	Proto      string `json:"proto" yaml:"proto"`
	UDPPort    int    `json:"udp_port" yaml:"udp_port" mapstructure:"udp_port"`
	BaudRate   int    `json:"baud_rate" yaml:"baud_rate" mapstructure:"baud_rate"`
	Device     string `json:"device" yaml:"device"`
	VLAN       string `json:"vlan" yaml:"vlan"`
	Autostart  bool   `json:"autostart" yaml:"autostart"`
	MAC        string `json:"mac" yaml:"mac"`
	MTU        int    `json:"mtu" yaml:"mtu"`
	Address    string `json:"address" yaml:"address"`
	Mask       int    `json:"mask" yaml:"mask"`
	Gateway    string `json:"gateway" yaml:"gateway"`
	RulesetIn  string `json:"ruleset_in" yaml:"ruleset_in" mapstructure:"ruleset_in"`
	RulesetOut string `json:"ruleset_out" yaml:"ruleset_out" mapstructure:"ruleset_out"`
}

type Route struct {
	Destination string `json:"destination" yaml:"destination"`
	Next        string `json:"next" yaml:"next"`
	Cost        int    `json:"cost" yaml:"cost"`
}

type OSPF struct {
	RouterID               string `json:"router_id" yaml:"router_id" mapstructure:"router_id"`
	Areas                  []Area `json:"areas" yaml:"areas" mapstructure:"areas"`
	DeadInterval           int    `json:"dead_interval" yaml:"dead_interval" mapstructure:"dead_interval"`
	HelloInterval          int    `json:"hello_interval" yaml:"hello_interval" mapstructure:"hello_interval"`
	RetransmissionInterval int    `json:"retransmission_interval" yaml:"retransmission_interval" mapstructure:"retransmission_interval"`
}

type Area struct {
	AreaID       int           `json:"area_id" yaml:"area_id" mapstructure:"area_id"`
	AreaNetworks []AreaNetwork `json:"area_networks" yaml:"area_networks" mapstructure:"area_networks"`
}

type AreaNetwork struct {
	Network string `json:"network" yaml:"network" mapstructure:"network"`
}

type Ruleset struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Default     string `json:"default" yaml:"default"`
	Rules       []Rule `json:"rules" yaml:"rules"`
}

type Rule struct {
	ID          int       `json:"id" yaml:"id"`
	Description string    `json:"description" yaml:"description"`
	Action      string    `json:"action" yaml:"action"`
	Protocol    string    `json:"protocol" yaml:"protocol"`
	Source      *AddrPort `json:"source" yaml:"source"`
	Destination *AddrPort `json:"destination" yaml:"destination"`
}

type AddrPort struct {
	Address string `json:"address" yaml:"address"`
	Port    int    `json:"port" yaml:"port"`
}
