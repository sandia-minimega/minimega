package type

type VM struct {
	Name string `json:"name"`
	Host string `json:"host"`
	IPv4 string `json:"ipv4"`
	CPUs int `json:"cpus"`
	RAM int `json:"ram"`
	Disk string `json:"disk"`
	Uptime    string `json:"uptime"`
	Networks  []string `json:"networks"`
	Taps      []string `json:"taps"`
	Snapshots []string `json:"snapshots"`
	DoNotBoot bool `json:"dnb"`
}