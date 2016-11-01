// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package main

import (
	"math/rand"
	"strconv"
	"testing"
)

func fakeHostData(N int) []*HostStats {
	res := []*HostStats{}

	for i := 0; i < N; i++ {
		res = append(res, &HostStats{
			Name:      strconv.Itoa(i),
			CPUCommit: i,
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

// TestHostSort sorts fakeHostData by CPUCommit and then updates the root and
// call Update many times to see if we keep getting the correct minimum.
func TestHostSort(t *testing.T) {
	N := 100
	hosts := fakeHostData(N)

	hostSortBy(cpuCommit).Sort(hosts)

	for i := 0; i < 10*N; i++ {
		v, _ := strconv.Atoi(hosts[0].Name)
		if i%N != v {
			t.Errorf("incorrect minimum by cpu commit: %v != %v", i, v)
		}

		hosts[0].CPUCommit += N
		hostSortBy(cpuCommit).Update(hosts, hosts[0].Name)
	}
}

func TestQueuedVMsLess(t *testing.T) {
	// q < q2
	q := QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q.ScheduleHost = "foo"
	q.SchedulePeers = "1"

	q2 := QueuedVMs{
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
	q = QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q.SchedulePeers = "1"
	q2 = QueuedVMs{
		Names: []string{"a", "b"},
	}

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}

	// q < q2
	q = QueuedVMs{
		Names: []string{"a", "b", "c"},
	}
	q2 = QueuedVMs{
		Names: []string{"a", "b"},
	}

	if !q.Less(q2) {
		t.Errorf("%v < %v", q2, q)
	}

	if q2.Less(q) {
		t.Errorf("%v < %v", q2, q)
	}
}
