package main

import (
	"fmt"
	"io/ioutil"
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

func hostStats(quiet bool) (string, error) {
	load, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return "", err
	}

	memory, err := ioutil.ReadFile("/proc/meminfo")
	if err != nil {
		return "", err
	}

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

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	// format the data

	// loadavg should look something like
	// 	0.31 0.28 0.24 1/309 21658
	f := strings.Fields(string(load))
	if len(f) != 5 {
		return "", fmt.Errorf("could not read loadavg")
	}
	outputLoad := strings.Join(f[0:3], " ")

	// meminfo - we're interested in MemTotal and MemFree+Cached+Buffers
	// we're doing this in a hacky way, and hoping the meminfo format is stable
	f = strings.Fields(string(memory))
	if len(f) < 12 {
		return "", fmt.Errorf("could not read meminfo")
	}
	if f[0] != "MemTotal:" {
		return "", fmt.Errorf("could not read meminfo")
	}
	memTotal, err := strconv.Atoi(f[1])
	if err != nil {
		return "", fmt.Errorf("could not read meminfo")
	}
	outputMemTotal := fmt.Sprintf("%d", memTotal/1024)
	if f[3] != "MemFree:" {
		return "", fmt.Errorf("could not read meminfo")
	}
	memFree, err := strconv.Atoi(f[4])
	if err != nil {
		return "", fmt.Errorf("could not read meminfo")
	}
	if f[6] != "Buffers:" {
		return "", fmt.Errorf("could not read meminfo")
	}
	memBuffers, err := strconv.Atoi(f[7])
	if err != nil {
		return "", fmt.Errorf("could not read meminfo")
	}
	if f[9] != "Cached:" {
		return "", fmt.Errorf("could not read meminfo")
	}
	memCached, err := strconv.Atoi(f[10])
	if err != nil {
		return "", fmt.Errorf("could not read meminfo")
	}
	outputMemUsed := fmt.Sprintf("%d", (memTotal-(memFree+memBuffers+memCached))/1024)

	// bandwidth ( megabytes / second ) for all interfaces in aggregate
	// again, a big hack, this time we look for a string with a ":" suffix, and offset from there
	f = strings.Fields(string(band1))
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

	var output string
	if quiet {
		output = fmt.Sprintf("%v %v %v %v %v", hostname, outputLoad, outputMemTotal, outputMemUsed, outputBandwidth)
	} else {
		output = fmt.Sprintf("hostname:\t%v\tload average:\t%v\tmemtotal:\t%v\tmemused:\t%v\tbandwidth:\t%v (MB/s)", hostname, outputLoad, outputMemTotal, outputMemUsed, outputBandwidth)
	}
	return output, nil
}
