// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

type HostStats struct {
	Name          string
	RxBps         float64
	TxBps         float64
	CPUs          int
	MemTotal      int
	MemUsed       int
	VMs           int
	Limit         int
	CPUCommit     uint64
	MemCommit     uint64
	NetworkCommit int
	Load          string
	Uptime        time.Duration
}

var hostCLIHandlers = []minicli.Handler{
	{ // host
		HelpShort: "report information about hosts",
		HelpLong: `
Report information about hosts in the current namespace:

- cpucommit  : total cpu commit
- cpus       : number of cpus
- load       : system load average
- memcommit  : total memory commit in MB
- memtotal   : total memory in MB
- memused    : memory used in MB
- name       : name of the machine
- netcommit  : total network interface commit
- rx         : RX bandwidth stats (MB/s)
- tx         : TX bandwidth stats (MB/s)
- uptime     : uptime
- vms        : number of VMs
- vmlimit    : limit based on coschedule values (-1 is no limit)

All VM-based stats are computed across namespaces.`,
		Patterns: []string{
			"host",
			"host <cpucommit,>",
			"host <cpus,>",
			"host <load,>",
			"host <memcommit,>",
			"host <memtotal,>",
			"host <memused,>",
			"host <name,>",
			"host <netcommit,>",
			"host <rx,>",
			"host <tx,>",
			"host <uptime,>",
			"host <vms,>",
			"host <vmlimit,>",
		},
		Call: wrapBroadcastCLI(cliHost),
	},
}

func init() {
	gob.Register(&HostStats{})
}

func (s *HostStats) IsFull() bool {
	return s.Limit != -1 && s.VMs >= s.Limit
}

func (h *HostStats) Print(v string) string {
	switch v {
	case "rx":
		return fmt.Sprintf("%.1f", h.RxBps)
	case "tx":
		return fmt.Sprintf("%.1f", h.TxBps)
	case "cpus":
		return strconv.Itoa(h.CPUs)
	case "cpucommit":
		return strconv.FormatUint(h.CPUCommit, 10)
	case "load":
		return h.Load
	case "memcommit":
		return strconv.FormatUint(h.MemCommit, 10)
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
	case "vmlimit":
		return strconv.Itoa(h.Limit)
	case "uptime":
		return h.Uptime.String()
	}

	return "???"

}

// Preferred ordering of host info fields in tabular. Don't include name --
// it's usually redundant in the tabular data unless .annotate is false.
var hostInfoKeys = []string{
	"cpus", "load", "memused", "memtotal", "rx", "tx", "vms", "vmlimit",
	"cpucommit", "memcommit", "netcommit", "uptime",
}

func cliHost(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	stats := NewHostStats()
	resp.Data = stats

	// If they selected one of the fields to display
	for k := range c.BoolArgs {
		resp.Response = stats.Print(k)
		return nil
	}

	// Must want all fields
	resp.Header = hostInfoKeys

	row := []string{}
	for _, k := range resp.Header {
		row = append(row, stats.Print(k))
	}
	resp.Tabular = [][]string{row}

	return nil
}

func hostLoad() (string, error) {
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

	return strings.Join(f[0:3], " "), nil
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
		log.Error("reading meminfo: %v", err)
	}

	outputMemUsed := (memTotal - (memFree + memBuffers + memCached)) / 1024
	outputMemTotal := memTotal / 1024

	return outputMemTotal, outputMemUsed, nil
}

func hostUptime() (time.Duration, error) {
	data, err := ioutil.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}

	// uptime should look something like
	//  2641.71 9287.55
	f := strings.Fields(string(data))
	if len(f) != 2 {
		return 0, errors.New("malformed uptime, expected float float")
	}

	uptime, err := time.ParseDuration(f[0] + "s")
	if err != nil {
		return 0, err
	}

	return uptime, nil
}
