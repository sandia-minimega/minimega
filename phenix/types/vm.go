package types

import (
	"sort"
	"strings"
)

type VMs []VM

func (this VMs) SortByName(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return strings.ToLower(this[i].Name) < strings.ToLower(this[j].Name)
		}

		return strings.ToLower(this[i].Name) > strings.ToLower(this[j].Name)
	})
}

func (this VMs) SortByHost(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return strings.ToLower(this[i].Host) < strings.ToLower(this[j].Host)
		}

		return strings.ToLower(this[i].Host) > strings.ToLower(this[j].Host)
	})
}

func (this VMs) SortByUptime(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return this[i].Uptime < this[j].Uptime
		}

		return this[i].Uptime > this[j].Uptime
	})
}

func (this VMs) SortBy(col string, asc bool) {
	switch col {
	case "name":
		this.SortByName(asc)
	case "host":
		this.SortByHost(asc)
	case "uptime":
		this.SortByUptime(asc)
	}
}

func (this VMs) Paginate(page, size int) VMs {
	var (
		start = (page - 1) * size
		end   = start + size
	)

	if start >= len(this) {
		return VMs{}
	}

	if end > len(this) {
		end = len(this)
	}

	return this[start:end]
}

type VM struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	Experiment  string    `json:"experiment"`
	Host        string    `json:"host"`
	IPv4        []string  `json:"ipv4"`
	CPUs        int       `json:"cpus"`
	RAM         int       `json:"ram"`
	Disk        string    `json:"disk"`
	DoNotBoot   bool      `json:"dnb"`
	Networks    []string  `json:"networks"`
	Taps        []string  `json:"taps"`
	Captures    []Capture `json:"captures"`
	Running     bool      `json:"running"`
	Redeploying bool      `json:"redeploying"`
	Uptime      float64   `json:"uptime"`
	Screenshot  string    `json:"screenshot,omitempty"`

	// Used internally to track network <--> IP relationship, since
	// network ordering from minimega may not be the same as network
	// ordering in the experiment database.
	Interfaces map[string]string `json:"-"`

	// Used internally for showing VM details.
	OSType   string                 `json:"-"`
	Metadata map[string]interface{} `json:"-"`
}
