package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
)

type Property struct {
	Node []Node `json:"nodes"`
	VLAN []VLAN `json:"vlans"`
}

type Node struct {
	Type       string      `json:"type"`
	General    General     `json:"general"`
	Hardware   Hardware    `json:"hardware"`
	Network    Network     `json:"network"`
	Injections []Injection `json:"injections"`
	Metadata   Metadata    `json:"metadata"`
}

type General struct {
	Hostname    string `json:"hostname"`
	Description string `json:"description"`
	Snapshot    bool   `json:"snapshot"`
	DoNotBoot   bool   `json:"do_not_boot"`
}

type Hardware struct {
	CPU    string  `json:"cpu"`
	VCPU   string  `json:"vcpus"`
	Memory string  `json:"memory"`
	OSType string  `json:"os_type"`
	Drives []Drive `json:"drives"`
}

type Drive struct {
	Image           string `json:"image"`
	Interface       string `json:"interface"`
	CacheMode       string `json:"cache_mode"`
	InjectPartition string `json:"inject_partition"`
}

type Network struct {
	Interfaces []Interface `json:"interfaces"`
	Routes     []Route     `json:"routes"`
	OSPF       OSPF        `json:"ospf"`
	Rulesets   []Ruleset   `json:"rulesets"`
}

type Interface struct {
	Name       string `json:"name"`
	Vlan       string `json:"vlan"`
	Address    string `json:"address"`
	Mask       int    `json:"mask"`
	Type       string `json:"type"`
	Proto      string `json:"proto"`
	Autostart  bool   `json:"autostart"`
	MAC        string `json:"mac"`
	MTU        int    `json:"mtu"`
	Gateway    string `json:"gateway"`
	RulesetIn  string `json:"ruleset_in"`
	RulesetOut string `json:"rulsect_out"`
}

type Route struct {
	Destination string `json:"destination"`
	Next        string `json:"next"`
	Cost        string `json:"cost"`
}

type AreaNetwork struct {
	Network string `json:"networks"`
}

type Area struct {
	AreaId       string        `json:"area_id"`
	AreaNetworks []AreaNetwork `json:"area_networks"`
}

type OSPF struct {
	RouterId string `json:"router_id"`
	Areas    []Area `json:"areas"`
}

type Ruleset struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     string `json:"default"`
	Rules       []Rule `json:"rules"`
}

type Rule struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Action      string   `json:"action"`
	Protocol    string   `json:"protocol"`
	Source      AddrPort `json:"source"`
	Destination AddrPort `json:"destination"`
}

type AddrPort struct {
	Address string `json:"address"`
	Port    string `json:"port"`
}

type Injection struct {
	Src         string `json:"src"`
	Dst         string `json:"dst"`
	Description string `json:"description"`
}

type Metadata struct {
	Infrastructure       string       `json:"infrastructure"`
	Provider             string       `json:"provider"`
	Simulator            string       `json:"simulator"`
	PublishEndpoint      string       `json:"publish_endpoint"`
	CycleTime            string       `json:"cycle_time"`
	DNP3                 []DNP3       `json:"dnp3"`
	DNP3Serial           []DNP3Serial `json:"dnp3-serial"`
	Modbus               []Modbus     `json:"modbus"`
	Logic                string       `json:"logic"`
	ConnectedRTU         []string     `json:"connected_rtus"`
	ConnectToScada       bool         `json:"connect_to_scada"`
	ManualRegisterConfig string       `json:"manual_register_config"`
}

type DNP3 struct {
	Type            string   `json:"type"`
	Name            string   `json:"name"`
	AnalogRead      []string `json:"analog_read"`
	BinaryRead      []string `json:"binary_read"`
	BinaryReadWrite []string `json:"binary_read_write"`
}

type DNP3Serial struct {
	Type            string            `json:"type"`
	Name            string            `json:"name"`
	AnalogRead      []AnalogRead      `json:"analog_read"`
	BinaryRead      []BinaryRead      `json:"binary_read"`
	BinaryReadWrite []BinaryReadWrite `json:"binary_read_write"`
}

type Modbus struct {
	Type            string            `json:"type"`
	Name            string            `json:"name"`
	AnalogRead      []AnalogRead      `json:"analog_read"`
	BinaryRead      []BinaryRead      `json:"binary_read"`
	BinaryReadWrite []BinaryReadWrite `json:"binary_read_write"`
}

type AnalogRead struct {
	Field          string `json:"field"`
	RegisterNumber int    `json:"register_number"`
	RegisterType   string `json:"register_number"`
}

type BinaryRead struct {
	Field          string `json:"field"`
	RegisterNumber int    `json:"register_number"`
	RegisterType   string `json:"register_number"`
}

type BinaryReadWrite struct {
	Field          string `json:"field"`
	RegisterNumber int    `json:"register_number"`
	RegisterType   string `json:"register_number"`
}

type VLAN struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

type Definition struct {
	Iface        []Iface        `json:"iface"`
	IfaceAddress []IfaceAddress `json:"iface_address"`
	IfaceRuleset []IfaceRuleset `json:"iface_rulesets"`
	StaticIface  []DefIface     `json:"static_iface"`
	DHCPIface    []DefIface     `json:"dhcp_iface"`
	SerialIface  []DefIface     `json:"serial_iface"`
}

type Iface struct {
	Name      string `json:"name"`
	VLAN      string `json:"vlan"`
	Autostart bool   `json:"autostart"`
	MAC       string `json:"mac"`
	MTU       int    `json:"mtu"`
}

type IfaceAddress struct {
	Addres  string `json:"address"`
	Mask    int    `json:"mask"`
	Gateway string `json:"gateway"`
}

type IfaceRuleset struct {
	RulesetOut string `json:"ruleset_out"`
	RulesetIn  string `json:"ruleset_in"`
}

type DefIface struct {
	Type     string `json:"type"`
	Proto    string `json:"proto"`
	UDPPort  int    `json:"udp_port"`
	BaudRate int    `json:"baud_rate"`
	Device   string `json:"device"`
}

func main() {

	file, err := ioutil.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println("Cannot read file:", os.Args[1], "-- Error:", err)
		return
	}

	var prop Property

	err = json.Unmarshal(file, &prop)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println(prop)
}
