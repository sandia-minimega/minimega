// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
)

// fcHolders counts the holders of one stage at the same time.
type fcHolders struct {
	cur, max atomic.Int32
}

func (h *fcHolders) enter() {
	n := h.cur.Add(1)
	for {
		m := h.max.Load()
		if n <= m || h.max.CompareAndSwap(m, n) {
			return
		}
	}
}

func (h *fcHolders) leave() {
	h.cur.Add(-1)
}

func (h *fcHolders) hold(d time.Duration) {
	h.enter()
	time.Sleep(d)
	h.leave()
}

// fillFCStage takes the free permits of s and returns how many it took.
func fillFCStage(s *fcStage) int {
	n := 0
	for {
		select {
		case s.ch <- struct{}{}:
			n++
		default:
			return n
		}
	}
}

func setFCStages(t *testing.T, st *fcLaunchStages) {
	t.Helper()

	old := fcStages
	fcStages = st
	t.Cleanup(func() { fcStages = old })
}

func setFCStartVMM(t *testing.T, fn func(*FirecrackerVM, VMConfig, string, string) error) {
	t.Helper()

	old := fcStartVMMFn
	fcStartVMMFn = fn
	t.Cleanup(func() { fcStartVMMFn = old })
}

// launchFCVMs launches n VMs at the same time and returns their Launch
// errors. It fails the test when a launch does not return.
func launchFCVMs(t *testing.T, f *fcDriverFakes, n int) ([]*FirecrackerVM, []error) {
	t.Helper()

	cfg := f.config(t)
	vms := make([]*FirecrackerVM, n)
	for i := range vms {
		vms[i] = f.newVM(t, cfg)
	}

	errs := make([]error, n)
	var wg sync.WaitGroup
	for i, vm := range vms {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = vm.Launch()
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Launch() did not return: a stage permit was not released")
	}

	t.Cleanup(func() {
		for _, vm := range vms {
			if err := vm.Flush(); err != nil {
				t.Errorf("Flush() = %v", err)
			}
		}
	})

	return vms, errs
}

