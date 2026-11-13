// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"

	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

// fakeReaper records the calls of reapTaps and returns configured results.
type fakeReaper struct {
	ports [][]string
	links [][]string

	portErr error

	// linkFail contains the names that delLinks does not destroy
	linkFail map[string]bool

	bridges   []string
	bridgeErr error
}

func (f *fakeReaper) delBridge(bridge string) error {
	f.bridges = append(f.bridges, bridge)

	return f.bridgeErr
}

func (f *fakeReaper) delPorts(bridge string, ports []string) error {
	f.ports = append(f.ports, ports)

	return f.portErr
}

func (f *fakeReaper) delLinks(names []string) ([]string, error) {
	f.links = append(f.links, names)

	var gone []string
	for _, name := range names {
		if !f.linkFail[name] {
			gone = append(gone, name)
		}
	}

	if len(gone) != len(names) {
		return gone, errors.New("device busy")
	}

	return gone, nil
}

func newTestBridge(r tapReaper) *Bridge {
	return &Bridge{
		Name:     "test_bridge",
		preExist: true,
		taps:     map[string]*Tap{},
		trunks:   map[string]bool{},
		tunnels:  map[string]bool{},
		mirrors:  map[string]bool{},
		bonds:    map[string]map[string]int{},
		captures: map[int]capture{},
		config:   map[string]string{},
		reaper:   r,
	}
}

func tapNames(b *Bridge) []string {
	var res []string
	for name := range b.taps {
		res = append(res, name)
	}
	sort.Strings(res)

	return res
}

func TestDestroyTapMarksDevice(t *testing.T) {
	b := newTestBridge(&fakeReaper{})
	b.taps["vm"] = &Tap{Name: "vm"}
	b.taps["host"] = &Tap{Name: "host", Host: true}

	for _, name := range []string{"vm", "host"} {
		if err := b.destroyTap(name); err != nil {
			t.Fatalf("destroyTap(%v): %v", name, err)
		}
	}

	if vm := b.taps["vm"]; !vm.Defunct || !vm.deleteLink {
		t.Errorf("vm tap: got Defunct=%v deleteLink=%v, want both true", vm.Defunct, vm.deleteLink)
	}
	if host := b.taps["host"]; !host.Defunct || host.deleteLink {
		t.Errorf("host tap: got Defunct=%v deleteLink=%v, want true and false", host.Defunct, host.deleteLink)
	}

	if err := b.destroyTap("missing"); err == nil {
		t.Error("destroyTap(missing): got nil error, want error")
	}
}

func TestReapTapsSplitsPortsAndDevices(t *testing.T) {
	r := &fakeReaper{}
	b := newTestBridge(r)

	b.taps["live"] = &Tap{Name: "live"}
	b.taps["vm1"] = &Tap{Name: "vm1"}
	b.taps["vm0"] = &Tap{Name: "vm0"}
	b.taps["host"] = &Tap{Name: "host", Host: true}
	b.taps["bonded"] = &Tap{Name: "bonded", Bond: "bond0"}
	b.bonds["bond0"] = map[string]int{"bonded": 1, "other": 1}

	for _, name := range []string{"vm1", "vm0", "host", "bonded"} {
		if err := b.destroyTap(name); err != nil {
			t.Fatalf("destroyTap(%v): %v", name, err)
		}
	}

	if err := b.reapTaps(); err != nil {
		t.Fatalf("reapTaps: %v", err)
	}

	// bonded taps have no port of their own and OVS deletes host taps
	wantPorts := [][]string{{"host", "vm0", "vm1"}}
	wantLinks := [][]string{{"bonded", "vm0", "vm1"}}

	if !reflect.DeepEqual(r.ports, wantPorts) {
		t.Errorf("delPorts calls: got %v, want %v", r.ports, wantPorts)
	}
	if !reflect.DeepEqual(r.links, wantLinks) {
		t.Errorf("delLinks calls: got %v, want %v", r.links, wantLinks)
	}
	if got, want := tapNames(b), []string{"live"}; !reflect.DeepEqual(got, want) {
		t.Errorf("taps after reap: got %v, want %v", got, want)
	}
	if _, ok := b.bonds["bond0"]["bonded"]; ok {
		t.Error("bonded tap is still in bond0")
	}

	// nothing is defunct, so a second pass does no work
	if err := b.reapTaps(); err != nil {
		t.Fatalf("second reapTaps: %v", err)
	}
	if len(r.ports) != 1 || len(r.links) != 1 {
		t.Errorf("second reapTaps made calls: ports %v, links %v", r.ports, r.links)
	}
}

