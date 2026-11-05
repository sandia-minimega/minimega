// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/vlans"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"
)

func TestVLANsAddMeshageSourceDoesNotBroadcast(t *testing.T) {
	var (
		originalVLANs       = vlans.Default
		originalBase        = *f_base
		originalMeshageNode = meshageNode
	)

	defer func() {
		vlans.Default = originalVLANs
		*f_base = originalBase
		meshageNode = originalMeshageNode
	}()

	vlans.Default = vlans.NewVLANs()
	*f_base = t.TempDir()
	meshageNode = nil

	c := &minicli.Command{
		StringArgs: map[string]string{
			"alias": "mesh-alias",
			"vlan":  "101",
		},
		Source: "meshage",
	}

	if err := cliVLANsAdd(&Namespace{Name: "test"}, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if vlan, err := vlans.ParseVLAN("test", "mesh-alias"); err != nil || vlan != 101 {
		t.Fatalf("got VLAN %d, %v; want 101, nil", vlan, err)
	}

	if _, err := os.Stat(filepath.Join(*f_base, "vlans")); err != nil {
		t.Fatal(err)
	}
}

// setupClearVLANsBlacklist replaces the global VLAN, base directory, and
// namespace state with fresh state for the test and restores it afterward.
func setupClearVLANsBlacklist(t *testing.T, nss ...*Namespace) {
	t.Helper()

	var (
		originalVLANs = vlans.Default
		originalBase  = *f_base
	)

	namespaceLock.Lock()
	originalNamespaces := namespaces
	namespaces = map[string]*Namespace{}
	for _, ns := range nss {
		namespaces[ns.Name] = ns
	}
	namespaceLock.Unlock()

	t.Cleanup(func() {
		vlans.Default = originalVLANs
		*f_base = originalBase

		namespaceLock.Lock()
		namespaces = originalNamespaces
		namespaceLock.Unlock()
	})

	vlans.Default = vlans.NewVLANs()
	*f_base = t.TempDir()
}

func clearVLANsBlacklistCommand(t *testing.T, vlan string) *minicli.Command {
	t.Helper()

	c, err := minicli.Compile("clear vlans blacklist " + vlan)
	if err != nil {
		t.Fatal(err)
	}

	return c
}

func TestClearVLANsBlacklistPatterns(t *testing.T) {
	tests := []struct {
		input     string
		blacklist bool
		vlan      string
		prefix    string
	}{
		{input: "clear vlans"},
		{input: "clear vlans all", prefix: "all"},
		{input: "clear vlans foo", prefix: "foo"},
		// without a VLAN ID, blacklist is still a prefix
		{input: "clear vlans blacklist", prefix: "blacklist"},
		{input: "clear vlans blacklist 100", blacklist: true, vlan: "100"},
	}

	for _, test := range tests {
		c, err := minicli.Compile(test.input)
		if err != nil {
			t.Errorf("%q: %v", test.input, err)
			continue
		}

		if got := c.BoolArgs["blacklist"]; got != test.blacklist {
			t.Errorf("%q: got blacklist %v, want %v", test.input, got, test.blacklist)
		}
		if got := c.StringArgs["vlan"]; got != test.vlan {
			t.Errorf("%q: got vlan %q, want %q", test.input, got, test.vlan)
		}
		if got := c.StringArgs["prefix"]; got != test.prefix {
			t.Errorf("%q: got prefix %q, want %q", test.input, got, test.prefix)
		}
	}
}

func TestClearVLANsBlacklist(t *testing.T) {
	ns := &Namespace{Name: "test"}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)
	vlans.Blacklist(200)

	c := clearVLANsBlacklistCommand(t, "100")
	if err := cliClearVLANs(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 200 {
		t.Errorf("got blacklist %v, want [200]", got)
	}

	if _, err := os.Stat(filepath.Join(*f_base, "vlans")); err != nil {
		t.Error(err)
	}
}

func TestClearVLANsBlacklistInvalid(t *testing.T) {
	ns := &Namespace{Name: "test"}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	if err := vlans.AddAlias("test", "foo", 200); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		vlan string
		want string
	}{
		{vlan: "foo", want: "expected integer VLAN"},
		{vlan: "-1", want: "invalid VLAN"},
		{vlan: "4096", want: "invalid VLAN"},
		{vlan: "101", want: "VLAN 101 is not blacklisted"},
		{vlan: "200", want: "VLAN 200 is not blacklisted"},
	}

	for _, test := range tests {
		c := clearVLANsBlacklistCommand(t, test.vlan)

		err := cliClearVLANs(ns, c, &minicli.Response{})
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("%v: got error %v, want %q", test.vlan, err, test.want)
		}
	}

	if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 100 {
		t.Errorf("got blacklist %v, want [100]", got)
	}
	if got, err := vlans.GetVLAN("test", "foo"); err != nil || got != 200 {
		t.Errorf("got VLAN %v, %v, want 200, nil", got, err)
	}
}

