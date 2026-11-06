// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fakeNetns records commands. locked counts the commands that ran while
// bridgeLock was held, and addOK is set when netnsAddTap ran under it.
type fakeNetns struct {
	cmds     [][]string
	locked   int
	fail     string
	failOut  string
	addCalls int
	addErr   error
	addOK    bool
}

func (f *fakeNetns) install(t *testing.T) {
	t.Helper()

	oldRun, oldDir, oldSys, oldAdd := netnsRun, netnsDir, netnsSysNet, netnsAddTap
	t.Cleanup(func() {
		netnsRun, netnsDir, netnsSysNet, netnsAddTap = oldRun, oldDir, oldSys, oldAdd
	})

	netnsRun = func(args ...string) (string, error) {
		f.cmds = append(f.cmds, args)
		if bridgeLock.TryLock() {
			bridgeLock.Unlock()
		} else {
			f.locked++
		}
		if f.fail != "" && strings.Contains(strings.Join(args, " "), f.fail) {
			return f.failOut, errors.New("exit status 1")
		}
		return "", nil
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

	dir := t.TempDir()
	netnsDir = filepath.Join(dir, "netns")
	netnsSysNet = filepath.Join(dir, "net")
	if err := os.MkdirAll(filepath.Join(netnsSysNet, "mega_tap7"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netnsSysNet, "mega_tap7", "mtu"), []byte("9000\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func testBridge() *Bridge {
	tapChan := make(chan string, 1)
	tapChan <- "mega_tap7"

	return &Bridge{
		Name:    "mega_bridge",
		taps:    map[string]*Tap{},
		tapChan: tapChan,
	}
}

func TestNetnsTapCommands(t *testing.T) {
	want := [][]string{
		{"ip", "-n", "mm-fc-a1b2c3-7", "tuntap", "add", "dev", "fc1", "mode", "tap", "user", "200007", "group", "200007"},
		{"ip", "-n", "mm-fc-a1b2c3-7", "link", "set", "dev", "vm1", "mtu", "1450"},
		{"ip", "-n", "mm-fc-a1b2c3-7", "link", "set", "dev", "fc1", "mtu", "1450"},
		{"ip", "-n", "mm-fc-a1b2c3-7", "link", "set", "dev", "vm1", "up"},
		{"ip", "-n", "mm-fc-a1b2c3-7", "link", "set", "dev", "fc1", "up"},
		{"tc", "-n", "mm-fc-a1b2c3-7", "qdisc", "add", "dev", "vm1", "ingress"},
		{"tc", "-n", "mm-fc-a1b2c3-7", "filter", "add", "dev", "vm1", "parent", "ffff:", "matchall", "action", "mirred", "egress", "redirect", "dev", "fc1"},
		{"tc", "-n", "mm-fc-a1b2c3-7", "qdisc", "add", "dev", "fc1", "ingress"},
		{"tc", "-n", "mm-fc-a1b2c3-7", "filter", "add", "dev", "fc1", "parent", "ffff:", "matchall", "action", "mirred", "egress", "redirect", "dev", "vm1"},
	}

	if got := netnsTapCommands("mm-fc-a1b2c3-7", 1, 200007, 1450); !reflect.DeepEqual(got, want) {
		t.Errorf("netnsTapCommands() =\n%q\nwant\n%q", got, want)
	}
}

func TestCreateNetnsTap(t *testing.T) {
	f := &fakeNetns{}
	f.install(t)
	b := testBridge()

	tap, mtu, err := b.CreateNetnsTap("mm-fc-a1b2c3-7", "00:11:22:33:44:55", 101, 0, 200007)
	if err != nil {
		t.Fatalf("CreateNetnsTap: %v", err)
	}
	if tap != "mega_tap7" || mtu != 9000 {
		t.Errorf("CreateNetnsTap() = %q, %d, want mega_tap7, 9000", tap, mtu)
	}

	want := append([][]string{
		{"ip", "link", "add", "mega_tap7", "type", "veth", "peer", "name", "vm0", "netns", "mm-fc-a1b2c3-7"},
		{"ip", "link", "set", "mega_tap7", "up"},
	}, netnsTapCommands("mm-fc-a1b2c3-7", 0, 200007, 9000)...)
	if !reflect.DeepEqual(f.cmds, want) {
		t.Errorf("commands =\n%q\nwant\n%q", f.cmds, want)
	}

	if f.locked != 0 {
		t.Errorf("%d commands ran while bridgeLock was held", f.locked)
	}
	if f.addCalls != 1 || !f.addOK {
		t.Errorf("netnsAddTap calls = %d, under bridgeLock = %v, want 1 call under the lock", f.addCalls, f.addOK)
	}
	if got := b.taps["mega_tap7"]; got == nil || got.VLAN != 101 || got.MAC != "00:11:22:33:44:55" {
		t.Errorf("tap table entry = %+v", got)
	}
}

func TestCreateNetnsTapCleanup(t *testing.T) {
	del := []string{"ip", "link", "del", "mega_tap7"}

	for _, tc := range []struct {
		name    string
		fail    string
		addErr  error
		wantAdd int
	}{
		{"veth", "type veth", nil, 0},
		{"up", "set mega_tap7 up", nil, 0},
		{"tuntap", "tuntap add", nil, 0},
		{"tc", "mirred", nil, 0},
		{"ovs", "", errors.New("add port failed"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeNetns{fail: tc.fail, addErr: tc.addErr}
			f.install(t)
			b := testBridge()

			tap, _, err := b.CreateNetnsTap("mm-fc-a1b2c3-7", "00:11:22:33:44:55", 101, 0, 200007)
			if err == nil {
				t.Fatalf("CreateNetnsTap() = %q, want an error", tap)
			}
			if f.addCalls != tc.wantAdd {
				t.Errorf("netnsAddTap calls = %d, want %d", f.addCalls, tc.wantAdd)
			}

			last := f.cmds[len(f.cmds)-1]
			if tc.name == "veth" {
				if len(f.cmds) != 1 {
					t.Errorf("commands after a failed veth add: %q", f.cmds)
				}
				return
			}
			if !reflect.DeepEqual(last, del) {
				t.Errorf("last command = %q, want %q", last, del)
			}
			if len(b.taps) != 0 {
				t.Errorf("tap table = %v, want empty", b.taps)
			}
		})
	}
}

func TestCreateNetnsTapMTU(t *testing.T) {
	f := &fakeNetns{}
	f.install(t)
	if err := os.WriteFile(filepath.Join(netnsSysNet, "mega_tap7", "mtu"), []byte("bad\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := testBridge().CreateNetnsTap("ns", "", 1, 0, 1); err == nil {
		t.Fatal("bad MTU: no error")
	}
	if last := f.cmds[len(f.cmds)-1]; !reflect.DeepEqual(last, []string{"ip", "link", "del", "mega_tap7"}) {
		t.Errorf("last command = %q", last)
	}
}

func TestCreateDestroyNetns(t *testing.T) {
	f := &fakeNetns{}
	f.install(t)

	if err := CreateNetns("mm-fc-a1b2c3-7"); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"ip", "netns", "add", "mm-fc-a1b2c3-7"}}; !reflect.DeepEqual(f.cmds, want) {
		t.Errorf("new netns commands = %q, want %q", f.cmds, want)
	}

	if err := os.MkdirAll(netnsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(netnsDir, "mm-fc-a1b2c3-7"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	f.cmds = nil
	if err := CreateNetns("mm-fc-a1b2c3-7"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"ip", "netns", "del", "mm-fc-a1b2c3-7"},
		{"ip", "netns", "add", "mm-fc-a1b2c3-7"},
	}
	if !reflect.DeepEqual(f.cmds, want) {
		t.Errorf("leftover netns commands = %q, want %q", f.cmds, want)
	}

	f.fail = "netns del"
	f.failOut = `Cannot remove namespace file "/var/run/netns/x": No such file or directory`
	if err := DestroyNetns("x"); err != nil {
		t.Errorf("missing netns: %v", err)
	}

	f.failOut = "permission denied"
	if err := DestroyNetns("x"); err == nil {
		t.Error("failed delete: no error")
	}
}
