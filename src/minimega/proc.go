// Copyright 2017-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"time"

	proc "github.com/c9s/goprocinfo/linux"
)

// #include <unistd.h>
import "C"

// ProcLimit is the maximum number of proceses to inspect per VM
const ProcLimit = 100

var (
	ClkTck   = float64(C.sysconf(C._SC_CLK_TCK))
	PageSize = uint64(C.getpagesize())
)

type ProcStats struct {
	*proc.ProcessStat  // embed
	*proc.ProcessStatm // embed

	// time at beginning and end of data collection
	Begin, End time.Time
}

type VMProcStats struct {
	Name, Namespace string

	// A and B are two snapshots of ProcStats
	A, B map[int]*ProcStats

	// queried from bridge
	RxRate, TxRate float64
}

// Time returns total time executed for all processes in MB
func (p *VMProcStats) Time() time.Duration {
	var tics uint64

	for _, v := range p.B {
		tics += v.Utime + v.Stime
	}

	return time.Duration(float64(tics)/ClkTck) * time.Second
}

// Size returns total memory size for all processes in MB
func (p *VMProcStats) Size() uint64 {
	var pages uint64

	for _, v := range p.B {
		pages += v.ProcessStatm.Size
	}

	return pages * PageSize
}

// Resident returns total resident memory size for all processes in MB
func (p *VMProcStats) Resident() uint64 {
	var pages uint64

	for _, v := range p.B {
		pages += v.ProcessStatm.Resident
	}

	return pages * PageSize
}

// Share returns total resident memory size for all processes in MB
func (p *VMProcStats) Share() uint64 {
	var pages uint64

	for _, v := range p.B {
		pages += v.ProcessStatm.Share
	}

	return pages * PageSize
}

// Count walks the tree and returns the number of processes
func (p *VMProcStats) Count() int {
	return len(p.B)
}

func (p *VMProcStats) cpuHelper(fn func(*ProcStats, *ProcStats) float64) float64 {
	var res float64

	// find overlapping PIDs in p.A and p.B
	for pid, v := range p.A {
		if v2, ok := p.B[pid]; ok {
			res += fn(v, v2)
		}
	}

	return res
}

func (p *VMProcStats) CPU() float64 {
	return p.cpuHelper(ProcCPU)
}

func (p *VMProcStats) GuestCPU() float64 {
	return p.cpuHelper(ProcGuestCPU)
}

// GetProcStats reads the ProcStats for the given PID.
func GetProcStats(pid int) (*ProcStats, error) {
	var err error

	p := &ProcStats{
		Begin: time.Now(),
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

	return p, nil
}

// ProcCPU computes CPU % between two snapshots of proc
func ProcCPU(p, p2 *ProcStats) float64 {
	// compute number of tics used in window by process
	tics := float64((p2.Utime + p2.Stime) - (p.Utime + p.Stime))
	d := p2.End.Sub(p.Begin)

	return tics / ClkTck / d.Seconds()
}

// ProcGuestCPU computes guest CPU % between two snapshots of proc
func ProcGuestCPU(p, p2 *ProcStats) float64 {
	vtics := float64(p2.GuestTime - p.GuestTime)
	d := p2.End.Sub(p.Begin)

	return vtics / ClkTck / d.Seconds()
}