func TestClearVLANsBlacklistInUse(t *testing.T) {
	nics := NetConfigs{{VLAN: 100}}

	ns := &Namespace{
		Name: "test",
		savedVMConfig: map[string]VMConfig{
			"saved": {BaseConfig: BaseConfig{Networks: nics}},
		},
		queue: []*QueuedVMs{
			{Names: []string{"a", "b"}, VMConfig: VMConfig{BaseConfig: BaseConfig{Networks: nics}}},
		},
	}
	ns.vmConfig.Networks = nics

	other := &Namespace{Name: "other"}
	other.vmConfig.Networks = nics

	setupClearVLANsBlacklist(t, ns, other)

	vlans.Blacklist(100)

	c := clearVLANsBlacklistCommand(t, "100")

	err := cliClearVLANs(ns, c, &minicli.Response{})
	if err == nil {
		t.Fatal("removed VLAN that is in use from the blacklist")
	}

	for _, want := range []string{
		"vm config in namespace test",
		"vm config in namespace other",
		"saved vm config saved in namespace test",
		"queued VMs a,b in namespace test",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got error %q, want it to contain %q", err, want)
		}
	}

	if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 100 {
		t.Errorf("got blacklist %v, want [100]", got)
	}
}

func TestNamespaceVLANVMUsers(t *testing.T) {
	ns := &Namespace{Name: "test"}

	vm1 := testVM(KVM, VM_RUNNING)
	vm1.(*KvmVM).Name = "vm1"
	vm1.(*KvmVM).Host = "host1"
	vm1.(*KvmVM).Networks = NetConfigs{{VLAN: 200}, {VLAN: 100}}

	// VMs that quit can start again so they still use the VLAN
	vm2 := testVM(CONTAINER, VM_QUIT)
	vm2.(*ContainerVM).Name = "vm2"
	vm2.(*ContainerVM).Host = "host2"
	vm2.(*ContainerVM).Networks = NetConfigs{{VLAN: 100}}

	vm3 := testVM(KVM, VM_RUNNING)
	vm3.(*KvmVM).Name = "vm3"
	vm3.(*KvmVM).Networks = NetConfigs{{VLAN: 200}}
	vm3.(*KvmVM).Bonds = BondConfigs{{Name: "bond0", VLAN: 200, Created: true}}

	// A bond keeps its VLAN after its interface on that VLAN disconnects
	vm4 := testVM(KVM, VM_RUNNING)
	vm4.(*KvmVM).Name = "vm4"
	vm4.(*KvmVM).Host = "host1"
	vm4.(*KvmVM).Networks = NetConfigs{{VLAN: DisconnectedVLAN}, {VLAN: 200}}
	vm4.(*KvmVM).Bonds = BondConfigs{{Name: "bond0", Interfaces: []int{0, 1}, VLAN: 100, Created: true}}

	// A bond that is not created yet has VLAN 0, which is not a use of VLAN 0
	vm5 := testVM(CONTAINER, VM_RUNNING)
	vm5.(*ContainerVM).Name = "vm5"
	vm5.(*ContainerVM).Networks = NetConfigs{{VLAN: 200}}
	vm5.(*ContainerVM).Bonds = BondConfigs{{Name: "bond0", Interfaces: []int{0}}}

	vms := []VM{vm1, vm2, vm3, vm4, vm5}

	got := ns.vlanVMUsers(100, vms)
	want := []string{
		"VM vm1 on host1 in namespace test",
		"VM vm2 on host2 in namespace test",
		"VM vm4 on host1 in namespace test",
	}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}

	for _, vlan := range []int{0, 300} {
		if got := ns.vlanVMUsers(vlan, vms); len(got) != 0 {
			t.Errorf("VLAN %v: got %q, want no users", vlan, got)
		}
	}
}

// TestNamespaceVLANVMUsersPeerBond checks that the check finds a bond of a VM
// that a peer sends in a mesh response.
func TestNamespaceVLANVMUsersPeerBond(t *testing.T) {
	vm := testVM(KVM, VM_RUNNING)
	vm.(*KvmVM).Name = "vm1"
	vm.(*KvmVM).Host = "host1"
	vm.(*KvmVM).Networks = NetConfigs{{VLAN: DisconnectedVLAN}, {VLAN: 200}}
	vm.(*KvmVM).Bonds = BondConfigs{{Name: "bond0", Interfaces: []int{0, 1}, VLAN: 100, Created: true}}

	// Mesh responses use gob
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(&minicli.Response{Host: "host1", Data: []VM{vm}}); err != nil {
		t.Fatal(err)
	}

	var resp minicli.Response
	if err := gob.NewDecoder(&buf).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	ns := &Namespace{Name: "test"}

	got := ns.vlanVMUsers(100, resp.Data.([]VM))
	if strings.Join(got, ",") != "VM vm1 on host1 in namespace test" {
		t.Errorf("got users %q, want VM vm1", got)
	}
}

