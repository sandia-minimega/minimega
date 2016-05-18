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
	"sync"
	"time"
)

const (
	MEGABYTE           = 1024 * 1024
	BANDWIDTH_INTERVAL = 5
)

type TapStat struct {
	Bridge      string
	RxStart     int
	RxStop      int
	TxStart     int
	TxStop      int
	Start, Stop time.Time
}

var (
	bandwidthStats map[string]*TapStat
	bandwidthLock  sync.Mutex
)

var hostCLIHandlers = []minicli.Handler{
	{ // host
		HelpShort: "report information about the host",
		Patterns: []string{
			"host",
			"host <name,>",
			"host <memused,>",
			"host <memtotal,>",
			"host <load,>",
			"host <bandwidth,>",
			"host <cpus,>",
		},
		Call: wrapBroadcastCLI(cliHost),
	},
}

func init() {
	go bandwidthCollector()
}

var hostInfoFns = map[string]func() (string, error){
	"name": func() (string, error) { return hostname, nil },
	"memused": func() (string, error) {
		_, used, err := hostStatsMemory()
		return fmt.Sprintf("%v MB", used), err
	},
	"memtotal": func() (string, error) {
		total, _, err := hostStatsMemory()
		return fmt.Sprintf("%v MB", total), err
	},
	"cpus": func() (string, error) {
		return fmt.Sprintf("%v", runtime.NumCPU()), nil
	},
	"bandwidth": hostStatsBandwidth,
	"load":      hostStatsLoad,
}

// Preferred ordering of host info fields in tabular
var hostInfoKeys = []string{
	"name", "cpus", "load", "memused", "memtotal", "bandwidth",
}

func (t TapStat) String() string {
	duration := t.Stop.Sub(t.Start).Seconds()
	rx := float64(t.RxStop-t.RxStart) / float64(MEGABYTE) / duration
	tx := float64(t.TxStop-t.TxStart) / float64(MEGABYTE) / duration

	// it's possible a VM went away during a previous poll, which can make
	// our value negative and invalid. Check for that and zero the field if
	// needed.
	if rx < 0.0 {
		rx = 0.0
	}
	if tx < 0.0 {
		tx = 0.0
	}

	return fmt.Sprintf("%.1f/%.1f", rx, tx)
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

func hostStatsBandwidth() (string, error) {
	bandwidthLock.Lock()
	defer bandwidthLock.Unlock()

	// get all rx and tx totals
	var rx int
	var tx int
	var duration float64
	for _, t := range bandwidthStats {
		rx += t.RxStop - t.RxStart
		tx += t.TxStop - t.TxStart

		duration += t.Stop.Sub(t.Start).Seconds()
	}

	rxMB := float64(rx) / float64(MEGABYTE) / duration
	txMB := float64(tx) / float64(MEGABYTE) / duration

	return fmt.Sprintf("%.1f/%.1f (rx/tx MB/s)", rxMB, txMB), nil
}

// readNetStats reads the tx or rx bytes for the given interface
func readNetStats(iface, dir string) (int, error) {
	d, err := ioutil.ReadFile(fmt.Sprintf("/sys/class/net/%v/statistics/%v_bytes", iface, dir))
	if err != nil {
		return 0, err
	}

	return strconv.Atoi(strings.TrimSpace(string(d)))
}

// enumerate bytes/second on all interfaces owned by minimega
func bandwidthCollector() {
	var err error
	for {
		time.Sleep(BANDWIDTH_INTERVAL * time.Second)

		stats := make(map[string]*TapStat)

		// get a list of every tap we own
		for _, v := range vms.Clone() {
			for _, net := range v.Config().Networks {
				stats[net.Tap] = &TapStat{
					Bridge: net.Bridge,
				}
			}
		}

		// for each tap, get rx/tx bytes
		for k, v := range stats {
			v.RxStart, err = readNetStats(k, "rx")
			if err != nil {
				log.Debugln(err)
				continue
			}

			v.TxStart, err = readNetStats(k, "tx")
			if err != nil {
				log.Debugln(err)
				continue
			}

			v.Start = time.Now()
		}

		time.Sleep(1 * time.Second)

		// and again
		for k, v := range stats {
			v.RxStop, err = readNetStats(k, "rx")
			if err != nil {
				log.Debugln(err)
				continue
			}

			v.TxStop, err = readNetStats(k, "tx")
			if err != nil {
				log.Debugln(err)
				continue
			}

			v.Stop = time.Now()
		}

		bandwidthLock.Lock()
		bandwidthStats = stats
		bandwidthLock.Unlock()
	}
}
