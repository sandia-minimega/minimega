// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeOVS records ovs-vsctl invocations. locked counts the invocations that
// ran while bridgeLock was held.
type fakeOVS struct {
	mu     sync.Mutex
	calls  [][]string
	failed []bool
	locked int
	fail   func(args []string) error
}

func (f *fakeOVS) run(args []string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, append([]string(nil), args...))
	if bridgeLock.TryLock() {
		bridgeLock.Unlock()
	} else {
		f.locked++
	}

	var err error
	if f.fail != nil {
		err = f.fail(args)
	}
	f.failed = append(f.failed, err != nil)

	return "", err
}

// portTaps returns the taps of the add-port clauses in args.
func portTaps(args []string) []string {
	var taps []string
	for i, a := range args {
		if a == "add-port" && i+2 < len(args) {
			taps = append(taps, args[i+2])
		}
	}
	return taps
}

func hasTap(args []string, tap string) bool {
	for _, t := range portTaps(args) {
		if t == tap {
			return true
		}
	}
	return false
}

// addAll adds the taps concurrently and returns each tap's result.
func addAll(o *ovsBatcher, taps []string) map[string]error {
	var mu sync.Mutex
	var wg sync.WaitGroup
	res := map[string]error{}

	for _, tap := range taps {
		wg.Add(1)
		go func(tap string) {
			defer wg.Done()
			err := o.add("br", tap, 0, false)
			mu.Lock()
			res[tap] = err
			mu.Unlock()
		}(tap)
	}
	wg.Wait()

	return res
}

func newReq(tap string) *ovsPortReq {
	return &ovsPortReq{bridge: "br", tap: tap, res: make(chan error, 1)}
}

// gatedOVS returns a fakeOVS whose first invocation closes entered and then
// waits until release is closed.
func gatedOVS() (f *fakeOVS, entered, release chan struct{}) {
	entered, release = make(chan struct{}), make(chan struct{})
	first := true
	f = &fakeOVS{fail: func([]string) error {
		if first {
			first = false
			close(entered)
			<-release
		}
		return nil
	}}
	return f, entered, release
}

// waitQueued waits until the queue of o holds n requests.
func waitQueued(t *testing.T, o *ovsBatcher, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for len(o.queue) < n {
		if time.Now().After(deadline) {
			t.Fatalf("queue length = %d, want %d", len(o.queue), n)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestOVSBatcherGroups(t *testing.T) {
	f := &fakeOVS{}
	o := newOVSBatcher(f.run, 256)

	var taps []string
	for i := 0; i < 600; i++ {
		taps = append(taps, fmt.Sprintf("t%d", i))
	}

	for tap, err := range addAll(o, taps) {
		if err != nil {
			t.Errorf("add %v: %v", tap, err)
		}
	}

	count := map[string]int{}
	for i, c := range f.calls {
		if n := len(portTaps(c)); n > 256 {
			t.Errorf("batch %d has %d ports, want at most 256", i, n)
		}
		for _, tap := range portTaps(c) {
			count[tap]++
		}
	}
	for _, tap := range taps {
		if count[tap] != 1 {
			t.Errorf("tap %v added %d times, want 1", tap, count[tap])
		}
	}
}

func TestOVSBatcherMax(t *testing.T) {
	o := newOVSBatcher(nil, 256)
	o.queue = make(chan *ovsPortReq, 300)
	for i := 0; i < 299; i++ {
		o.queue <- newReq(fmt.Sprintf("t%d", i))
	}

	if n := len(o.collect(newReq("first"))); n != 256 {
		t.Errorf("first batch = %d requests, want 256", n)
	}
	if n := len(o.collect(<-o.queue)); n != 44 {
		t.Errorf("second batch = %d requests, want 44", n)
	}
	if n := len(o.queue); n != 0 {
		t.Errorf("queue length = %d, want 0", n)
	}
}

func TestOVSBatcherNoWait(t *testing.T) {
	var mu sync.Mutex
	var ran time.Time
	o := newOVSBatcher(func([]string) (string, error) {
		mu.Lock()
		ran = time.Now()
		mu.Unlock()
		return "", nil
	}, 256)

	start := time.Now()
	if err := o.add("br", "t0", 0, false); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if d := ran.Sub(start); ran.IsZero() || d > 40*time.Millisecond {
		t.Errorf("commit started %v after the add, want no wait", d)
	}
}

func TestOVSBatcherNextBatch(t *testing.T) {
	f, entered, release := gatedOVS()
	o := newOVSBatcher(f.run, 256)

	first := make(chan error, 1)
	go func() { first <- o.add("br", "g", 0, false) }()
	<-entered

	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = o.add("br", fmt.Sprintf("t%d", i), i*5, i == 2)
		}(i)
	}
	waitQueued(t, o, 3)
	close(release)
	wg.Wait()

	if err := <-first; err != nil {
		t.Errorf("add g: %v", err)
	}
	for i, err := range errs {
		if err != nil {
			t.Errorf("add t%d: %v", i, err)
		}
	}
	if len(f.calls) != 2 {
		t.Fatalf("invocations = %d, want 2", len(f.calls))
	}
	if got := portTaps(f.calls[0]); !reflect.DeepEqual(got, []string{"g"}) {
		t.Errorf("first batch = %q, want [g]", got)
	}

	args := strings.Join(f.calls[1], " ")
	for _, want := range []string{
		"-- add-port br t0",
		"-- add-port br t1 tag=5",
		"-- add-port br t2 tag=10 -- set Interface t2 type=internal",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args %q do not contain %q", args, want)
		}
	}
	if strings.Contains(args, "t0 tag") {
		t.Errorf("args %q tag the untagged port", args)
	}
}

