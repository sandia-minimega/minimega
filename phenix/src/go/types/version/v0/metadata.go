package v0

type Metadata struct {
	Infrastructure       string       `json:"infrastructure" yaml:"infrastructure"`
	Provider             string       `json:"provider" yaml:"provider"`
	Simulator            string       `json:"simulator" yaml:"simulator"`
	PublishEndpoint      string       `json:"publish_endpoint" yaml:"publish_endpoint" mapstructure:"publish_endpoint" structs:"publish_endpoint"`
	CycleTime            string       `json:"cycle_time" yaml:"cycle_time"`
	DNP3                 []DNP3       `json:"dnp3" yaml:"dnp3"`
	DNP3Serial           []DNP3Serial `json:"dnp3-serial" yaml:"dnp3-serial" mapstructure:"dnp3-serial" structs:"dnp3-serial"`
	Modbus               []Modbus     `json:"modbus" yaml:"modbus"`
	Logic                string       `json:"logic" yaml:"logic"`
	ConnectedRTU         []string     `json:"connected_rtus" yaml:"connected_rtus" mapstructure:"connected_rtus" structs:"connected_rtus"`
	ConnectToSCADA       bool         `json:"connect_to_scada" yaml:"connect_to_scada" mapstructure:"connect_to_scada" structs:"connect_to_scada"`
	ManualRegisterConfig string       `json:"manual_register_config" yaml:"manual_register_config" mapstructure:"manual_register_config" structs:"manual_register_config"`

	DomainController DomainController `json:"domain_controller" yaml:"domain_controller" mapstructure:"domain_controller" structs:"domain_controller"`
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
	RegisterType   string `json:"register_type" yaml:"register_type"`
}

type DomainController struct {
	IP       string `json:"ip" yaml:"ip" mapstructure:"ip" structs:"ip"`
	Domain   string `json:"domain" yaml:"domain" mapstructure:"domain" structs:"domain"`
	Username string `json:"username" yaml:"username" mapstructure:"username" structs:"username"`
	Password string `json:"password" yaml:"password" mapstructure:"password" structs:"password"`
}