func TestFCLaunchStageLimits(t *testing.T) {
	f := setupFCDriver(t)
	setFCStages(t, newFCLaunchStages(2))

	var disk, network, vmm, inflight fcHolders

	fcPrepareDiskFn = func(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (fcDisk, error) {
		inflight.enter()
		disk.hold(5 * time.Millisecond)
		return fcDisk{HostPath: filepath.Join(instanceDir, "disk-0.raw"), CacheType: "Writeback"}, nil
	}
	fcCreateNetns = func() (*bridge.Netns, error) {
		network.hold(5 * time.Millisecond)
		return new(bridge.Netns), nil
	}
	fcCreateTap = func(nic NetConfig, ns *bridge.Netns, index, uid int) (string, int, error) {
		network.hold(5 * time.Millisecond)
		return fmt.Sprintf("mega_tap%d", index), 1500, nil
	}
	setFCStartVMM(t, func(*FirecrackerVM, VMConfig, string, string) error {
		vmm.hold(30 * time.Millisecond)
		inflight.leave()
		return nil
	})

	vms, errs := launchFCVMs(t, f, 20)
	for i, err := range errs {
		if err != nil {
			t.Errorf("Launch() of VM %d = %v", i, err)
		}
	}

	for _, c := range []struct {
		name  string
		h     *fcHolders
		limit int32
	}{
		{"disk", &disk, 2},
		{"network", &network, 2},
		{"vmm", &vmm, 4},
		{"in-flight", &inflight, 8},
	} {
		if got := c.h.max.Load(); got < 1 || got > c.limit {
			t.Errorf("%v holders at the same time = %d, want 1 to %d", c.name, got, c.limit)
		}
	}

	for _, vm := range vms {
		got := vm.launchTimes()
		if !got.Disk.Entered || !got.Network.Entered || !got.VMM.Entered || got.VMM.Run < 30*time.Millisecond {
			t.Errorf("vm %v launch times = %v, want all stages and a vmm run of 30ms or more", vm.ID, got)
		}
	}
}

func TestFCLaunchStageFailures(t *testing.T) {
	for _, stage := range []string{"disk", "network", "bridge", "vmm"} {
		t.Run(stage, func(t *testing.T) {
			f := setupFCDriver(t)
			st := newFCLaunchStages(1)
			setFCStages(t, st)

			fail := errors.New(stage + " failed")
			var taps atomic.Int32

			fcPrepareDiskFn = func(instanceDir string, index int, dc DiskConfig, snapshot bool, vmID, uid int) (fcDisk, error) {
				if stage == "disk" {
					return fcDisk{}, fail
				}
				return fcDisk{HostPath: filepath.Join(instanceDir, "disk-0.raw"), CacheType: "Writeback"}, nil
			}
			fcCreateNetns = func() (*bridge.Netns, error) {
				if stage == "network" {
					return nil, fail
				}
				return new(bridge.Netns), nil
			}
			fcCreateTap = func(nic NetConfig, ns *bridge.Netns, index, uid int) (string, int, error) {
				tap := fmt.Sprintf("mega_tap%d", taps.Add(1))
				if stage == "bridge" {
					done := make(chan error, 1)
					done <- fail
					fcTapAdds.Lock()
					fcTapAdds.m[tap] = done
					fcTapAdds.Unlock()
				}
				return tap, 1500, nil
			}
			setFCStartVMM(t, func(*FirecrackerVM, VMConfig, string, string) error {
				if stage == "vmm" {
					return fail
				}
				return nil
			})

			// More launches than the in-flight bound and each stage limit.
			vms, errs := launchFCVMs(t, f, 6)
			for i, err := range errs {
				if err == nil || !strings.Contains(err.Error(), fail.Error()) {
					t.Errorf("Launch() of VM %d = %v, want %q", i, err, fail)
				}
			}

			for _, s := range []struct {
				name string
				s    *fcStage
			}{
				{"in-flight", st.inflight},
				{"disk", st.disk},
				{"network", st.network},
				{"vmm", st.vmm},
			} {
				if n := len(s.s.ch); n != 0 {
					t.Errorf("%v stage holds %d permits after the failed launches", s.name, n)
				}
			}

			for _, vm := range vms {
				if got := vm.GetState(); got != VM_ERROR {
					t.Errorf("vm %v state = %v, want %v", vm.ID, got, VM_ERROR)
				}
				if tap := vm.Networks[0].Tap; stage == "bridge" && tap != "" {
					t.Errorf("vm %v has tap %v after its bridge port add failed", vm.ID, tap)
				}
			}
		})
	}
}

func TestFCRelaunchUsesOnlyVMMStage(t *testing.T) {
	f := setupFCDriver(t)
	setFCStages(t, newFCLaunchStages(1))
	vm := f.newVM(t, f.config(t))

	err := vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	waitFCState(t, vm, VM_BUILDING)

	if got := vm.launchTimes(); !got.Disk.Entered || !got.Network.Entered || !got.VMM.Entered {
		t.Errorf("first launch times = %v, want all stages", got)
	}

	if err := vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, vm, VM_QUIT)

	// A relaunch that waits for the in-flight bound, the disk stage, or the
	// network stage blocks.
	st := newFCLaunchStages(1)
	for _, s := range []*fcStage{st.inflight, st.disk, st.network} {
		fillFCStage(s)
	}
	setFCStages(t, st)

	done := make(chan error, 1)
	go func() { done <- vm.Start() }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() from QUIT = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start() from QUIT waits for a stage other than vmm")
	}
	cleanupFCVM(t, vm)
	waitFCState(t, vm, VM_RUNNING)

	got := vm.launchTimes()
	if got.Admit != 0 || got.Disk.Entered || got.Network.Entered || !got.VMM.Entered {
		t.Errorf("relaunch times = %v, want only the vmm stage", got)
	}
	if n := len(st.vmm.ch); n != 0 {
		t.Errorf("vmm stage holds %d permits after the relaunch", n)
	}
}

func TestFCLaunchStageSizes(t *testing.T) {
	// The limits of the in-flight bound, disk, network, and vmm.
	for p, want := range map[int][4]int{
		0:  {4, 1, 1, 2},
		2:  {8, 2, 2, 4},
		8:  {32, 8, 8, 16},
		96: {384, 96, 8, 192},
	} {
		st := newFCLaunchStages(p)
		got := [4]int{fillFCStage(st.inflight), fillFCStage(st.disk), fillFCStage(st.network), fillFCStage(st.vmm)}
		if got != want {
			t.Errorf("newFCLaunchStages(%d) limits = %v, want %v", p, got, want)
		}
	}
}

