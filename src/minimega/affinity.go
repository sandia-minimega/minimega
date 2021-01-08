// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

package main

import (
	"errors"
	"fmt"
	log "minilog"
	"runtime"
	"strconv"
)

// initAffinity creates affinityCPUSets according to affinityFilter.
func (ns *Namespace) initAffinity() {
	ns.affinityMu.Lock()
	defer ns.affinityMu.Unlock()

	ns.affinityCPUSets = make(map[string][]int)
	if len(ns.affinityFilter) > 0 {
		for _, cpu := range ns.affinityFilter {
			ns.affinityCPUSets[cpu] = []int{}
		}

		return
	}

	for i := 0; i < runtime.NumCPU(); i++ {
		ns.affinityCPUSets[strconv.Itoa(i)] = []int{}
	}
}

// enableAffinity applies affinity to all VMs in the namespace. If affinity was
// already assigned to some VMs, they will be reassigned.
func (ns *Namespace) enableAffinity() error {
	// clear previous affinity
	ns.initAffinity()

	err := ns.Apply(Wildcard, func(vm VM, _ bool) (bool, error) {
		return true, ns.addAffinity(vm)
	})

	if err == nil {
		ns.affinityEnabled = true
	}

	return err
}

// addAffinity adds affinity to a single VM.
func (ns *Namespace) addAffinity(vm VM) error {
	ns.affinityMu.Lock()
	defer ns.affinityMu.Unlock()

	// find cpu with the fewest number of entries
	var cpu string
	for k, v := range ns.affinityCPUSets {
		if cpu == "" {
			cpu = k
			continue
		}
		if len(v) < len(ns.affinityCPUSets[cpu]) {
			cpu = k
		}
	}

	if cpu == "" {
		return errors.New("could not find a valid CPU set!")
	}

	if err := setAffinity(cpu, vm.GetPID()); err != nil {
		return err
	}

	ns.affinityCPUSets[cpu] = append(ns.affinityCPUSets[cpu], vm.GetPID())
	return nil
}

func (ns *Namespace) disableAffinity() error {
	if !ns.affinityEnabled {
		return errors.New("affinity is not enabled for this namespace")
	}

	ns.affinityMu.Lock()
	defer ns.affinityMu.Unlock()

	for cpu, pids := range ns.affinityCPUSets {
		for _, pid := range pids {
			if err := clearAffinity(pid); err != nil {
				return err
			}
		}

		ns.affinityCPUSets[cpu] = nil
	}

	ns.affinityCPUSets = nil
	ns.affinityEnabled = false
	return nil
}

// setAffinity sets the affinity for the PID to the given CPU.
func setAffinity(cpu string, pid int) error {
	log.Debug("set affinity to %v for %v", cpu, pid)

	out, err := processWrapper("taskset", "-a", "-p", cpu, strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("%v: %v", err, out)
	}
	return nil
}

// clearAffinity removes the affinity for a PID.
func clearAffinity(pid int) error {
	log.Debug("clear affinity for %v", pid)

	out, err := processWrapper("taskset", "-p", "0xffffffffffffffff", strconv.Itoa(pid))
	if err != nil {
		return fmt.Errorf("%v: %v", err, out)
	}
	return nil
}