// TestClearVLANsBlacklistLocalVM checks that the command finds a VM on this
// node when this node is not in the hosts for the namespace.
func TestClearVLANsBlacklistLocalVM(t *testing.T) {
	vm := testVM(KVM, VM_RUNNING)
	vm.(*KvmVM).ID = 1
	vm.(*KvmVM).Name = "vm1"
	vm.(*KvmVM).Host = "local"
	vm.(*KvmVM).Networks = NetConfigs{{VLAN: 100}}

	ns := &Namespace{Name: "test", Hosts: map[string]bool{}}
	ns.VMs.m = map[int]VM{1: vm}

	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
	if err == nil || !strings.Contains(err.Error(), "VM vm1 on local in namespace test") {
		t.Errorf("got error %v, want VM vm1 in use", err)
	}

	if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 100 {
		t.Errorf("got blacklist %v, want [100]", got)
	}
}

func TestCollectPeerVMs(t *testing.T) {
	newVM := func(name string) VM {
		vm := testVM(KVM, VM_RUNNING)
		vm.(*KvmVM).Name = name
		return vm
	}

	send := func(resps ...minicli.Responses) <-chan minicli.Responses {
		out := make(chan minicli.Responses, len(resps))
		for _, r := range resps {
			out <- r
		}
		close(out)
		return out
	}

	vms, err := collectPeerVMs(send(
		minicli.Responses{{Host: hostname, Data: []VM{newVM("local")}}},
		minicli.Responses{
			{Host: hostname + "-peer1", Data: []VM{newVM("peer1")}},
			{Host: hostname + "-peer2"},
		},
	), []string{hostname + "-peer1", hostname + "-peer2"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vms) != 1 || vms[0].GetName() != "peer1" {
		t.Errorf("got VMs %v, want [peer1]", vms)
	}

	// a host that does not answer, a failed mesh send, and a bad response
	_, err = collectPeerVMs(send(
		minicli.Responses{
			{Host: hostname + "-peer1", Data: []VM{newVM("peer1")}},
			{Host: hostname + "-peer2", Error: "timed out"},
		},
		minicli.Responses{{Host: hostname, Error: "unable to send"}},
		minicli.Responses{{Host: hostname + "-peer3", Data: "foo"}},
	), nil)
	if err == nil {
		t.Fatal("got no error for hosts that did not return VMs")
	}
	for _, want := range []string{"timed out", "unable to send", "unknown data field"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got error %q, want it to contain %q", err, want)
		}
	}
}

// TestCollectPeerVMsMissingHost checks that a namespace host without a
// response is an error. Mesh routing skips hosts that have no route.
func TestCollectPeerVMsMissingHost(t *testing.T) {
	out := make(chan minicli.Responses, 1)
	out <- minicli.Responses{{Host: hostname + "-peer1"}}
	close(out)

	_, err := collectPeerVMs(out, []string{hostname + "-peer2", hostname + "-peer1"})
	if err == nil || !strings.Contains(err.Error(), hostname+"-peer2: no response") {
		t.Errorf("got error %v, want no response from peer2", err)
	}
	if err != nil && strings.Contains(err.Error(), hostname+"-peer1:") {
		t.Errorf("got error %v for peer1 that responded", err)
	}
}

// pollUntil polls cond until it returns true or a timeout occurs. Returns the last
// value of cond.
func pollUntil(cond func() bool) bool {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if cond() {
			return true
		}

		time.Sleep(time.Millisecond)
	}

	return cond()
}

// activeVLANUses returns the number of commands that use VLANs.
func activeVLANUses() int {
	vlanUseLock.mu.Lock()
	defer vlanUseLock.mu.Unlock()

	return len(vlanUseLock.active)
}

// launchingCount returns the number of launching VM batches in the namespace.
func launchingCount(ns *Namespace) int {
	ns.configLock.Lock()
	defer ns.configLock.Unlock()

	return len(ns.launching)
}

// lockVMs locks the VMs of the namespace and returns a function that unlocks
// them once.
func lockVMs(t *testing.T, ns *Namespace) func() {
	t.Helper()

	ns.VMs.mu.Lock()
	unlock := sync.OnceFunc(ns.VMs.mu.Unlock)
	t.Cleanup(unlock)

	return unlock
}

