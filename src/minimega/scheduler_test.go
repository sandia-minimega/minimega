// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"testing"
)

var ThreeVMs = []*QueuedVMs{
	&QueuedVMs{
		Names: []string{"a", "b", "c"},
		VMConfig: VMConfig{
			BaseConfig: BaseConfig{
				SchedulePeers: "0",
				Vcpus:         "1",
				Memory:        "1",
			},
		},
	},
}

func fakeHostData(N int) []*HostStats {
	res := []*HostStats{}

	for i := 0; i < N; i++ {
		res = append(res, &HostStats{
			Name:          strconv.Itoa(i),
			CPUCommit:     i,
			MemCommit:     i,
			NetworkCommit: i,
			CPUs:          1, // actual number doesn't matter
			MemTotal:      1, // actual number doesn't matter
		})
	}

	// same seed so that we don't get unreproducable results
	r := rand.New(rand.NewSource(0))

	// randomly shuffle the elements
	for i := 0; i < len(res)-2; i++ {
		j := r.Intn(i + 1)
		res[i], res[j] = res[j], res[i]
	}

	return res
}

// testHostSort sorts fakeHostData for n hosts using the provided hostSortBy
// function then updates the root and call Update many times to see if we keep
// getting the correct minimum.
func testHostSort(n int, by hostSortBy) error {
	hosts := fakeHostData(n)

	by.Sort(hosts)

	for i := 0; i < 10*n; i++ {
		v, _ := strconv.Atoi(hosts[0].Name)
		if i%n != v {
			return fmt.Errorf("incorrect minimum: %v != %v", i, v)
		}

		// increment all by n so that they move to the bottom of the heap
		hosts[0].CPUCommit += n
		hosts[0].MemCommit += n
		hosts[0].NetworkCommit += n

		by.Update(hosts, hosts[0].Name)
	}

	return nil
}

func TestHostSortCPU(t *testing.T) {
	if err := testHostSort(100, cpuCommit); err != nil {
		t.Error(err)
	}
}

func TestHostSortMem(t *testing.T) {
	if err := testHostSort(100, memoryCommit); err != nil {
		t.Error(err)
	}
}

func TestHostSortNet(t *testing.T) {
	if err := testHostSort(100, networkCommit); err != nil {
		t.Error(err)
	}
}

func TestCPUCommit(t *testing.T) {
	// h < h2
	h := &HostStats{
		CPUCommit: 1,
		CPUs:      1,
	}
	h2 := &HostStats{
		CPUCommit: 2,
		CPUs:      1,
	}

	if !cpuCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}

	// make h "full"
	h.VMs = 1
	h.Limit = 1

	if cpuCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}
}

func TestMemCommit(t *testing.T) {
	// h < h2
	h := &HostStats{
		MemCommit: 1,
		MemTotal:  1,
	}
	h2 := &HostStats{
		MemCommit: 2,
		MemTotal:  1,
	}

	if !memoryCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}

	// make h "full"
	h.VMs = 1
	h.Limit = 1

	if memoryCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}
}

func TestNetCommit(t *testing.T) {
	// h < h2
	h := &HostStats{
		NetworkCommit: 1,
	}
	h2 := &HostStats{
		NetworkCommit: 2,
	}

	if !networkCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}

	// make h "full"
	h.VMs = 1
	h.Limit = 1

	if networkCommit(h, h2) {
		t.Errorf("%v > %v", h, h2)
	}
}

func TestQueuedVMsLess(t *testing.T) {
	// q < q2
	q := &QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q.ScheduleHost = "foo"
	q.SchedulePeers = "1"

	q2 := &QueuedVMs{
		Names: []string{"a", "b"},
	}
	q2.ScheduleHost = "foo"

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}

	// q < q2
	q = &QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q.SchedulePeers = "1"
	q2 = &QueuedVMs{
		Names: []string{"a", "b"},
	}

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}

	// q < q2
	q = &QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q2 = &QueuedVMs{
		Names: []string{"a", "b"},
	}

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}
}

func TestScheduleImpossible(t *testing.T) {
	// three VMs with 0 peers and only two machines
	queue := ThreeVMs
	hosts := fakeHostData(2)

	if s, err := schedule(queue, hosts, cpuCommit); err == nil {
		t.Error("scheduler did the impossible: %v", s)
	}
}

func TestScheduleExact(t *testing.T) {
	// three VMs with 0 peers and three machines
	queue := ThreeVMs
	hosts := fakeHostData(3)

	if _, err := schedule(queue, hosts, cpuCommit); err != nil {
		t.Error(err)
	}
}

func TestScheduleEasy(t *testing.T) {
	// three VMs with 0 peers and four machines
	queue := ThreeVMs
	hosts := fakeHostData(4)

	if _, err := schedule(queue, hosts, cpuCommit); err != nil {
		t.Error(err)
	}
}

func TestScheduleHost(t *testing.T) {
	// four VMs with 0 peers, one pinned to host 0 and four machines
	queue := append(ThreeVMs, &QueuedVMs{
		Names: []string{"picky"},
		VMConfig: VMConfig{
			BaseConfig: BaseConfig{
				ScheduleHost:  "0",
				SchedulePeers: "0",
				Vcpus:         "1",
				Memory:        "1",
			},
		},
	})
	hosts := fakeHostData(4)

	s, err := schedule(queue, hosts, cpuCommit)
	if err != nil {
		t.Error(err)
	}

	if len(s["0"]) > 1 {
		t.Error("too many VMs on host 0")
	}
}

func TestScheduleBig(t *testing.T) {
	var names []string
	for i := 0; i < 1000; i++ {
		names = append(names, strconv.Itoa(i))
	}

	queue := []*QueuedVMs{
		&QueuedVMs{
			Names: names,
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					Vcpus:  "1",
					Memory: "1",
				},
			},
		},
		&QueuedVMs{
			Names: []string{"picky"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					ScheduleHost:  "0",
					SchedulePeers: "0",
					Vcpus:         "1",
					Memory:        "1",
				},
			},
		},
		&QueuedVMs{
			Names: []string{"lesspicky"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					SchedulePeers: "0",
					Vcpus:         "1",
					Memory:        "1",
				},
			},
		},
		&QueuedVMs{
			// Ideally, these would be scheduled on the same host but the
			// scheduler isn't smart enough. Instead, we expect two hosts to
			// have two VMs each.
			Names: []string{"ice", "fire"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					SchedulePeers: "1",
					Vcpus:         "1",
					Memory:        "1",
				},
			},
		},
	}
	hosts := fakeHostData(40)

	s, err := schedule(queue, hosts, cpuCommit)
	if err != nil {
		t.Error(err)
	}

outer:
	for k, v := range s {
		// skip "picky"
		if len(v) == 1 && len(v[0].Names) == 1 {
			continue
		}

		// search for the picky VMs in hosts with more than one VM
		for _, q := range v {
			for _, n := range q.Names {
				if strings.Contains(n, "picky") {
					t.Errorf("too many VMs on host %v for %v", k, n)

					continue outer
				}
			}
		}
	}
}
