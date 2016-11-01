// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"runtime"
	"strconv"
	"strings"
)

type HostStats struct {
	Name          string
	RxBps         float64
	TxBps         float64
	CPUs          int
	MemTotal      int
	MemUsed       int
	VMs           int
	CPUCommit     int
	MemCommit     int
	NetworkCommit int
	Load          string

	Limit int // for scheduler, not used by the host API
}

var hostCLIHandlers = []minicli.Handler{
	{ // host
		HelpShort: "report information about the host",
		HelpLong: `
Report information about the host:

- bandwidth  : RX/TX bandwidth stats
- cpucommit  : total cpu commit
- cpus       : number of cpus
- load       : system load average
- memcommit  : total memory commit in MB
- memtotal   : total memory in MB
- memused    : memory used in MB
- name       : name of the machine
- netcommit  : total network interface commit
- vms        : number of VMs

All stats about VMs are based on the active namespace. To see information
across namespaces, run "mesh send all host".`,
		Patterns: []string{
			"host",
			"host <bandwidth,>",
			"host <cpus,>",
			"host <cpucommit,>",
			"host <load,>",
			"host <memcommit,>",
			"host <memtotal,>",
			"host <memused,>",
			"host <name,>",
			"host <netcommit,>",
			"host <vms,>",
		},
		Call: wrapBroadcastCLI(cliHost),
	},
}

func (h *HostStats) Populate(v string) error {
	switch v {
	case "bandwidth":
		h.RxBps, h.TxBps = bridges.BandwidthStats()
	case "cpus":
		h.CPUs = runtime.NumCPU()
	case "cpucommit":
		h.CPUCommit = vms.CPUCommit()
	case "load":
		load, err := ioutil.ReadFile("/proc/loadavg")
		if err != nil {
			return err
		}

		// loadavg should look something like
		// 	0.31 0.28 0.24 1/309 21658
		f := strings.Fields(string(load))
		if len(f) != 5 {
			return fmt.Errorf("could not read loadavg")
		}

		h.Load = strings.Join(f[0:3], " ")
		return nil
	case "memcommit":
		h.MemCommit = vms.MemCommit()
	case "memtotal", "memused":
		total, used, err := hostStatsMemory()
		h.MemTotal = total
		h.MemUsed = used
		return err
	case "name":
		h.Name = hostname
	case "netcommit":
		h.NetworkCommit = vms.NetworkCommit()
	case "vms":
		h.VMs = vms.Count()
	default:
		return errors.New("unreachable")
	}

	return nil
}

func (h *HostStats) Print(v string) string {
	switch v {
	case "bandwidth":
		return fmt.Sprintf("%.1f/%.1f (rx/tx MB/s)", h.RxBps, h.TxBps)
	case "cpus":
		return strconv.Itoa(h.CPUs)
	case "cpucommit":
		return strconv.Itoa(h.CPUCommit)
	case "load":
		return h.Load
	case "memcommit":
		return strconv.Itoa(h.MemCommit)
	case "memtotal":
		return strconv.Itoa(h.MemTotal)
	case "memused":
		return strconv.Itoa(h.MemUsed)
	case "name":
		return h.Name
	case "netcommit":
		return strconv.Itoa(h.NetworkCommit)
	case "vms":
		return strconv.Itoa(h.VMs)
	}

	return "???"

}

// Preferred ordering of host info fields in tabular. Don't include name --
// it's usually redundant in the tabular data unless .annotate is false.
var hostInfoKeys = []string{
	"cpus", "load", "memused", "memtotal", "bandwidth", "vms", "cpucommit",
	"memcommit", "netcommit",
}

func cliHost(c *minicli.Command, resp *minicli.Response) error {
	stats := HostStats{
		Name: hostname,
	}
	resp.Data = &stats

	// If they selected one of the fields to display
	for k := range c.BoolArgs {
		if err := stats.Populate(k); err != nil {
			return err
		}

		resp.Response = stats.Print(k)
		return nil
	}

	// Must want all fields
	resp.Header = hostInfoKeys

	row := []string{}
	for _, k := range resp.Header {
		if err := stats.Populate(k); err != nil {
			return err
		}

		row = append(row, stats.Print(k))
	}
	resp.Tabular = [][]string{row}

	return nil
}

func hostStatsMemory() (int, int, error) {
	memory, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer memory.Close()

	scanner := bufio.NewScanner(memory)

	var memTotal int
	var memFree int
	var memCached int
	var memBuffers int

	for scanner.Scan() {
		d := strings.Fields(scanner.Text())
		switch d[0] {
		case "MemTotal:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo MemTotal: %v", err)
			}
			memTotal = m
			log.Debug("got memTotal %v", memTotal)
		case "MemFree:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo MemFree: %v", err)
			}
			memFree = m
			log.Debug("got memFree %v", memFree)
		case "Buffers:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo Buffers: %v", err)
			}
			memBuffers = m
			log.Debug("got memBuffers %v", memBuffers)
		case "Cached:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo Cached: %v", err)
			}
			memCached = m
			log.Debug("got memCached %v", memCached)
		}
	}

	if err := scanner.Err(); err != nil {
		log.Error("reading meminfo:", err)
	}

	outputMemUsed := (memTotal - (memFree + memBuffers + memCached)) / 1024
	outputMemTotal := memTotal / 1024

	return outputMemTotal, outputMemUsed, nil
}
