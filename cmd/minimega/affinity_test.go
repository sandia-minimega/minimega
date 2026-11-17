// Copyright 2015-2023 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

type pidVM struct {
	VM
	pid int
}

func (v pidVM) GetPID() int { return v.pid }

func sleeper(t *testing.T) int {
	if _, err := exec.LookPath("taskset"); err != nil {
		t.Skip("taskset not available")
	}
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	return cmd.Process.Pid
}

func pidCPUs(t *testing.T, pid int) string {
	out, err := exec.Command("taskset", "-c", "-p", strconv.Itoa(pid)).CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	return strings.TrimSpace(string(out)[strings.LastIndex(string(out), ":")+1:])
}

// Filter values are CPU numbers: each must pin to that CPU, including CPU 0.
func TestAddAffinityPinsToCPUNumber(t *testing.T) {
	// use CPUs this process may run on; the runner may not own CPUs 0 and 1
	var set unix.CPUSet
	if err := unix.SchedGetaffinity(0, &set); err != nil {
		t.Fatal(err)
	}
	var cpus []string
	for i := 0; i < 1024 && len(cpus) < 2; i++ { // 1024 = CPU_SETSIZE
		if set.IsSet(i) {
			cpus = append(cpus, strconv.Itoa(i))
		}
	}
	if len(cpus) < 2 {
		t.Skip("need 2 CPUs")
	}
	for _, cpu := range cpus {
		pid := sleeper(t)
		ns := &Namespace{}
		ns.affinityCPUSets = map[string][]int{cpu: {}}

		if err := ns.addAffinity(pidVM{pid: pid}); err != nil {
			t.Fatalf("cpu %v: %v", cpu, err)
		}
		if got := pidCPUs(t, pid); got != cpu {
			t.Errorf("cpu %v: pinned to %q", cpu, got)
		}
	}
}

// A VM that exited must not stop affinity from being disabled.
func TestDisableAffinityIgnoresDeadPIDs(t *testing.T) {
	pid := sleeper(t)
	ns := &Namespace{}
	ns.affinityEnabled = true
	ns.affinityCPUSets = map[string][]int{"0": {1 << 30, pid}}

	if err := ns.disableAffinity(); err != nil {
		t.Fatal(err)
	}
	if ns.affinityEnabled || ns.affinityCPUSets != nil {
		t.Errorf("affinity still enabled: %v %v", ns.affinityEnabled, ns.affinityCPUSets)
	}
}