func TestFCStageRun(t *testing.T) {
	s := newFCStage(1)
	fail := errors.New("step failed")

	var st fcStageTime
	if err := s.run(&st, func() error {
		time.Sleep(2 * time.Millisecond)
		return fail
	}); err != fail {
		t.Errorf("run() = %v, want %v", err, fail)
	}
	if !st.Entered || st.Run < 2*time.Millisecond || len(s.ch) != 0 {
		t.Errorf("run() times = %v, permits held %d, want a run of 2ms or more and no permits", st, len(s.ch))
	}

	var skipped fcStageTime
	s.acquire()
	if err := s.runIf(false, &skipped, func() error { return nil }); err != nil || skipped.Entered {
		t.Errorf("runIf(false) = %v, %v, want no error and no stage", err, skipped)
	}
	s.release()
}

func TestFCLaunchSummary(t *testing.T) {
	var times []fcLaunchTimes
	for i := 10; i >= 1; i-- {
		d := time.Duration(i)
		times = append(times, fcLaunchTimes{
			Admit:   d * 100 * time.Millisecond,
			Disk:    fcStageTime{Entered: true, Run: d * time.Millisecond},
			Network: fcStageTime{Entered: true, Run: d * 100 * time.Microsecond},
			VMM:     fcStageTime{Entered: true, Wait: d * 10 * time.Millisecond, Run: d * 10 * time.Millisecond},
		})
	}

	want := "minifire: 10 microVMs lit in 2.5s (disk p50/p95 5ms/10ms, network p50/p95 500µs/1ms, vmm p50/p95 50ms/100ms; waits p95 1.1s)"
	if got := fcLaunchSummary(times, 2500*time.Millisecond); got != want {
		t.Errorf("fcLaunchSummary(10) =\n%q\nwant\n%q", got, want)
	}

	want = "minifire: 1 microVM lit in 1.235s (disk p50/p95 1ms/1ms, network p50/p95 100µs/100µs, vmm p50/p95 10ms/10ms; waits p95 110ms)"
	if got := fcLaunchSummary(times[9:], 1234567*time.Microsecond); got != want {
		t.Errorf("fcLaunchSummary(1) =\n%q\nwant\n%q", got, want)
	}

	if times[0].Disk.Run != 10*time.Millisecond {
		t.Errorf("fcLaunchSummary changed its argument: %v", times[0])
	}
}

func TestFCLaunchBatch(t *testing.T) {
	var b fcLaunchBatch

	vm := &FirecrackerVM{BaseVM: &BaseVM{}}
	vm.times = fcLaunchTimes{Disk: fcStageTime{Entered: true, Run: 3 * time.Millisecond}}
	b.add(vm)
	b.add(&KvmVM{BaseVM: &BaseVM{}})

	if b.len() != 2 {
		t.Fatalf("len() = %d, want 2", b.len())
	}

	want := "minifire: 2 microVMs lit in 1s (disk p50/p95 0s/3ms, network p50/p95 0s/0s, vmm p50/p95 0s/0s; waits p95 0s)"
	if got := b.message(time.Second); got != want {
		t.Errorf("message() =\n%q\nwant\n%q", got, want)
	}
}

func TestFCPercentile(t *testing.T) {
	if got := fcPercentile(nil, 95); got != 0 {
		t.Errorf("fcPercentile(nil, 95) = %v, want 0", got)
	}

	var d []time.Duration
	for i := 100; i >= 1; i-- {
		d = append(d, time.Duration(i))
	}
	for p, want := range map[int]time.Duration{1: 1, 50: 50, 95: 95, 100: 100} {
		if got := fcPercentile(d, p); got != want {
			t.Errorf("fcPercentile(1..100, %d) = %v, want %v", p, got, want)
		}
	}

	if got := fcPercentile([]time.Duration{7}, 50); got != 7 {
		t.Errorf("fcPercentile([7], 50) = %v, want 7", got)
	}
}

func TestFCLaunchTimesString(t *testing.T) {
	lt := fcLaunchTimes{
		Admit:  456700 * time.Nanosecond,
		Disk:   fcStageTime{Entered: true, Wait: 1500 * time.Microsecond, Run: 2345678 * time.Microsecond},
		Bridge: 12345 * time.Microsecond,
	}

	want := "in-flight wait 457µs; disk wait 1.5ms run 2.346s; network -; bridge 12.3ms; vmm -"
	if got := lt.String(); got != want {
		t.Errorf("String() =\n%q\nwant\n%q", got, want)
	}
	if got := lt.waits(); got != 456700*time.Nanosecond+1500*time.Microsecond {
		t.Errorf("waits() = %v", got)
	}
}
