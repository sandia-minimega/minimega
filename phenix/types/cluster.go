package types

import (
	"errors"
	"sort"
)

var ErrHostNotFound = errors.New("host not found")

type Hosts []Host

func (this Hosts) SortByUnallocatedCPU(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		ui := this[i].CPUs - this[i].CPUCommit
		uj := this[j].CPUs - this[j].CPUCommit

		if asc {
			return ui < uj
		}

		return ui > uj
	})
}

func (this Hosts) SortByCommittedCPU(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return this[i].CPUCommit < this[j].CPUCommit
		}

		return this[i].CPUCommit > this[j].CPUCommit
	})
}

func (this Hosts) SortByUnallocatedMem(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		ui := this[i].MemTotal - this[i].MemCommit
		uj := this[j].MemTotal - this[j].MemCommit

		if asc {
			return ui < uj
		}

		return ui > uj
	})
}

func (this Hosts) SortByCommittedMem(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return this[i].MemCommit < this[j].MemCommit
		}

		return this[i].MemCommit > this[j].MemCommit
	})
}

func (this Hosts) SortByVMs(asc bool) {
	sort.Slice(this, func(i, j int) bool {
		if asc {
			return this[i].VMs < this[j].VMs
		}

		return this[i].VMs > this[j].VMs
	})
}

func (this Hosts) FindHostByName(name string) *Host {
	for _, host := range this {
		if host.Name == name {
			return &host
		}
	}

	return nil
}

func (this Hosts) IncrHostVMs(name string, incr int) error {
	for idx, host := range this {
		if host.Name == name {
			host.VMs += incr
			this[idx] = host

			return nil
		}
	}

	return ErrHostNotFound
}

func (this Hosts) IncrHostCPUCommit(name string, incr int) error {
	for idx, host := range this {
		if host.Name == name {
			host.CPUCommit += incr
			this[idx] = host

			return nil
		}
	}

	return ErrHostNotFound
}

func (this Hosts) IncrHostMemCommit(name string, incr int) error {
	for idx, host := range this {
		if host.Name == name {
			host.MemCommit += incr
			this[idx] = host

			return nil
		}
	}

	return ErrHostNotFound
}

type Cluster struct {
	Hosts []Host `json:"hosts"`
}

type Host struct {
	Name        string   `json:"name"`
	CPUs        int      `json:"cpus"`
	CPUCommit   int      `json:"cpucommit"`
	Load        []string `json:"load"`
	MemUsed     int      `json:"memused"`
	MemTotal    int      `json:"memtotal"`
	MemCommit   int      `json:"memcommit"`
	Tx          float64  `json:"tx"`
	Rx          float64  `json:"rx"`
	Bandwidth   string   `json:"bandwidth"`
	NetCommit   int      `json:"netcommit"`
	VMs         int      `json:"vms"`
	Uptime      float64  `json:"uptime"`
	Schedulable bool     `json:"schedulable"`
}
