// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	log "minilog"
	"os"
	"strconv"
	"strings"
	"time"
)

var (
	bandwidthLast     int64
	bandwidthLastTime int64
)

func hostStatsCLI(c cliCommand) cliResponse {
	var quiet bool
	if len(c.Args) != 0 {
		quiet = true
	}
	s, err := hostStats(quiet)
	if err != nil {
		return cliResponse{
			Error: err.Error(),
		}
	}
	return cliResponse{
		Response: s,
	}
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
			log.Debugln("got memTotal %v", memTotal)
		case "MemFree:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo MemFree: %v", err)
			}
			memFree = m
			log.Debugln("got memFree %v", memFree)
		case "Buffers:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo Buffers: %v", err)
			}
			memBuffers = m
			log.Debugln("got memBuffers %v", memBuffers)
		case "Cached:":
			m, err := strconv.Atoi(d[1])
			if err != nil {
				return 0, 0, fmt.Errorf("cannot parse meminfo Cached: %v", err)
			}
			memCached = m
			log.Debugln("got memCached %v", memCached)
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
	band1, err := ioutil.ReadFile("/proc/net/dev")
	if err != nil {
		return "", err
	}

	time.Sleep(1 * time.Second)

	band2, err := ioutil.ReadFile("/proc/net/dev")
	if err != nil {
		return "", err
	}
	now := time.Now().Unix()

	// bandwidth ( megabytes / second ) for all interfaces in aggregate
	// again, a big hack, this time we look for a string with a ":" suffix, and offset from there
	f := strings.Fields(string(band1))
	var total1 int64
	var elapsed int64
	if bandwidthLast == 0 {
		for i, v := range f {
			if strings.HasSuffix(v, ":") {
				if len(f) < (i + 16) {
					return "", fmt.Errorf("could not read netdev")
				}
				recv, err := strconv.ParseInt(f[i+1], 10, 64)
				if err != nil {
					return "", fmt.Errorf("could not read netdev")
				}
				send, err := strconv.ParseInt(f[i+9], 10, 64)
				if err != nil {
					return "", fmt.Errorf("could not read netdev")
				}
				total1 += recv + send
			}
		}
		elapsed = 1
	} else {
		total1 = bandwidthLast
		elapsed = now - bandwidthLastTime
	}

	f = strings.Fields(string(band2))
	var total2 int64
	for i, v := range f {
		if strings.HasSuffix(v, ":") {
			if len(f) < (i + 16) {
				return "", fmt.Errorf("could not read netdev")
			}
			recv, err := strconv.ParseInt(f[i+1], 10, 64)
			if err != nil {
				return "", fmt.Errorf("could not read netdev")
			}
			send, err := strconv.ParseInt(f[i+9], 10, 64)
			if err != nil {
				return "", fmt.Errorf("could not read netdev")
			}
			total2 += recv + send
		}
	}

	bandwidth := (float32(total2-total1) / 1048576.0) / float32(elapsed)
	outputBandwidth := fmt.Sprintf("%.1f", bandwidth)
	bandwidthLast = total2
	bandwidthLastTime = now

	return outputBandwidth, nil
}

func hostStats(quiet bool) (string, error) {
	load, err := hostStatsLoad()
	if err != nil {
		return "", err
	}

	memoryTotal, memoryUsed, err := hostStatsMemory()
	if err != nil {
		return "", err
	}

	bandwidth, err := hostStatsBandwidth()
	if err != nil {
		return "", err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	var output string
	if quiet {
		output = fmt.Sprintf("%v %v %v %v %v", hostname, load, memoryTotal, memoryUsed, bandwidth)
	} else {
		output = fmt.Sprintf("hostname:\t%v\tload average:\t%v\tmemtotal:\t%v\tmemused:\t%v\tbandwidth:\t%v (MB/s)", hostname, load, memoryTotal, memoryUsed, bandwidth)
	}
	return output, nil
}
