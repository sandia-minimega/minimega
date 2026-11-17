// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLaunchedCount(t *testing.T) {
	vmErr := errors.New("vm failed")
	batchErr := launchBatchError{errors.New("no file")}

	for _, tt := range []struct {
		total int
		errs  []error
		want  int
	}{
		{10, nil, 10},
		{10, []error{vmErr, vmErr}, 8},
		{10, []error{batchErr}, 0},
		{10, []error{vmErr, fmt.Errorf("wrapped: %w", batchErr)}, 0},
		{1, []error{vmErr, vmErr}, 0},
	} {
		if got := launchedCount(tt.total, tt.errs); got != tt.want {
			t.Errorf("launchedCount(%d, %v) = %d, want %d", tt.total, tt.errs, got, tt.want)
		}
	}
}

// TestScheduleParallel checks that each host has at most the limit of
// launches in progress, that the hosts launch at the same time, and that the
// stats count VMs.
func TestScheduleParallel(t *testing.T) {
	oldParallel, oldLaunch := scheduleHostParallel, scheduleLaunch
	t.Cleanup(func() {
		scheduleHostParallel = oldParallel
		scheduleLaunch = oldLaunch
	})
	scheduleHostParallel = func() int { return 3 }

	var mu sync.Mutex
	running, most := map[string]int{}, map[string]int{}

	scheduleLaunch = func(n *Namespace, host string, q *QueuedVMs) scheduleResult {
		mu.Lock()
		running[host]++
		most[host] = max(most[host], running[host])
		mu.Unlock()

		time.Sleep(20 * time.Millisecond)

		mu.Lock()
		running[host]--
		mu.Unlock()

		// the VMs with names that start with "bad" fail
		res := scheduleResult{host: host}
		for _, name := range q.Names {
			if strings.HasPrefix(name, "bad") {
				res.failed++
			} else {
				res.launched++
			}
		}
		if res.failed > 0 {
			res.err = "bad VM"
		}
		return res
	}

	assignment := map[string][]*QueuedVMs{}
	for _, host := range []string{"h1", "h2"} {
		for i := 0; i < 10; i++ {
			assignment[host] = append(assignment[host], &QueuedVMs{
				Names: []string{fmt.Sprintf("%v-a%d", host, i), fmt.Sprintf("%v-b%d", host, i)},
			})
		}
	}
	assignment["h1"][0].Names = append(assignment["h1"][0].Names, "bad1")

	n := &Namespace{Name: "test"}
	n.queue = []*QueuedVMs{{Names: make([]string, 41)}}

	if err := n.schedule(assignment, n.queue); err != nil {
		t.Fatal(err)
	}

	for _, host := range []string{"h1", "h2"} {
		if most[host] != 3 {
			t.Errorf("host %v had at most %d launches in progress, want 3", host, most[host])
		}
	}

	stats := n.scheduleStats[len(n.scheduleStats)-1]
	if stats.launched != 40 || stats.failures != 1 || stats.total != 41 {
		t.Errorf("launched %d, failures %d, total %d, want 40, 1, 41", stats.launched, stats.failures, stats.total)
	}
}

func TestMeshageVMResponseWait(t *testing.T) {
	tid1, wait1 := meshageVMResponseWait()
	tid2, wait2 := meshageVMResponseWait()
	defer meshageVMResponseDone(tid1)
	defer meshageVMResponseDone(tid2)

	if tid1 == tid2 {
		t.Fatalf("two waits got TID %d", tid1)
	}

	meshageVMResponseDeliver(meshageVMResponse{TID: tid2, Launched: 2})
	meshageVMResponseDeliver(meshageVMResponse{TID: tid1, Launched: 1})
	// a response that no launch waits for is dropped
	meshageVMResponseDeliver(meshageVMResponse{TID: tid1, Launched: 99})

	for _, tt := range []struct {
		wait <-chan meshageVMResponse
		want int
	}{{wait1, 1}, {wait2, 2}} {
		select {
		case resp := <-tt.wait:
			if resp.Launched != tt.want {
				t.Errorf("got response %d, want %d", resp.Launched, tt.want)
			}
		case <-time.After(time.Second):
			t.Fatalf("no response %d", tt.want)
		}
	}

	meshageVMResponsesLock.Lock()
	_, ok1 := meshageVMResponses[tid1]
	meshageVMResponsesLock.Unlock()
	if ok1 {
		t.Error("the wait for a delivered response is still registered")
	}
}

func TestVMConflicts(t *testing.T) {
	vm := &KvmVM{BaseVM: &BaseVM{Name: "taken", BaseConfig: BaseConfig{UUID: "u1"}}}
	vms := []VM{vm}

	for _, tt := range []struct {
		q    *QueuedVMs
		want string
	}{
		{&QueuedVMs{Names: []string{"free", ""}}, ""},
		{&QueuedVMs{Names: []string{"free", "taken"}}, "name `taken`"},
		{&QueuedVMs{Names: []string{"free"}, VMConfig: VMConfig{BaseConfig: BaseConfig{UUID: "u1"}}}, "UUID `u1`"},
	} {
		err := vmConflicts(vms, []*QueuedVMs{tt.q})
		if tt.want == "" && err != nil {
			t.Errorf("%v: %v, want no error", tt.q.Names, err)
		}
		if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
			t.Errorf("%v: %v, want an error with %q", tt.q.Names, err, tt.want)
		}
	}
}

// TestQueueIndex checks that queued names conflict with each other, and that
// the index starts again with an empty queue.
func TestQueueIndex(t *testing.T) {
	n := &Namespace{Name: "test", QueueVMs: true}

	if err := n.Queue("a[1-3]", KVM, VMConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := n.Queue("a2", KVM, VMConfig{}); err == nil || !strings.Contains(err.Error(), "already queued") {
		t.Errorf("Queue of a queued name = %v, want already queued", err)
	}
	if err := n.Queue("b1", KVM, VMConfig{}); err != nil {
		t.Errorf("Queue of a new name = %v", err)
	}

	// what schedule and `ns flush` do
	n.clearQueue()

	if err := n.Queue("a2", KVM, VMConfig{}); err != nil {
		t.Errorf("Queue after the queue was cleared = %v", err)
	}
}
