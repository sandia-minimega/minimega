// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build linux

package bridge

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

// fakeNetns records the netns tap steps. locked counts the steps that ran
// while bridgeLock was held, and addOK is set when netnsAddTap ran under it.
type fakeNetns struct {
	steps    []string
	locked   int
	fail     string
	addCalls int
	addErr   error
	addOK    bool
}

func (f *fakeNetns) step(s string) error {
	f.steps = append(f.steps, s)
	if bridgeLock.TryLock() {
		bridgeLock.Unlock()
	} else {
		f.locked++
	}
	if f.fail != "" && strings.HasPrefix(s, f.fail) {
		return errors.New(f.fail + " failed")
	}
	return nil
}

func (f *fakeNetns) install(t *testing.T) {
	t.Helper()

	oldVeth, oldHost, oldTap, oldDel := netnsAddVeth, netnsSetupHost, netnsSetupTap, netnsDelLink
	oldAdd, oldBatch := netnsAddTap, OVSBatching
	t.Cleanup(func() {
		netnsAddVeth, netnsSetupHost, netnsSetupTap, netnsDelLink = oldVeth, oldHost, oldTap, oldDel
		netnsAddTap, OVSBatching = oldAdd, oldBatch
	})
	OVSBatching = false

	netnsAddVeth = func(tap, peer string, ns *Netns) error {
		return f.step(fmt.Sprintf("veth %v %v", tap, peer))
	}
	netnsSetupHost = func(tap string) (int, error) {
		return 9000, f.step("host " + tap)
	}
	netnsSetupTap = func(ns *Netns, index, uid, mtu int) error {
		return f.step(fmt.Sprintf("tap %d %d %d", index, uid, mtu))
	}
	netnsDelLink = func(name string) error {
		return f.step("del " + name)
	}

	netnsAddTap = func(b *Bridge, tap, mac string, vlan int) error {
		f.addCalls++
		if bridgeLock.TryLock() {
			bridgeLock.Unlock()
		} else {
			f.addOK = true
		}
		if f.addErr != nil {
			return f.addErr
		}
		b.taps[tap] = &Tap{Name: tap, Bridge: b.Name, VLAN: vlan, MAC: mac}
		return nil
	}
}

func netnsTestBridge() *Bridge {
	tapChan := make(chan string, 1)
	tapChan <- "mega_tap7"

	return &Bridge{
		Name:    "mega_bridge",
		taps:    map[string]*Tap{},
		tapChan: tapChan,
	}
}

func TestCreateNetnsTap(t *testing.T) {
	f := &fakeNetns{}
	f.install(t)
	b := netnsTestBridge()

	tap, mtu, err := b.CreateNetnsTap(&Netns{}, "00:11:22:33:44:55", 101, 0, 200007)
	if err != nil {
		t.Fatalf("CreateNetnsTap: %v", err)
	}
	if tap != "mega_tap7" || mtu != 9000 {
		t.Errorf("CreateNetnsTap() = %q, %d, want mega_tap7, 9000", tap, mtu)
	}

	want := []string{"veth mega_tap7 vm0", "host mega_tap7", "tap 0 200007 9000"}
	if !reflect.DeepEqual(f.steps, want) {
		t.Errorf("steps = %q, want %q", f.steps, want)
	}
	if f.locked != 0 {
		t.Errorf("%d steps ran while bridgeLock was held", f.locked)
	}
	if f.addCalls != 1 || !f.addOK {
		t.Errorf("netnsAddTap calls = %d, under bridgeLock = %v, want 1 call under the lock", f.addCalls, f.addOK)
	}
	if got := b.taps["mega_tap7"]; got == nil || got.VLAN != 101 || got.MAC != "00:11:22:33:44:55" {
		t.Errorf("tap table entry = %+v", got)
	}
}

func TestCreateNetnsTapCleanup(t *testing.T) {
	for _, tc := range []struct {
		name    string
		fail    string
		addErr  error
		wantAdd int
	}{
		{"veth", "veth", nil, 0},
		{"host", "host", nil, 0},
		{"tap", "tap", nil, 0},
		{"ovs", "", errors.New("add port failed"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeNetns{fail: tc.fail, addErr: tc.addErr}
			f.install(t)
			b := netnsTestBridge()

			tap, _, err := b.CreateNetnsTap(&Netns{}, "00:11:22:33:44:55", 101, 0, 200007)
			if err == nil {
				t.Fatalf("CreateNetnsTap() = %q, want an error", tap)
			}
			if f.addCalls != tc.wantAdd {
				t.Errorf("netnsAddTap calls = %d, want %d", f.addCalls, tc.wantAdd)
			}

			last := f.steps[len(f.steps)-1]
			if tc.name == "veth" {
				if len(f.steps) != 1 {
					t.Errorf("steps after a failed veth add: %q", f.steps)
				}
				return
			}
			if last != "del mega_tap7" {
				t.Errorf("last step = %q, want del mega_tap7", last)
			}
			if len(b.taps) != 0 {
				t.Errorf("tap table = %v, want empty", b.taps)
			}
		})
	}
}

