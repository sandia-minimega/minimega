// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"io/ioutil"
	log "minilog"
	"strconv"
	"strings"
	"time"

	proc "github.com/c9s/goprocinfo/linux"
)

// #include <unistd.h>
import "C"

var (
	ClkTck   = float64(C.sysconf(C._SC_CLK_TCK))
	PageSize = uint64(C.getpagesize())
)

type ProcStats struct {
	*proc.ProcessStat  // embed
	*proc.ProcessStatm // embed

	// time at beginning and end of data collection
	Begin, End time.Time

	Children map[int]*ProcStats
}

// CPU computes CPU % between two snapshots of proc
func (p *ProcStats) CPU(p2 *ProcStats) float64 {
	// compute number of tics used in window by process
	tics := float64((p2.Utime + p2.Stime) - (p.Utime + p.Stime))
	d := p2.End.Sub(p.Begin)

	return tics / ClkTck / d.Seconds()
}

// GuestCPU computes guest CPU % between two snapshots of proc
func (p *ProcStats) GuestCPU(p2 *ProcStats) float64 {
	vtics := float64(p2.GuestTime - p.GuestTime)
	d := p2.End.Sub(p.Begin)

	return vtics / ClkTck / d.Seconds()
}

// Time walks the tree and returns total time
func (p *ProcStats) Time() time.Duration {
	v := time.Duration(float64(p.Utime+p.Stime)/ClkTck) * time.Second

	for _, c := range p.Children {
		v += c.Time()
	}

	return v
}

// Size walks the tree and returns total memory size
func (p *ProcStats) Size() uint64 {
	v := PageSize * p.ProcessStatm.Size

	for _, c := range p.Children {
		v += c.Size()
	}

	return v
}

// Resident walks the tree and returns total resident memory size
func (p *ProcStats) Resident() uint64 {
	v := PageSize * p.ProcessStatm.Resident

	for _, c := range p.Children {
		v += c.Resident()
	}

	return v
}

// Share walks the tree and returns total shared memory size
func (p *ProcStats) Share() uint64 {
	v := PageSize * p.ProcessStatm.Share

	for _, c := range p.Children {
		v += c.Share()
	}

	return v
}

// GetProcStats reads the ProcStats for the given PID and its children.
func GetProcStats(pid int) (*ProcStats, error) {
	var err error

	p := &ProcStats{
		Begin:    time.Now(),
		Children: map[int]*ProcStats{},
	}

	p.ProcessStat, err = proc.ReadProcessStat(fmt.Sprintf("/proc/%v/stat", pid))
	if err != nil {
		return nil, fmt.Errorf("unable to read process stat: %v", err)
	}

	p.ProcessStatm, err = proc.ReadProcessStatm(fmt.Sprintf("/proc/%v/statm", pid))
	if err != nil {
		return nil, fmt.Errorf("unable to read process statm: %v", err)
	}

	p.End = time.Now()

	for _, c := range ListChildren(pid) {
		p2, err := GetProcStats(c)
		if err != nil {
			log.Debug("unable to read proc stats for %v: %v", c, err)
			continue
		}

		p.Children[c] = p2
	}

	return p, nil
}

func ListChildren(pid int) []int {
	b, err := ioutil.ReadFile(fmt.Sprintf("/proc/%[1]v/task/%[1]v/children", pid))
	if err != nil {
		return nil
	}

	res := []int{}

	for _, v := range strings.Fields(string(b)) {
		if i, err := strconv.Atoi(v); err == nil {
			res = append(res, i)
		}
	}

	return res
}
