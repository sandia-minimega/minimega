// Copyright 2015-2021 National Technology & Engineering Solutions of Sandia, LLC (NTESS). 
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain 
// rights in this software.

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
				Coschedule: 0,
				VCPUs:      1,
				Memory:     1,
			},
		},
	},
}

func fakeHostData(N int, uniform bool) []*HostStats {
	res := []*HostStats{}

	for i := 0; i < N; i++ {
		c := uint64(i)
		if uniform {
			c = 1
		}

		res = append(res, &HostStats{
			Name:          strconv.Itoa(i),
			CPUCommit:     c,
			MemCommit:     c,
			NetworkCommit: int(c),
			CPUs:          1, // actual number doesn't matter
			MemTotal:      1, // actual number doesn't matter
			Limit:         -1,
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

// testHostSort sorts fakeHostData for N hosts using the provided hostSortBy
// function then updates the root and call Update many times to see if we keep
// getting the correct minimum.
func testHostSort(N int, by hostSortBy) error {
	hosts := fakeHostData(N, false)

	by.Sort(hosts)

	for i := 0; i < 10*N; i++ {
		v, _ := strconv.Atoi(hosts[0].Name)
		if i%N != v {
			return fmt.Errorf("incorrect minimum: %v != %v", i, v)
		}

		// increment all by N so that they move to the bottom of the heap
		hosts[0].CPUCommit += uint64(N)
		hosts[0].MemCommit += uint64(N)
		hosts[0].NetworkCommit += N

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
		Limit:     -1,
	}
	h2 := &HostStats{
		CPUCommit: 2,
		CPUs:      1,
		Limit:     -1,
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
		Limit:     -1,
	}
	h2 := &HostStats{
		MemCommit: 2,
		MemTotal:  1,
		Limit:     -1,
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
		Limit:         -1,
	}
	h2 := &HostStats{
		NetworkCommit: 2,
		Limit:         -1,
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
		Names:    []string{"a", "b", "c"},
		VMConfig: NewVMConfig(),
	}
	q.Schedule = "foo"
	q.Coschedule = 1

	q2 := &QueuedVMs{
		Names:    []string{"a", "b"},
		VMConfig: NewVMConfig(),
	}
	q2.Schedule = "foo"

	if !q.Less(q2) {
		t.Errorf("%v < %v", &q2.BaseConfig, &q.BaseConfig)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}

	// q < q2
	q = &QueuedVMs{
		Names:    []string{"a", "b", "c"},
		VMConfig: NewVMConfig(),
	}
	q.Coschedule = 1
	q2 = &QueuedVMs{
		Names:    []string{"a", "b"},
		VMConfig: NewVMConfig(),
	}

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}

	// q < q2
	q = &QueuedVMs{
		Names:    []string{"a", "b", "c"},
		VMConfig: NewVMConfig(),
	}
	q2 = &QueuedVMs{
		Names:    []string{"a", "b"},
		VMConfig: NewVMConfig(),
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
	hosts := fakeHostData(2, false)

	if s, err := schedule(queue, hosts, cpuCommit); err == nil {
		t.Errorf("scheduler did the impossible: %v", s)
	}
}

func TestScheduleExact(t *testing.T) {
	// three VMs with 0 peers and three machines
	queue := ThreeVMs
	hosts := fakeHostData(3, false)

	if _, err := schedule(queue, hosts, cpuCommit); err != nil {
		t.Error(err)
	}
}

func TestScheduleEasy(t *testing.T) {
	// three VMs with 0 peers and four machines
	queue := ThreeVMs
	hosts := fakeHostData(4, false)

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
				Schedule:   "0",
				Coschedule: 0,
				VCPUs:      1,
				Memory:     1,
			},
		},
	})
	hosts := fakeHostData(4, false)

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
					VCPUs:      1,
					Memory:     1,
					Coschedule: -1,
				},
			},
		},
		&QueuedVMs{
			Names: []string{"picky"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					Schedule:   "0",
					Coschedule: 0,
					VCPUs:      1,
					Memory:     1,
				},
			},
		},
		&QueuedVMs{
			Names: []string{"lesspicky"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					Coschedule: 0,
					VCPUs:      1,
					Memory:     1,
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
					Coschedule: 1,
					VCPUs:      1,
					Memory:     1,
				},
			},
		},
	}
	hosts := fakeHostData(40, false)

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