func TestReapTapsKeepsTapWhenPortRemains(t *testing.T) {
	r := &fakeReaper{portErr: errors.New("ovs down")}
	b := newTestBridge(r)

	b.taps["vm0"] = &Tap{Name: "vm0"}
	b.taps["host"] = &Tap{Name: "host", Host: true}
	b.destroyTap("vm0")
	b.destroyTap("host")

	if err := b.reapTaps(); err == nil {
		t.Fatal("reapTaps: got nil error, want error")
	}

	// the device is gone but the port is not, so the tap stays
	if got, want := tapNames(b), []string{"host", "vm0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("taps after failed del-port: got %v, want %v", got, want)
	}
	if b.taps["vm0"].deleteLink {
		t.Error("vm0 deleteLink is still set after the device was destroyed")
	}

	r.portErr = nil
	if err := b.reapTaps(); err != nil {
		t.Fatalf("retry reapTaps: %v", err)
	}

	wantPorts := [][]string{{"host", "vm0"}, {"host", "vm0"}}
	wantLinks := [][]string{{"vm0"}}

	if !reflect.DeepEqual(r.ports, wantPorts) {
		t.Errorf("delPorts calls: got %v, want %v", r.ports, wantPorts)
	}
	if !reflect.DeepEqual(r.links, wantLinks) {
		t.Errorf("delLinks calls: got %v, want %v", r.links, wantLinks)
	}
	if len(b.taps) != 0 {
		t.Errorf("taps after retry: got %v, want none", tapNames(b))
	}
}

func TestReapTapsKeepsTapWhenDeviceRemains(t *testing.T) {
	r := &fakeReaper{linkFail: map[string]bool{"vm1": true}}
	b := newTestBridge(r)

	b.taps["vm0"] = &Tap{Name: "vm0"}
	b.taps["vm1"] = &Tap{Name: "vm1"}
	b.destroyTap("vm0")
	b.destroyTap("vm1")

	if err := b.reapTaps(); err == nil {
		t.Fatal("reapTaps: got nil error, want error")
	}

	if got, want := tapNames(b), []string{"vm1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("taps after partial delete: got %v, want %v", got, want)
	}
	if vm1 := b.taps["vm1"]; !vm1.portReaped || !vm1.deleteLink {
		t.Errorf("vm1: got portReaped=%v deleteLink=%v, want both true", vm1.portReaped, vm1.deleteLink)
	}

	r.linkFail = nil
	if err := b.reapTaps(); err != nil {
		t.Fatalf("retry reapTaps: %v", err)
	}

	// the retry must not remove the port of vm1 again
	wantPorts := [][]string{{"vm0", "vm1"}}
	wantLinks := [][]string{{"vm0", "vm1"}, {"vm1"}}

	if !reflect.DeepEqual(r.ports, wantPorts) {
		t.Errorf("delPorts calls: got %v, want %v", r.ports, wantPorts)
	}
	if !reflect.DeepEqual(r.links, wantLinks) {
		t.Errorf("delLinks calls: got %v, want %v", r.links, wantLinks)
	}
	if len(b.taps) != 0 {
		t.Errorf("taps after retry: got %v, want none", tapNames(b))
	}
}

func TestDestroyRemovesAllTaps(t *testing.T) {
	r := &fakeReaper{}
	b := newTestBridge(r)

	b.taps["vm0"] = &Tap{Name: "vm0"}
	b.taps["host"] = &Tap{Name: "host", Host: true}
	b.taps["old"] = &Tap{Name: "old"}
	b.destroyTap("old")

	if err := b.destroy(); err != nil {
		t.Fatalf("destroy: %v", err)
	}

	wantPorts := [][]string{{"host", "old", "vm0"}}
	wantLinks := [][]string{{"old", "vm0"}}

	if !reflect.DeepEqual(r.ports, wantPorts) {
		t.Errorf("delPorts calls: got %v, want %v", r.ports, wantPorts)
	}
	if !reflect.DeepEqual(r.links, wantLinks) {
		t.Errorf("delLinks calls: got %v, want %v", r.links, wantLinks)
	}
	if len(b.taps) != 0 {
		t.Errorf("taps after destroy: got %v, want none", tapNames(b))
	}
}

func TestBridgesReapTapsReapsEveryBridge(t *testing.T) {
	bad := &fakeReaper{portErr: errors.New("ovs down")}
	good := &fakeReaper{}

	b0 := newTestBridge(bad)
	b0.Name = "b0"
	b0.taps["t0"] = &Tap{Name: "t0"}
	b0.destroyTap("t0")

	b1 := newTestBridge(good)
	b1.Name = "b1"
	b1.taps["t1"] = &Tap{Name: "t1"}
	b1.destroyTap("t1")

	brs := Bridges{bridges: map[string]*Bridge{"b0": b0, "b1": b1}}

	if err := brs.ReapTaps(); err == nil {
		t.Fatal("ReapTaps: got nil error, want error")
	}
	if len(b1.taps) != 0 {
		t.Errorf("b1 taps: got %v, want none", tapNames(b1))
	}
	if len(b0.taps) != 1 {
		t.Errorf("b0 taps: got %v, want [t0]", tapNames(b0))
	}
}