// vlan100VMs returns a VM batch with an unknown VM type that uses VLAN 100. Its
// launch fails before it starts a VM.
func vlan100VMs(names ...string) *QueuedVMs {
	return &QueuedVMs{
		Names:    names,
		VMConfig: VMConfig{BaseConfig: BaseConfig{Networks: NetConfigs{{VLAN: 100}}}},
	}
}

// TestClearVLANsBlacklistInUseBranches checks each type of VLAN user alone.
func TestClearVLANsBlacklistInUseBranches(t *testing.T) {
	nics := NetConfigs{{VLAN: 100}}

	tests := []struct {
		name  string
		setup func(ns *Namespace)
		want  string
	}{
		{
			name:  "vm config",
			setup: func(ns *Namespace) { ns.vmConfig.Networks = nics },
			want:  "vm config in namespace test",
		},
		{
			name: "saved vm config",
			setup: func(ns *Namespace) {
				ns.savedVMConfig["saved"] = VMConfig{BaseConfig: BaseConfig{Networks: nics}}
			},
			want: "saved vm config saved in namespace test",
		},
		{
			name:  "queued VMs",
			setup: func(ns *Namespace) { ns.queue = []*QueuedVMs{vlan100VMs("a", "b")} },
			want:  "queued VMs a,b in namespace test",
		},
		{
			name:  "launching VMs",
			setup: func(ns *Namespace) { ns.addLaunching([]*QueuedVMs{vlan100VMs("a")}) },
			want:  "launching VMs in namespace test",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ns := &Namespace{Name: "test", savedVMConfig: map[string]VMConfig{}}
			ns.vmConfig.Networks = NetConfigs{{VLAN: 200}}
			test.setup(ns)

			setupClearVLANsBlacklist(t, ns)
			vlans.Blacklist(100)

			err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
			if err == nil || !strings.HasSuffix(err.Error(), "is in use by "+test.want) {
				t.Errorf("got error %v, want only %q", err, test.want)
			}

			if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 100 {
				t.Errorf("got blacklist %v, want [100]", got)
			}
		})
	}
}

// TestClearVLANsBlacklistLaunch checks that Namespace.Launch shows its VMs as
// launching before they are in VMs.
func TestClearVLANsBlacklistLaunch(t *testing.T) {
	ns := &Namespace{Name: "test"}
	setupClearVLANsBlacklist(t, ns)

	// The launch stops before it adds the VMs to VMs
	unlock := lockVMs(t, ns)

	launched := make(chan []error, 1)
	go func() {
		launched <- ns.Launch("", vlan100VMs("a"))
	}()

	pollUntil(func() bool { return launchingCount(ns) > 0 })

	got := ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "launching VMs in namespace test" {
		t.Errorf("got users %q during launch, want launching VMs", got)
	}

	unlock()

	if errs := <-launched; len(errs) == 0 {
		t.Error("launched VM with unknown type")
	}

	if got := ns.vlanConfigUsers(100); len(got) != 0 {
		t.Errorf("got users %q after launch, want none", got)
	}
}

// TestClearVLANsBlacklistSchedule checks that the scheduler moves the VMs from
// the queue to launching in one step.
func TestClearVLANsBlacklistSchedule(t *testing.T) {
	// The scheduler stops when it names the VMs, before Launch
	ids := make(chan int)
	q := vlan100VMs("")

	ns := &Namespace{
		Name:  "test",
		Hosts: map[string]bool{hostname: true},
		queue: []*QueuedVMs{q},
		vmID:  &Counter{vals: ids},
	}
	setupClearVLANsBlacklist(t, ns)

	scheduled := make(chan error, 1)
	go func() {
		scheduled <- ns.schedule(map[string][]*QueuedVMs{hostname: {q}}, []*QueuedVMs{q})
	}()

	t.Cleanup(func() {
		close(ids)
		<-scheduled
	})

	pollUntil(func() bool { return len(ns.queued()) == 0 })

	got := ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "launching VMs in namespace test" {
		t.Errorf("got users %q during schedule, want launching VMs", got)
	}
}

