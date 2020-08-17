package type

type Hosts struct {
	Name string `json:"name"`
	CPUs int `json:"cpus"`
	Load []int `json:"load"`
	MemUsed int `json:"mem_used"`
	Bandwidth int `json:"bandwidth"`
	NoVMs int `json:"no_vms"`
	Uptime string `json:"uptime"`
}