func TestDestroyNetnsTap(t *testing.T) {
	f := &fakeNetns{}
	f.install(t)
	b := netnsTestBridge()

	if err := b.DestroyNetnsTap("mega_tap7"); err == nil {
		t.Error("unknown tap: no error")
	}

	b.taps["mega_tap7"] = &Tap{Name: "mega_tap7", Bridge: b.Name}
	if err := b.DestroyNetnsTap("mega_tap7"); err != nil {
		t.Fatal(err)
	}
	if !b.taps["mega_tap7"].Defunct {
		t.Error("tap is not defunct")
	}
	if want := []string{"del mega_tap7"}; !reflect.DeepEqual(f.steps, want) || f.locked != 0 {
		t.Errorf("steps = %q (%d under bridgeLock), want %q", f.steps, f.locked, want)
	}
}

// fakeSetns replaces the namespace syscalls. fail is the setns call (from 1)
// that fails.
type fakeSetns struct {
	fds      []int
	unshared int
	fail     int
}

func (s *fakeSetns) install(t *testing.T) {
	t.Helper()

	oldUnshare, oldSetns := netnsUnshare, netnsSetns
	t.Cleanup(func() { netnsUnshare, netnsSetns = oldUnshare, oldSetns })

	netnsUnshare = func() error {
		s.unshared++
		return nil
	}
	netnsSetns = func(fd int) error {
		s.fds = append(s.fds, fd)
		if len(s.fds) == s.fail {
			return unix.EPERM
		}
		return nil
	}
}

func TestNewNetns(t *testing.T) {
	s := &fakeSetns{}
	s.install(t)

	ns, err := NewNetns()
	if err != nil {
		t.Fatal(err)
	}
	if s.unshared != 1 || len(s.fds) != 1 || s.fds[0] == ns.fd {
		t.Errorf("unshare calls %d, setns fds %v, netns fd %d", s.unshared, s.fds, ns.fd)
	}

	if want := fmt.Sprintf("/proc/%d/fd/%d", os.Getpid(), ns.fd); ns.Path() != want {
		t.Errorf("Path() = %q, want %q", ns.Path(), want)
	}
	if _, err := os.Stat(ns.Path()); err != nil {
		t.Errorf("Path(): %v", err)
	}

	if err := ns.Close(); err != nil {
		t.Fatal(err)
	}
	if err := ns.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if ns.Path() != "" {
		t.Errorf("Path() after Close = %q", ns.Path())
	}
	if (*Netns)(nil).Path() != "" || (*Netns)(nil).Close() != nil {
		t.Error("nil Netns")
	}
}

func TestNewNetnsErrors(t *testing.T) {
	s := &fakeSetns{fail: 1}
	s.install(t)

	if ns, err := NewNetns(); err == nil || ns != nil || !strings.Contains(err.Error(), "restore") {
		t.Errorf("failed restore: NewNetns() = %v, %v", ns, err)
	}

	netnsUnshare = func() error { return unix.EPERM }
	s.fds = nil
	if ns, err := NewNetns(); !errors.Is(err, unix.EPERM) || ns != nil || len(s.fds) != 0 {
		t.Errorf("failed unshare: NewNetns() = %v, %v; setns calls %v", ns, err, s.fds)
	}
}

func TestNetnsDo(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ns := &Netns{f: f, fd: int(f.Fd())}

	for _, tc := range []struct {
		name    string
		fail    int
		fnErr   error
		wantFn  bool
		wantErr string
	}{
		{"ok", 0, nil, true, ""},
		{"fn error", 0, errors.New("fn failed"), true, "fn failed"},
		{"enter", 1, nil, false, "enter netns"},
		{"leave", 2, errors.New("fn failed"), true, "leave netns"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &fakeSetns{fail: tc.fail}
			s.install(t)

			var ran bool
			err := ns.Do(func() error {
				ran = true
				if len(s.fds) != 1 || s.fds[0] != ns.fd {
					t.Errorf("fn ran after setns %v, want [%d]", s.fds, ns.fd)
				}
				return tc.fnErr
			})

			if ran != tc.wantFn {
				t.Errorf("fn ran = %v, want %v", ran, tc.wantFn)
			}
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Errorf("Do() = %v, want %q", err, tc.wantErr)
			}
			if tc.wantFn && (len(s.fds) != 2 || s.fds[1] == ns.fd) {
				t.Errorf("setns fds = %v, want the netns, then the original", s.fds)
			}
		})
	}

	if err := (&Netns{}).Do(func() error { return nil }); err == nil {
		t.Error("closed Netns: no error")
	}
}