// TestClearVLANsBlacklistScheduleFlush checks that Schedule shows its copy of
// the queue as launching when another command flushes the queue.
func TestClearVLANsBlacklistScheduleFlush(t *testing.T) {
	// The scheduler stops when it names the VMs, before Launch
	ids := make(chan int)

	ns := &Namespace{
		Name:  "test",
		Hosts: map[string]bool{hostname: true},
		queue: []*QueuedVMs{vlan100VMs("")},
		vmID:  &Counter{vals: ids},
	}
	setupClearVLANsBlacklist(t, ns)

	// Schedule stops when it gets the VMs in the namespace, after the copy
	unlock := lockVMs(t, ns)

	scheduled := make(chan error, 1)
	go func() {
		scheduled <- ns.Schedule(false)
	}()

	finish := sync.OnceValue(func() error {
		unlock()
		close(ids)
		return <-scheduled
	})
	t.Cleanup(func() { finish() })

	pollUntil(func() bool { return launchingCount(ns) > 0 })

	if err := cliNamespaceFlush(ns, nil, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	got := ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "launching VMs in namespace test" {
		t.Errorf("got users %q after the flush, want launching VMs", got)
	}

	finish()

	if n := launchingCount(ns); n != 0 {
		t.Errorf("got %v launching VM batches after Schedule, want 0", n)
	}

	// The dry run and its error paths also end the launch
	ns.queue = []*QueuedVMs{vlan100VMs("a")}
	ns.Schedule(true)

	if n := launchingCount(ns); n != 0 {
		t.Errorf("got %v launching VM batches after a dry run, want 0", n)
	}
}

// TestScheduleKeepsNewQueuedVMs checks that Schedule removes from the queue
// only the VMs that it schedules, not the VMs that other commands queue while
// Schedule gets the hosts and the VMs.
func TestScheduleKeepsNewQueuedVMs(t *testing.T) {
	// The scheduler stops when it names the VMs, before Launch
	ids := make(chan int)

	ns := &Namespace{
		Name:  "test",
		Hosts: map[string]bool{hostname: true},
		queue: []*QueuedVMs{vlan100VMs("")},
		vmID:  &Counter{vals: ids},
	}
	setupClearVLANsBlacklist(t, ns)

	// Schedule stops when it gets the VMs in the namespace, after the copy
	unlock := lockVMs(t, ns)

	scheduled := make(chan error, 1)
	go func() {
		scheduled <- ns.Schedule(false)
	}()

	finish := sync.OnceValue(func() error {
		unlock()
		close(ids)
		return <-scheduled
	})
	t.Cleanup(func() { finish() })

	pollUntil(func() bool { return launchingCount(ns) > 0 })

	// Queue adds a batch in the same way
	added := vlan100VMs("b")
	ns.configLock.Lock()
	ns.queue = append(ns.queue, added)
	ns.configLock.Unlock()

	if err := finish(); err != nil {
		t.Fatal(err)
	}

	if got := ns.queued(); len(got) != 1 || got[0] != added {
		t.Errorf("got %v queued VM batches after Schedule, want only the added batch", len(got))
	}
}

// TestVMConfigCloneLooksUpVLANs checks that "vm config clone" puts only the
// VLANs that it looks up in the VM configuration, not the VLANs of the VM.
func TestVMConfigCloneLooksUpVLANs(t *testing.T) {
	vm := testVM(KVM, VM_RUNNING)
	vm.(*KvmVM).ID = 1
	vm.(*KvmVM).Name = "vm1"
	vm.(*KvmVM).Memory = 4096
	vm.(*KvmVM).KVMConfig = NewVMConfig().KVMConfig
	vm.(*KvmVM).Networks = NetConfigs{{VLAN: 100, Raw: "100"}}

	ns := &Namespace{
		Name:     "test",
		Hosts:    map[string]bool{hostname: true},
		vmConfig: NewVMConfig(),
	}
	ns.VMs.m = map[int]VM{1: vm}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	clone := func() error {
		c := minicli.MustCompile("vm config clone vm1")
		return cliVMConfig(ns, c, &minicli.Response{})
	}

	// The clone stops when it starts to look up the VLANs
	vlanUseLock.mu.Lock()
	unlock := sync.OnceFunc(vlanUseLock.mu.Unlock)
	t.Cleanup(unlock)

	cloned := make(chan error, 1)
	go func() {
		cloned <- clone()
	}()

	pollUntil(func() bool {
		ns.configLock.Lock()
		defer ns.configLock.Unlock()

		return ns.vmConfig.Memory == 4096
	})

	if got := ns.vlanConfigUsers(100); len(got) != 0 {
		t.Errorf("got users %q before the lookup, want none", got)
	}

	unlock()

	if err := <-cloned; err != nil {
		t.Fatal(err)
	}

	got := ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "vm config in namespace test" {
		t.Errorf("got users %q after the lookup, want vm config", got)
	}

	// A clone that starts while the VLAN is held does not get the VLAN
	release, err := vlans.HoldBlacklisted(100)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	if err := clone(); err == nil || !strings.Contains(err.Error(), "is being removed") {
		t.Errorf("got error %v, want VLAN 100 is being removed", err)
	}

	if got := ns.vlanConfigUsers(100); len(got) != 0 {
		t.Errorf("got users %q after the failed lookup, want none", got)
	}
}

// TestClearVLANsBlacklistLaunchCopy checks that "vm launch" shows its copy of
// the VM configuration as launching until the copy is in the queue.
func TestClearVLANsBlacklistLaunchCopy(t *testing.T) {
	ns := &Namespace{
		Name:     "test",
		Hosts:    map[string]bool{hostname: true},
		QueueVMs: true,
	}
	ns.vmConfig.Networks = NetConfigs{{VLAN: 100}}
	setupClearVLANsBlacklist(t, ns)

	// Queue stops when it gets the VMs in the namespace, after the copy
	unlock := lockVMs(t, ns)

	queued := make(chan error, 1)
	go func() {
		queued <- cliVMLaunch(ns, minicli.MustCompile("vm launch kvm a"), &minicli.Response{})
	}()

	pollUntil(func() bool { return launchingCount(ns) > 0 })

	// Another command changes the VM configuration
	ns.clearVMConfig(Wildcard)

	got := ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "launching VMs in namespace test" {
		t.Errorf("got users %q before the copy is queued, want launching VMs", got)
	}

	unlock()

	if err := <-queued; err != nil {
		t.Fatal(err)
	}

	got = ns.vlanConfigUsers(100)
	if strings.Join(got, ",") != "queued VMs a in namespace test" {
		t.Errorf("got users %q after the copy is queued, want queued VMs", got)
	}
}

// TestClearVLANsBlacklistConfigRace checks that the command does not race with
// commands that change the VM configurations and the queue. Each step keeps a
// user of VLAN 100, so the command must always fail. Run it with -race.
func TestClearVLANsBlacklistConfigRace(t *testing.T) {
	ns := &Namespace{Name: "test", savedVMConfig: map[string]VMConfig{}, QueueVMs: true}
	ns.vmConfig = NewVMConfig()
	ns.vmConfig.Networks = NetConfigs{{VLAN: 100}}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	vmConfig := func(args string) error {
		c := minicli.MustCompile("vm config " + args)
		return cliVMConfig(ns, c, &minicli.Response{})
	}

	run := func(cmd string) error {
		c := minicli.MustCompile(cmd)
		c.SetNamespace(ns.Name)
		return consume(runCommands(c))
	}

	steps := []func(i int) error{
		func(int) error { return vmConfig("save s") },
		func(int) error { return run("clear vm config") },
		func(int) error { return vmConfig("restore s") },
		func(i int) error {
			c := minicli.MustCompile(fmt.Sprintf("vm launch kvm vm%v", i))
			return cliVMLaunch(ns, c, &minicli.Response{})
		},
		func(int) error { return run("clear vm config networks") },
		func(int) error { return ns.processVMNets([]string{"200"}) },
		func(int) error { return vmConfig("save s") },
		func(int) error {
			// The lookup fails while the command holds the VLAN
			for {
				err := ns.processVMNets([]string{"100"})
				if err == nil || !strings.Contains(err.Error(), "is being removed") {
					return err
				}
				time.Sleep(time.Millisecond)
			}
		},
		func(int) error { return cliNamespaceFlush(ns, nil, &minicli.Response{}) },
	}

	changed := make(chan struct{})
	go func() {
		defer close(changed)

		for i := 0; i < 50; i++ {
			for _, step := range steps {
				if err := step(i); err != nil {
					t.Error(err)
					return
				}
			}
		}
	}()

	for checks := 0; ; checks++ {
		select {
		case <-changed:
			if checks == 0 {
				t.Error("did not run the command during the changes")
			}
			return
		default:
		}

		err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
		if err == nil || !strings.Contains(err.Error(), "VLAN 100 is in use by") {
			t.Errorf("got error %v, want VLAN 100 in use", err)
			vlans.Blacklist(100)
		}

		// Let the changes look up the VLAN between the checks
		time.Sleep(time.Millisecond)
	}
}

// TestVMConfigNetRace checks that "vm config net" does not race with the check
// of the VM configurations. Run it with -race.
func TestVMConfigNetRace(t *testing.T) {
	ns := &Namespace{Name: "test", vmConfig: NewVMConfig()}
	setupClearVLANsBlacklist(t, ns)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			if err := ns.processVMNets([]string{"200"}); err != nil {
				t.Error(err)
			}
		}
	}()

	for i := 0; i < 100; i++ {
		ns.vlanConfigUsers(100)
	}

	wg.Wait()
}