func testScheduleUniformity(N, M int, by hostSortBy) error {
	var queue []*QueuedVMs

	var want uint64
	for i := 0; i < N; i++ {
		want += uint64(i)

		var names []string
		for j := 0; j < M; j++ {
			names = append(names, strconv.Itoa(i))
		}

		var nets []NetConfig
		for j := 0; j < i; j++ {
			nets = append(nets, NetConfig{})
		}

		queue = append(queue, &QueuedVMs{
			Names: names,
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					VCPUs:      uint64(i),
					Memory:     uint64(i),
					Networks:   nets,
					Coschedule: -1,
				},
			},
		})
	}

	hosts := fakeHostData(M, true)

	// scheduling should evenly distribute VMs over machines
	s, err := schedule(queue, hosts, by)
	if err != nil {
		return err
	}

	for k, v := range s {
		var cpu, mem, nets uint64
		for _, q := range v {
			cpu += q.VCPUs * uint64(len(q.Names))
			mem += q.Memory * uint64(len(q.Names))

			nets += uint64(len(q.Networks) * len(q.Names))
		}

		if cpu != want {
			return fmt.Errorf("cpu commit uneven for %v: %v != %v", k, cpu, want)
		}
		if mem != want {
			return fmt.Errorf("memory commit uneven for %v: %v != %v", k, mem, want)
		}
		if nets != want {
			return fmt.Errorf("network commit uneven for %v: %v != %v", k, nets, want)
		}
	}

	return nil
}

func TestScheduleUniformityCPU(t *testing.T) {
	if err := testScheduleUniformity(10, 10, cpuCommit); err != nil {
		t.Error(err)
	}
}

func TestScheduleUniformityMem(t *testing.T) {
	if err := testScheduleUniformity(10, 10, memoryCommit); err != nil {
		t.Error(err)
	}
}

func TestScheduleUniformityNet(t *testing.T) {
	if err := testScheduleUniformity(10, 10, networkCommit); err != nil {
		t.Error(err)
	}
}

// TestColocate tests colocate chaining
func TestColocate(t *testing.T) {
	queue := []*QueuedVMs{
		&QueuedVMs{
			Names: []string{"vm-0"},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					VCPUs:      1,
					Memory:     1,
					Coschedule: -1,
					Schedule:   "0",
				},
			},
		},
	}
	for i := 1; i < 10; i++ {
		queue = append(queue, &QueuedVMs{
			Names: []string{"vm-" + strconv.Itoa(i)},
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					Colocate:   "vm-" + strconv.Itoa(i-1),
					VCPUs:      1,
					Memory:     1,
					Coschedule: -1,
				},
			},
		})
	}

	hosts := fakeHostData(2, true)

	s, err := schedule(queue, hosts, cpuCommit)
	if err != nil {
		t.Error(err)
	}

	// all VMs should be on "0"
	if len(s["0"]) != 10 {
		t.Errorf("expected 10 VMs on `0`, got %v", len(s["0"]))
	}
}

// TestColocateError tests a few impossible configurations
func TestColocateError(t *testing.T) {
	a := &QueuedVMs{
		Names: []string{"a"},
		VMConfig: VMConfig{
			BaseConfig: BaseConfig{
				VCPUs:      1,
				Memory:     1,
				Coschedule: -1,
				Schedule:   "0",
			},
		},
	}
	b := &QueuedVMs{
		Names: []string{"b"},
		VMConfig: VMConfig{
			BaseConfig: BaseConfig{
				VCPUs:      1,
				Memory:     1,
				Coschedule: -1,
				Colocate:   "a",
			},
		},
	}
	queue := []*QueuedVMs{a, b}

	hosts := func() []*HostStats { return fakeHostData(2, true) }

	// create error
	a.Coschedule = 0
	if _, err := schedule(queue, hosts(), cpuCommit); err == nil {
		t.Error("expected error, conflicting coschedule limits -- a limit is 0")
	} else {
		t.Log(err)
	}
	// undo error, make sure it goes away
	a.Coschedule = -1
	if _, err := schedule(queue, hosts(), cpuCommit); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// create error
	b.Coschedule = 0
	if _, err := schedule(queue, hosts(), cpuCommit); err == nil {
		t.Error("expected error, conflicting coschedule limits -- b limit is 0")
	} else {
		t.Log(err)
	}
	// undo error, make sure it goes away
	b.Coschedule = -1
	if _, err := schedule(queue, hosts(), cpuCommit); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// create error
	b.Colocate = "c"
	if _, err := schedule(queue, hosts(), cpuCommit); err == nil {
		t.Error("expected error, nonexistent colocate VM")
	} else {
		t.Log(err)
	}
	// undo error, make sure it goes away
	b.Colocate = "a"
	if _, err := schedule(queue, hosts(), cpuCommit); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func BenchmarkSchedule(b *testing.B) {
	var names []string
	for i := 0; i < 10000; i++ {
		names = append(names, strconv.Itoa(i))
	}

	queue := []*QueuedVMs{
		&QueuedVMs{
			Names: names,
			VMConfig: VMConfig{
				BaseConfig: BaseConfig{
					VCPUs:      1,
					Memory:     1,
					Coschedule: -1,
				},
			},
		},
	}

	hosts := fakeHostData(100, false)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		schedule(queue, hosts, cpuCommit)
	}
}
