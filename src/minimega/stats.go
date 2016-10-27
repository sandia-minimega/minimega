// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"minicli"
	log "minilog"
	"os"
	"runtime"
	"strconv"
	"strings"
)

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
			"host <netcommit,>",
			"host <vms,>",
		},
		Call: wrapBroadcastCLI(cliHost),
	},
}

var hostInfoFns = map[string]func() (string, error){
	"bandwidth": func() (string, error) {
		rx, tx := bridges.BandwidthStats()

		return fmt.Sprintf("%.1f/%.1f (rx/tx MB/s)", rx, tx), nil
	},
	"cpus": func() (string, error) {
		return strconv.Itoa(runtime.NumCPU()), nil
	},
	"load": hostStatsLoad,
	"memtotal": func() (string, error) {
		total, _, err := hostStatsMemory()
		return strconv.Itoa(total), err
	},
	"memused": func() (string, error) {
		_, used, err := hostStatsMemory()
		return strconv.Itoa(used), err
	},
	"vms": func() (string, error) {
		return strconv.Itoa(vms.Count()), nil
	},
	"cpucommit": func() (string, error) {
		return strconv.Itoa(vms.CPUCommit()), nil
	},
	"memcommit": func() (string, error) {
		return strconv.Itoa(vms.MemCommit()), nil
	},
	"netcommit": func() (string, error) {
		return strconv.Itoa(vms.NetworkCommit()), nil
	},
}

// Preferred ordering of host info fields in tabular
var hostInfoKeys = []string{
	"name", "cpus", "load", "memused", "memtotal", "bandwidth",
	"vms", "cpucommit", "memcommit", "netcommit",
}

func cliHost(c *minicli.Command, resp *minicli.Response) error {
	// If they selected one of the fields to display
	for k := range c.BoolArgs {
		val, err := hostInfoFns[k]()
		if err != nil {
			return err
		}

		resp.Response = val
		return nil
	}

	// Must want all fields
	resp.Header = hostInfoKeys

	row := []string{}
	for _, k := range resp.Header {
		val, err := hostInfoFns[k]()
		if err != nil {
			return err
		}

		row = append(row, val)
	}
	resp.Tabular = [][]string{row}

	return nil
}

func hostStatsLoad() (string, error) {
	load, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return "", err
	}

	// loadavg should look something like
	// 	0.31 0.28 0.24 1/309 21658
	f := strings.Fields(string(load))
	if len(f) != 5 {
		return "", fmt.Errorf("could not read loadavg")
	}
	outputLoad := strings.Join(f[0:3], " ")

	return outputLoad, nil
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