// TestClearVLANsBlacklistRescheduleRace checks that the command does not race
// with commands that change the scheduled VMs and the hosts. Run it with -race.
func TestClearVLANsBlacklistRescheduleRace(t *testing.T) {
	q := vlan100VMs("a", "b", "c")

	ns := &Namespace{
		Name:  "test",
		Hosts: map[string]bool{hostname: true},
		queue: []*QueuedVMs{q},
	}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	delHost := minicli.MustCompile("ns del-hosts foo")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 100; i++ {
			ns.configLock.Lock()
			q.Names = []string{"a", "b", "c"}
			ns.assignment = map[string][]*QueuedVMs{hostname: {q}}
			ns.configLock.Unlock()

			if err := ns.Reschedule("a", hostname); err != nil {
				t.Error(err)
			}
			if err := cliNamespaceDelHost(ns, delHost, &minicli.Response{}); err != nil {
				t.Error(err)
			}
		}
	}()

	for i := 0; i < 100; i++ {
		if err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{}); err == nil {
			t.Fatal("removed VLAN that is in use from the blacklist")
		}
	}

	wg.Wait()
}

func TestHostTapVLANUsers(t *testing.T) {
	taps := []bridge.Tap{
		{Name: "mega_tap1", VLAN: 100},
		{Name: "mega_tap2", VLAN: 200},
		{Name: "mega_tap3", VLAN: 100},
	}

	got := hostTapVLANUsers(100, taps)
	want := []string{"host tap mega_tap1", "host tap mega_tap3"}

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestClearVLANsBlacklistAliasNamedBlacklisted checks that an alias with the
// name BLACKLISTED is not a blacklist entry, even when a VM config uses it.
func TestClearVLANsBlacklistAliasNamedBlacklisted(t *testing.T) {
	ns := &Namespace{Name: "test"}
	ns.vmConfig.Networks = NetConfigs{{VLAN: 150}}
	setupClearVLANsBlacklist(t, ns)

	if err := vlans.AddAlias("test", vlans.BlacklistedVLAN, 150); err != nil {
		t.Fatal(err)
	}

	err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "150"), &minicli.Response{})
	if err == nil || !strings.Contains(err.Error(), "VLAN 150 is not blacklisted") {
		t.Errorf("got error %v, want not blacklisted", err)
	}
}