func TestOVSBatcherSplit(t *testing.T) {
	f := &fakeOVS{fail: func(args []string) error {
		if hasTap(args, "bad") {
			return errors.New("ovs cmd failed")
		}
		return nil
	}}
	o := newOVSBatcher(f.run, 256)

	taps := []string{"bad"}
	for i := 0; i < 7; i++ {
		taps = append(taps, fmt.Sprintf("t%d", i))
	}

	var reqs []*ovsPortReq
	for _, tap := range taps {
		reqs = append(reqs, newReq(tap))
	}
	o.flush(reqs)

	for i, r := range reqs {
		err := <-r.res
		if i == 0 {
			if err == nil || !strings.Contains(err.Error(), "add port failed") {
				t.Errorf("add bad = %v, want an add port error", err)
			}
		} else if err != nil {
			t.Errorf("add %v: %v", taps[i], err)
		}
	}

	// 1 for all 8, then 2 at each of the 3 levels of halving.
	if len(f.calls) != 7 {
		t.Errorf("invocations = %d, want 7", len(f.calls))
	}

	ok := map[string]int{}
	for i, c := range f.calls {
		if f.failed[i] {
			continue
		}
		for _, tap := range portTaps(c) {
			ok[tap]++
		}
	}
	for _, tap := range taps[1:] {
		if ok[tap] != 1 {
			t.Errorf("tap %v committed %d times, want 1", tap, ok[tap])
		}
	}
	if ok["bad"] != 0 {
		t.Error("bad tap committed")
	}
}

func TestOVSBatcherDuplicate(t *testing.T) {
	f := &fakeOVS{}
	o := newOVSBatcher(f.run, 256)

	reqs := []*ovsPortReq{newReq("a"), newReq("b"), newReq("a")}
	o.flush(reqs)

	if len(f.calls) != 1 {
		t.Fatalf("invocations = %d, want 1", len(f.calls))
	}
	if got := portTaps(f.calls[0]); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("taps = %q, want [a b]", got)
	}

	for i, r := range reqs[:2] {
		if err := <-r.res; err != nil {
			t.Errorf("request %d: %v", i, err)
		}
	}

	err := <-reqs[2].res
	if err == nil || err == errAlreadyExists || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("duplicate = %v, want an already exists error that is not errAlreadyExists", err)
	}
}

func TestOVSBatcherAlreadyExists(t *testing.T) {
	f := &fakeOVS{fail: func(args []string) error {
		if hasTap(args, "x") {
			return errAlreadyExists
		}
		return nil
	}}
	o := newOVSBatcher(f.run, 256)

	if err := o.add("br", "x", 0, false); err != errAlreadyExists {
		t.Errorf("single add = %v, want errAlreadyExists", err)
	}

	f.calls = nil
	reqs := []*ovsPortReq{newReq("x"), newReq("y")}
	o.flush(reqs)

	if err := <-reqs[0].res; err != errAlreadyExists {
		t.Errorf("x in a batch = %v, want errAlreadyExists", err)
	}
	if err := <-reqs[1].res; err != nil {
		t.Errorf("y in a batch: %v", err)
	}
	if len(f.calls) != 3 {
		t.Errorf("invocations = %d, want 3", len(f.calls))
	}
}

func TestCreateNetnsTapBatched(t *testing.T) {
	add := []string{"--", "add-port", "mega_bridge", "mega_tap7", "tag=101"}
	del := []string{"del-port", "mega_bridge", "mega_tap7"}

	for _, tc := range []struct {
		name   string
		exists bool
		want   [][]string
	}{
		{"new", false, [][]string{add}},
		{"exists", true, [][]string{add, del, add}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := &fakeNetns{}
			n.install(t)
			OVSBatching = true

			first := true
			f := &fakeOVS{fail: func(args []string) error {
				if tc.exists && first && hasTap(args, "mega_tap7") {
					first = false
					return errAlreadyExists
				}
				return nil
			}}
			old := ovsCmdWrapper
			ovsCmdWrapper = f.run
			t.Cleanup(func() { ovsCmdWrapper = old })

			b := netnsTestBridge()
			tap, mtu, err := b.CreateNetnsTap(&Netns{}, "00:11:22:33:44:55", 101, 0, 200007)
			if err != nil {
				t.Fatalf("CreateNetnsTap: %v", err)
			}
			if tap != "mega_tap7" || mtu != 9000 {
				t.Errorf("CreateNetnsTap() = %q, %d, want mega_tap7, 9000", tap, mtu)
			}

			if !reflect.DeepEqual(f.calls, tc.want) {
				t.Errorf("ovs calls =\n%q\nwant\n%q", f.calls, tc.want)
			}
			if f.locked != 0 {
				t.Errorf("%d ovs calls ran while bridgeLock was held", f.locked)
			}
			if n.addCalls != 0 {
				t.Errorf("netnsAddTap calls = %d, want 0", n.addCalls)
			}
			if got := b.taps["mega_tap7"]; got == nil || got.VLAN != 101 || got.MAC != "00:11:22:33:44:55" || got.Host {
				t.Errorf("tap table entry = %+v", got)
			}
		})
	}
}
