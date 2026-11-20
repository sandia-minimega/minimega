// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"
)

// fcNetworkStageMax is the largest limit of the network stage. The kernel
// serializes netlink changes on rtnl, so more launches in the stage only wait
// on that lock.
const fcNetworkStageMax = 8

// fcStage limits the number of launches in one stage at the same time.
type fcStage struct {
	ch chan struct{}
}

func newFCStage(n int) *fcStage {
	return &fcStage{ch: make(chan struct{}, max(n, 1))}
}

func (s *fcStage) acquire() {
	s.ch <- struct{}{}
}

func (s *fcStage) release() {
	<-s.ch
}

// run runs fn in the stage and records the times in t.
func (s *fcStage) run(t *fcStageTime, fn func() error) error {
	start := time.Now()
	s.acquire()
	defer s.release()

	entered := time.Now()
	t.Entered = true
	t.Wait = entered.Sub(start)
	defer func() { t.Run = time.Since(entered) }()

	return fn()
}

// runIf is run when enter is true. Otherwise it runs fn outside the stage and
// records nothing.
func (s *fcStage) runIf(enter bool, t *fcStageTime, fn func() error) error {
	if !enter {
		return fn()
	}

	return s.run(t, fn)
}

// fcLaunchStages are the stages of a Firecracker launch: disk (disks,
// injects, and boot files), network (netns and netlink setup of the NICs), and
// vmm (jail, jailer, API socket, and VMM configuration). inflight bounds the
// first launches from the start of the launch to the end of the vmm stage.
type fcLaunchStages struct {
	inflight, disk, network, vmm *fcStage
}

// newFCLaunchStages returns the stages for p parallel launches.
func newFCLaunchStages(p int) *fcLaunchStages {
	p = max(p, 1)

	return &fcLaunchStages{
		inflight: newFCStage(4 * p),
		disk:     newFCStage(p),
		network:  newFCStage(min(p, fcNetworkStageMax)),
		vmm:      newFCStage(2 * p),
	}
}

// fcStages are the launch stages. When nil, fcGetStages sets the stages for
// -firecracker-launch-parallel. Tests set it.
var (
	fcStages   *fcLaunchStages
	fcStagesMu sync.Mutex
)

func fcGetStages() *fcLaunchStages {
	fcStagesMu.Lock()
	defer fcStagesMu.Unlock()

	if fcStages == nil {
		fcStages = newFCLaunchStages(fcLaunchParallel())
	}

	return fcStages
}

// fcLaunchParallel returns -firecracker-launch-parallel, or the number of
// CPUs when it is 0.
func fcLaunchParallel() int {
	if n := *f_firecrackerLaunchParallel; n > 0 {
		return n
	}

	return runtime.NumCPU()
}

// fcStartParallel returns -firecracker-start-parallel, or the number of CPUs
// when it is 0.
func fcStartParallel() int {
	if n := *f_firecrackerStartParallel; n > 0 {
		return n
	}

	return runtime.NumCPU()
}

// fcStageTime is the time that a launch waited for a stage and the time that
// it ran in the stage.
type fcStageTime struct {
	Entered   bool
	Wait, Run time.Duration
}

func (t fcStageTime) String() string {
	if !t.Entered {
		return "-"
	}

	return fmt.Sprintf("wait %v run %v", fcRoundDuration(t.Wait), fcRoundDuration(t.Run))
}

// fcLaunchTimes are the times of one launch. Admit is the wait for the
// in-flight bound, and Bridge is the wait for the bridge port adds after the
// network stage.
type fcLaunchTimes struct {
	Admit              time.Duration
	Disk, Network, VMM fcStageTime
	Bridge             time.Duration
}

// waits returns the sum of the times that the launch waited for a stage.
func (t fcLaunchTimes) waits() time.Duration {
	return t.Admit + t.Disk.Wait + t.Network.Wait + t.VMM.Wait
}

func (t fcLaunchTimes) String() string {
	return fmt.Sprintf("in-flight wait %v; disk %v; network %v; bridge %v; vmm %v",
		fcRoundDuration(t.Admit), t.Disk, t.Network, fcRoundDuration(t.Bridge), t.VMM)
}

// fcTimedVM is a VM that keeps the times of its last launch.
type fcTimedVM interface {
	launchTimes() fcLaunchTimes
}

// fcLaunchBatch collects the launch times of the Firecracker VMs of one
// vms.Launch batch.
type fcLaunchBatch struct {
	mu    sync.Mutex
	times []fcLaunchTimes
}

func (b *fcLaunchBatch) add(vm VM) {
	var t fcLaunchTimes
	if v, ok := vm.(fcTimedVM); ok {
		t = v.launchTimes()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.times = append(b.times, t)
}

func (b *fcLaunchBatch) len() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.times)
}

func (b *fcLaunchBatch) message(total time.Duration) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return fcLaunchSummary(b.times, total)
}

// fcLaunchSummary returns fcLaunchedMessage for the launches with times,
// with the batch time total and the p50 and p95 run time of each stage and
// p95 of the waits.
func fcLaunchSummary(times []fcLaunchTimes, total time.Duration) string {
	var disk, network, vmm, waits []time.Duration
	for _, t := range times {
		disk = append(disk, t.Disk.Run)
		network = append(network, t.Network.Run)
		vmm = append(vmm, t.VMM.Run)
		waits = append(waits, t.waits())
	}

	return fmt.Sprintf("%v in %v (disk p50/p95 %v, network p50/p95 %v, vmm p50/p95 %v; waits p95 %v)",
		fcLaunchedMessage(len(times)), fcRoundDuration(total),
		fcMedianP95(disk), fcMedianP95(network), fcMedianP95(vmm),
		fcRoundDuration(fcPercentile(waits, 95)))
}

func fcMedianP95(d []time.Duration) string {
	return fmt.Sprintf("%v/%v", fcRoundDuration(fcPercentile(d, 50)), fcRoundDuration(fcPercentile(d, 95)))
}

// fcPercentile returns the p-th percentile of d by the nearest-rank method.
// It sorts d.
func fcPercentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}

	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })

	i := (p*len(d)+99)/100 - 1
	return d[min(max(i, 0), len(d)-1)]
}

// fcRoundDuration rounds d to milliseconds from one second, to 0.1 ms from
// one millisecond, and to microseconds below that.
func fcRoundDuration(d time.Duration) time.Duration {
	switch {
	case d >= time.Second:
		return d.Round(time.Millisecond)
	case d >= time.Millisecond:
		return d.Round(100 * time.Microsecond)
	}

	return d.Round(time.Microsecond)
}