// TestClearVLANsBlacklistHold checks that the command refuses lookups of the
// VLAN during the check and ends the hold when it fails.
func TestClearVLANsBlacklistHold(t *testing.T) {
	ns := &Namespace{Name: "test"}
	ns.vmConfig.Networks = NetConfigs{{VLAN: 100}}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	release, err := vlans.HoldBlacklisted(100)
	if err != nil {
		t.Fatal(err)
	}

	err = cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
	if err == nil || !strings.Contains(err.Error(), "already being removed") {
		t.Errorf("got error %v, want already being removed", err)
	}

	if _, err := lookupVLAN(ns.Name, "100"); err == nil {
		t.Error("looked up VLAN that is held")
	}

	release()

	// the VLAN is in use so the command fails and must end its hold
	if err := cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{}); err == nil {
		t.Fatal("removed VLAN that is in use from the blacklist")
	}

	if got, err := lookupVLAN(ns.Name, "100"); err != nil || got != 100 {
		t.Errorf("got VLAN %v, %v, want 100, nil", got, err)
	}
}

// holdObserved returns true when the command holds the VLAN.
func holdObserved(ns *Namespace, vlan string) bool {
	_, err := lookupVLAN(ns.Name, vlan)
	return err != nil && strings.Contains(err.Error(), "is being removed from the blacklist")
}

// TestClearVLANsBlacklistWaitsForUse checks that the command waits for a
// command that looked up the VLAN before the hold and then finds its use.
func TestClearVLANsBlacklistWaitsForUse(t *testing.T) {
	ns := &Namespace{Name: "test"}
	setupClearVLANsBlacklist(t, ns)

	vlans.Blacklist(100)

	// A command that looked up the VLAN and did not store it yet
	done := vlanUseLock.Use()

	var wg sync.WaitGroup
	t.Cleanup(func() {
		done()
		wg.Wait()
	})

	cleared := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cleared <- cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
	}()

	if !pollUntil(func() bool { return holdObserved(ns, "100") }) {
		t.Fatal("command did not hold the VLAN")
	}

	select {
	case err := <-cleared:
		t.Fatalf("command did not wait for VLAN use, got error %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	ns.setVMNetworks(NetConfigs{{VLAN: 100}})
	done()

	err := <-cleared
	if err == nil || !strings.Contains(err.Error(), "vm config in namespace test") {
		t.Errorf("got error %v, want vm config in use", err)
	}

	if got := vlans.GetBlacklist(); len(got) != 1 || got[0] != 100 {
		t.Errorf("got blacklist %v, want [100]", got)
	}
}

// TestClearVLANsBlacklistDoesNotBlockLookups checks that commands that use
// other VLANs do not wait while the command waits for a VLAN use.
func TestClearVLANsBlacklistDoesNotBlockLookups(t *testing.T) {
	ns := &Namespace{Name: "test"}
	other := &Namespace{Name: "other", vmConfig: NewVMConfig()}
	setupClearVLANsBlacklist(t, ns, other)

	vlans.Blacklist(100)

	// A command that looked up the VLAN and waits for vmLock
	done := vlanUseLock.Use()

	var wg sync.WaitGroup
	t.Cleanup(func() {
		done()
		wg.Wait()
	})

	cleared := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		cleared <- cliClearVLANs(ns, clearVLANsBlacklistCommand(t, "100"), &minicli.Response{})
	}()

	if !pollUntil(func() bool { return holdObserved(ns, "100") }) {
		t.Fatal("command did not hold the VLAN")
	}

	// Let the command start to wait
	time.Sleep(50 * time.Millisecond)

	configured := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		configured <- other.processVMNets([]string{"200"})
	}()

	select {
	case err := <-configured:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("vm config net waited for clear vlans blacklist")
	}

	done()

	if err := <-cleared; err != nil {
		t.Errorf("unable to remove VLAN that is not in use: %v", err)
	}
}