func TestDelPortArgs(t *testing.T) {
	got := delPortArgs("mega_bridge", []string{"mega_tap0", "mega_tap1"})
	want := []string{
		"--", "--if-exists", "del-port", "mega_bridge", "mega_tap0",
		"--", "--if-exists", "del-port", "mega_bridge", "mega_tap1",
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDestroyRetriesDevicesAfterBridgeDelete(t *testing.T) {
	r := &fakeReaper{portErr: errors.New("ovs busy"), linkFail: map[string]bool{"vm0": true}}
	b := newTestBridge(r)
	b.preExist = false

	b.taps["vm0"] = &Tap{Name: "vm0"}
	b.taps["vm1"] = &Tap{Name: "vm1"}

	if err := b.destroy(); err == nil {
		t.Fatal("destroy: got nil error, want error")
	}

	// del-br removed the ports, so only the device of vm0 is left
	if got, want := tapNames(b), []string{"vm0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("taps after failed destroy: got %v, want %v", got, want)
	}
	if !b.taps["vm0"].portReaped {
		t.Error("vm0 portReaped is false after del-br")
	}

	r.portErr = nil
	r.linkFail = nil

	if err := b.destroy(); err != nil {
		t.Fatalf("retry destroy: %v", err)
	}

	wantPorts := [][]string{{"vm0", "vm1"}}
	wantLinks := [][]string{{"vm0", "vm1"}, {"vm0"}}

	if !reflect.DeepEqual(r.ports, wantPorts) {
		t.Errorf("delPorts calls: got %v, want %v", r.ports, wantPorts)
	}
	if !reflect.DeepEqual(r.links, wantLinks) {
		t.Errorf("delLinks calls: got %v, want %v", r.links, wantLinks)
	}
	if want := []string{"test_bridge"}; !reflect.DeepEqual(r.bridges, want) {
		t.Errorf("delBridge calls: got %v, want %v", r.bridges, want)
	}
	if len(b.taps) != 0 {
		t.Errorf("taps after retry: got %v, want none", tapNames(b))
	}
}

func TestDestroyRetriesBridgeDelete(t *testing.T) {
	r := &fakeReaper{bridgeErr: errors.New("ovs down")}
	b := newTestBridge(r)
	b.preExist = false

	if err := b.destroy(); err == nil {
		t.Fatal("destroy: got nil error, want error")
	}

	r.bridgeErr = nil

	if err := b.destroy(); err != nil {
		t.Fatalf("retry destroy: %v", err)
	}
	if err := b.destroy(); err != nil {
		t.Fatalf("third destroy: %v", err)
	}

	if want := []string{"test_bridge", "test_bridge"}; !reflect.DeepEqual(r.bridges, want) {
		t.Errorf("delBridge calls: got %v, want %v", r.bridges, want)
	}
}

func TestBridgesReapTapsFinishesDestroyedBridge(t *testing.T) {
	r := &fakeReaper{linkFail: map[string]bool{"t0": true}}
	b0 := newTestBridge(r)
	b0.Name = "b0"
	b0.preExist = false
	b0.taps["t0"] = &Tap{Name: "t0"}

	brs := Bridges{bridges: map[string]*Bridge{"b0": b0}}

	if err := brs.DestroyBridge("b0"); err == nil {
		t.Fatal("DestroyBridge: got nil error, want error")
	}
	if _, ok := brs.bridges["b0"]; !ok {
		t.Fatal("b0 was forgotten while its tap device remains")
	}

	if err := brs.ReapTaps(); err == nil {
		t.Fatal("ReapTaps: got nil error while the device remains")
	}
	if _, ok := brs.bridges["b0"]; !ok {
		t.Fatal("b0 was forgotten while its tap device remains")
	}

	r.linkFail = nil

	if err := brs.ReapTaps(); err != nil {
		t.Fatalf("ReapTaps: %v", err)
	}
	if _, ok := brs.bridges["b0"]; ok {
		t.Error("b0 is still in the bridges after its cleanup finished")
	}
	if want := []string{"b0"}; !reflect.DeepEqual(r.bridges, want) {
		t.Errorf("delBridge calls: got %v, want %v", r.bridges, want)
	}
}

func TestIsNoDevice(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("destroy tap failed: %w", unix.ENODEV), true},
		{netlink.LinkNotFoundError{}, true},
		{unix.EBUSY, false},
		{errors.New("other"), false},
		{nil, false},
	}

	for _, c := range cases {
		if got := isNoDevice(c.err); got != c.want {
			t.Errorf("isNoDevice(%v): got %v, want %v", c.err, got, c.want)
		}
	}
}