// TestVLANUseUntilStored checks that the commands use the VLAN until they
// store it.
func TestVLANUseUntilStored(t *testing.T) {
	tests := []struct {
		name string
		run  func(ns *Namespace) error
		// lock stops the command after the lookup, before it stores the VLAN
		lock func(t *testing.T, ns *Namespace) func()
	}{
		{
			name: "vm config net",
			run:  func(ns *Namespace) error { return ns.processVMNets([]string{"100"}) },
			lock: func(t *testing.T, ns *Namespace) func() {
				ns.configLock.Lock()
				unlock := sync.OnceFunc(ns.configLock.Unlock)
				t.Cleanup(unlock)
				return unlock
			},
		},
		{
			name: "vm net connect",
			run: func(ns *Namespace) error {
				c := minicli.MustCompile("vm net connect vm1 0 100")
				return cliVMNetMod(ns, c, &minicli.Response{})
			},
			lock: lockVMs,
		},
		{
			name: "vm net add",
			run: func(ns *Namespace) error {
				c := minicli.MustCompile("vm net add vm1 100")
				return cliVMNetMod(ns, c, &minicli.Response{})
			},
			lock: lockVMs,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ns := &Namespace{Name: "test", vmConfig: NewVMConfig()}
			setupClearVLANsBlacklist(t, ns)

			vlans.Blacklist(100)

			unlock := test.lock(t, ns)

			ran := make(chan error, 1)
			go func() {
				ran <- test.run(ns)
			}()

			if !pollUntil(func() bool { return activeVLANUses() > 0 }) {
				t.Error("command does not use the VLAN before it stores it")
			}

			unlock()
			<-ran

			if got := activeVLANUses(); got != 0 {
				t.Errorf("got %v VLAN uses after the command, want 0", got)
			}
		})
	}
}

func TestVLANUseTrackerWait(t *testing.T) {
	u := newVLANUseTracker()

	// No uses, so Wait returns immediately
	u.Wait()

	done1 := u.Use()
	done1Again := done1

	waited := make(chan struct{})
	go func() {
		u.Wait()
		close(waited)
	}()

	// Let Wait start before the next use
	time.Sleep(50 * time.Millisecond)

	select {
	case <-waited:
		t.Fatal("Wait did not wait for the use")
	default:
	}

	// Wait does not wait for the uses that start after it
	done2 := u.Use()
	defer done2()

	done1()
	done1Again()

	select {
	case <-waited:
	case <-time.After(5 * time.Second):
		t.Fatal("Wait waited for a use that started after it")
	}

	if len(u.active) != 1 {
		t.Errorf("got %v active uses, want 1", len(u.active))
	}
}

func TestNamespaceVMInfoCommands(t *testing.T) {
	ns := &Namespace{Name: "test", Hosts: map[string]bool{hostname: true, "peer1": true, "peer2": true}}

	peers, cmds := ns.vmInfoCommands()
	slices.Sort(peers)

	if strings.Join(peers, ",") != "peer1,peer2" {
		t.Errorf("got peers %q, want peer1 and peer2", peers)
	}

	if len(cmds) != 2 || cmds[0].Original != "vm info" || !strings.HasPrefix(cmds[1].Original, "mesh send") {
		t.Errorf("got %v commands, want vm info and mesh send", len(cmds))
	}

	ns.Hosts = map[string]bool{}

	if peers, cmds := ns.vmInfoCommands(); len(peers) != 0 || len(cmds) != 0 {
		t.Errorf("got peers %q and %v commands for no hosts, want none", peers, len(cmds))
	}
}
