// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/ron"
)

const (
	fcCloneSrcMAC = "06:00:ac:10:00:02"
	fcCloneMAC    = "06:00:ac:10:02:01"
)

// fcCloneFakeCC is a fake ron server for the identity of a clone. respond
// gives the guest result of each command: the exit code, the output, and
// false for no response.
type fcCloneFakeCC struct {
	t  *testing.T
	vm *FirecrackerVM

	mu      sync.Mutex
	client  bool
	next    int
	cmds    map[int]*ron.Command
	codes   map[int]int
	outs    map[int]string
	sent    []*ron.Command
	deleted []string
	respond func(*ron.Command) (int, string, bool)
}

var (
	_ fcCCServer = (*fcCloneFakeCC)(nil)
	_ fcCloneCC  = (*fcCloneFakeCC)(nil)
)

func newFCCloneFakeCC(t *testing.T, vm *FirecrackerVM) *fcCloneFakeCC {
	return &fcCloneFakeCC{
		t:     t,
		vm:    vm,
		cmds:  map[int]*ron.Command{},
		codes: map[int]int{},
		outs:  map[int]string{},
	}
}

// checkUnlocked fails the test when the caller holds vm.lock.
func (c *fcCloneFakeCC) checkUnlocked(op string) {
	done := make(chan struct{})
	go func() {
		c.vm.lock.Lock()
		c.vm.lock.Unlock()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		c.t.Errorf("%v called with vm.lock held", op)
	}
}

func (c *fcCloneFakeCC) RegisterVM(ron.VM) {}

func (c *fcCloneFakeCC) UnregisterVM(ron.VM) {}

func (c *fcCloneFakeCC) ListenUnixBound(path, uuid string, uid int) error { return nil }

func (c *fcCloneFakeCC) CloseUnix(path string) error { return nil }

func (c *fcCloneFakeCC) setClient(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.client = v
}

func (c *fcCloneFakeCC) setRespond(f func(*ron.Command) (int, string, bool)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.respond = f
}

func (c *fcCloneFakeCC) HasClient(uuid string) bool {
	c.checkUnlocked("HasClient")

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client && uuid == c.vm.UUID
}

func (c *fcCloneFakeCC) NewCommand(cmd *ron.Command) int {
	c.checkUnlocked("NewCommand")

	c.mu.Lock()
	defer c.mu.Unlock()

	c.next++
	cmd.ID = c.next
	c.cmds[cmd.ID] = cmd.Copy()
	c.sent = append(c.sent, cmd.Copy())

	if c.respond != nil {
		if code, out, ok := c.respond(cmd); ok {
			c.cmds[cmd.ID].CheckedIn = []string{c.vm.UUID}
			c.codes[cmd.ID] = code
			c.outs[cmd.ID] = out
		}
	}

	return cmd.ID
}

func (c *fcCloneFakeCC) GetCommand(id int) *ron.Command {
	c.checkUnlocked("GetCommand")

	c.mu.Lock()
	defer c.mu.Unlock()
	if cmd, ok := c.cmds[id]; ok {
		return cmd.Copy()
	}
	return nil
}

func (c *fcCloneFakeCC) GetExitCode(id int, uuid string) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	code, ok := c.codes[id]
	if !ok || uuid != c.vm.UUID {
		return 0, fmt.Errorf("no exit code for command %v", id)
	}
	return code, nil
}

func (c *fcCloneFakeCC) GetResponse(id int, raw bool) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.outs[id], nil
}

func (c *fcCloneFakeCC) DeleteCommand(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cmds, id)
	c.deleted = append(c.deleted, fmt.Sprintf("command %d", id))
	return nil
}

func (c *fcCloneFakeCC) DeleteResponse(id int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.deleted = append(c.deleted, fmt.Sprintf("response %d", id))
	return nil
}

func (c *fcCloneFakeCC) commands() []*ron.Command {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sent)
}

func (c *fcCloneFakeCC) deletes() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.deleted)
}

// fcCloneFixture is a launched restore of one snapshot.
type fcCloneFixture struct {
	f  *fcDriverFakes
	vm *FirecrackerVM
	cc *fcCloneFakeCC

	mu       sync.Mutex
	tapVLANs []int
	connects []string
}

func setupFCClone(t *testing.T, clone bool) *fcCloneFixture {
	t.Helper()

	f := setupFCDriver(t)
	setFCSnapshotSeams(t)

	oldRun, oldTap, oldConnect := fcRunCmd, fcCreateTap, fcCloneConnect
	oldTimeout, oldPoll := fcCloneTimeout, fcClonePoll
	t.Cleanup(func() {
		fcRunCmd, fcCreateTap, fcCloneConnect = oldRun, oldTap, oldConnect
		fcCloneTimeout, fcClonePoll = oldTimeout, oldPoll
	})

	fcCloneTimeout, fcClonePoll = 5*time.Second, 5*time.Millisecond
	fcRunCmd = func(args ...string) (string, error) { return "", nil }

	x := &fcCloneFixture{f: f}
	fcCreateTap = func(nic NetConfig, ns *bridge.Netns, index, uid int) (string, int, error) {
		x.mu.Lock()
		x.tapVLANs = append(x.tapVLANs, nic.VLAN)
		x.mu.Unlock()
		return oldTap(nic, ns, index, uid)
	}
	fcCloneConnect = func(vm *FirecrackerVM, i, vlan int, br string) error {
		x.mu.Lock()
		x.connects = append(x.connects, fmt.Sprintf("NIC %d VLAN %d bridge %v", i, vlan, br))
		x.mu.Unlock()
		vm.Networks[i].VLAN = vlan
		return nil
	}

	prefix := filepath.Join(f.dir, "snap", "src")
	fcWriteFile(t, prefix+".state", []byte("state"))
	fcWriteFile(t, prefix+".mem", []byte("mem"))
	fcWriteFile(t, prefix+".hdd", make([]byte, 4096))

	cfg := f.config(t)
	cfg.Disks = DiskConfigs{{Path: prefix + ".hdd"}}
	cfg.FirecrackerSnapshotPath = prefix
	cfg.FirecrackerClone = clone
	cfg.Networks[0].MAC = fcCloneSrcMAC

	data, err := json.Marshal(fcTestSnapshotInfo(cfg))
	if err != nil {
		t.Fatal(err)
	}
	fcWriteFile(t, prefix+".json", data)

	cfg.Networks[0].MAC = fcCloneMAC
	x.vm = f.newVM(t, cfg)
	x.cc = newFCCloneFakeCC(t, x.vm)

	err = x.vm.Launch()
	skipNoFIFO(t, err)
	if err != nil {
		t.Fatalf("Launch() = %v", err)
	}
	t.Cleanup(func() {
		if x.vm.GetState()&VM_KILLABLE != 0 {
			if err := x.vm.Kill(); err != nil {
				t.Errorf("Kill() = %v", err)
			}
			waitFCState(t, x.vm, VM_QUIT)
		}
		if err := x.vm.Flush(); err != nil {
			t.Errorf("Flush() = %v", err)
		}
	})
	waitFCState(t, x.vm, VM_BUILDING)

	return x
}

// start resumes the VM, then connects it to the fake cc server, as the
// namespace does.
func (x *fcCloneFixture) start(t *testing.T) {
	t.Helper()

	if err := x.vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitFCState(t, x.vm, VM_RUNNING)

	if err := x.vm.connectCC(x.cc); err != nil {
		t.Fatalf("connectCC() = %v", err)
	}
}

func (x *fcCloneFixture) record() ([]int, []string) {
	x.mu.Lock()
	defer x.mu.Unlock()
	return slices.Clone(x.tapVLANs), slices.Clone(x.connects)
}

func (x *fcCloneFixture) nic() NetConfig {
	return x.vm.GetNetworks()[0]
}

// waitFCCloneTag waits until the clone_identity tag of vm is not pending,
// and returns it.
func waitFCCloneTag(t *testing.T, vm *FirecrackerVM) string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		v := vm.Tag(fcCloneTag)
		if v != "pending" {
			return v
		}
		if time.Now().After(deadline) {
			t.Fatalf("%v tag is %q after 10s", fcCloneTag, v)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// waitFCCloneDeletes waits until the fake cc server has n deletes.
func waitFCCloneDeletes(t *testing.T, cc *fcCloneFakeCC, n int) []string {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for {
		d := cc.deletes()
		if len(d) >= n {
			return d
		}
		if time.Now().After(deadline) {
			t.Fatalf("cc deletes = %q, want %d", d, n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func fcCloneWantScript() string {
	return "set -e\n" + fcCloneNICFunc +
		"d0=$(mm_nic 06:00:ac:10:00:02)\n" +
		`ip link set dev "$d0" address 06:00:ac:10:02:01 2>/dev/null || { ip link set dev "$d0" down && ip link set dev "$d0" address 06:00:ac:10:02:01 && ip link set dev "$d0" up; }` + "\n" +
		"hostname -- 'fc1' || true\n" +
		"if pidof udhcpc >/dev/null 2>&1; then echo minimega-clone-dhcp; fi\n"
}

func checkFCCloneCommand(t *testing.T, vm *FirecrackerVM, c *ron.Command, script string) {
	t.Helper()

	if want := []string{"sh", "-c", script}; !reflect.DeepEqual(c.Command, want) {
		t.Errorf("command =\n%q\nwant\n%q", c.Command, want)
	}
	if c.Filter == nil || !reflect.DeepEqual(*c.Filter, ron.Filter{UUID: vm.UUID}) {
		t.Errorf("filter = %+v, want only UUID %v", c.Filter, vm.UUID)
	}
	if c.Prefix != fcCloneTag || c.Background {
		t.Errorf("prefix = %q, background = %v, want %q and false", c.Prefix, c.Background, fcCloneTag)
	}
}

func TestFCCloneIdentity(t *testing.T) {
	x := setupFCClone(t, true)

	if got, want := x.f.requests(1), []string{"GET /", "PUT /logger", "PUT /serial", "PUT /snapshot/load"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("restore requests =\n%q\nwant\n%q", got, want)
	}

	// The launch releases the read lock of the prefix.
	fcPrefixes.mu.Lock()
	locks := len(fcPrefixes.locks)
	fcPrefixes.mu.Unlock()
	if locks != 0 {
		t.Errorf("prefix locks after the launch = %d, want 0", locks)
	}

	// The clone keeps its MAC, and its tap has no bridge port.
	nic := x.nic()
	if nic.MAC != fcCloneMAC || nic.VLAN != DisconnectedVLAN || nic.Bridge != DefaultBridge {
		t.Errorf("NIC = MAC %v, VLAN %v, bridge %v, want %v, %v, %v", nic.MAC, nic.VLAN, nic.Bridge, fcCloneMAC, DisconnectedVLAN, DefaultBridge)
	}
	if v, _ := x.vm.Info("vlan"); v != "[disconnected]" {
		t.Errorf("Info(vlan) = %v, want [disconnected]", v)
	}
	if taps, _ := x.record(); !reflect.DeepEqual(taps, []int{DisconnectedVLAN}) {
		t.Errorf("tap VLANs = %v, want [%v]", taps, DisconnectedVLAN)
	}

	x.cc.setRespond(func(*ron.Command) (int, string, bool) { return 0, "", true })
	x.start(t)

	// The identity waits for the cc client.
	time.Sleep(50 * time.Millisecond)
	if v := x.vm.Tag(fcCloneTag); v != "pending" {
		t.Errorf("%v tag before the cc client = %q, want pending", fcCloneTag, v)
	}
	if got := x.cc.commands(); len(got) != 0 {
		t.Errorf("commands before the cc client = %v", got)
	}
	if err := x.vm.Save(filepath.Join(t.TempDir(), "s")); err == nil || !strings.Contains(err.Error(), "is a clone without its identity") {
		t.Errorf("Save() of a pending clone = %v, want a clone error", err)
	}

	x.cc.setClient(true)
	if v := waitFCCloneTag(t, x.vm); v != "ok" {
		t.Fatalf("%v tag = %q, want ok", fcCloneTag, v)
	}

	cmds := x.cc.commands()
	if len(cmds) != 1 {
		t.Fatalf("commands = %d, want 1", len(cmds))
	}
	checkFCCloneCommand(t, x.vm, cmds[0], fcCloneWantScript())
	if got := waitFCCloneDeletes(t, x.cc, 2); !reflect.DeepEqual(got, []string{"command 1", "response 1"}) {
		t.Errorf("deletes = %q", got)
	}

	if _, connects := x.record(); !reflect.DeepEqual(connects, []string{"NIC 0 VLAN 101 bridge " + DefaultBridge}) {
		t.Errorf("connects = %q", connects)
	}
	if nic := x.nic(); nic.VLAN != 101 || nic.MAC != fcCloneMAC {
		t.Errorf("NIC after the identity = VLAN %v, MAC %v", nic.VLAN, nic.MAC)
	}
}

func TestFCCloneIdentityDHCP(t *testing.T) {
	x := setupFCClone(t, true)

	x.cc.setRespond(func(c *ron.Command) (int, string, bool) {
		if c.ID == 1 {
			return 0, fcCloneDHCPMarker + "\n", true
		}
		return 0, "", true
	})
	x.cc.setClient(true)
	x.start(t)

	if v := waitFCCloneTag(t, x.vm); v != "ok" {
		t.Fatalf("%v tag = %q, want ok", fcCloneTag, v)
	}
	waitFCCloneDeletes(t, x.cc, 4)

	cmds := x.cc.commands()
	if len(cmds) != 2 {
		t.Fatalf("commands = %d, want 2", len(cmds))
	}
	want := "r=0\n" + fcCloneNICFunc + `d=$(mm_nic 06:00:ac:10:02:01) && udhcpc -i "$d" -n -q -t 3 || r=1` + "\nexit $r\n"
	checkFCCloneCommand(t, x.vm, cmds[1], want)
}

func TestFCCloneIdentityTimeout(t *testing.T) {
	x := setupFCClone(t, true)
	fcCloneTimeout = 100 * time.Millisecond

	x.start(t)
	if v := waitFCCloneTag(t, x.vm); !strings.Contains(v, "no cc client connected in 100ms") {
		t.Fatalf("%v tag = %q, want a timeout error", fcCloneTag, v)
	}

	if got := x.cc.commands(); len(got) != 0 {
		t.Errorf("commands = %v, want none", got)
	}
	if _, connects := x.record(); len(connects) != 0 {
		t.Errorf("connects = %q, want none", connects)
	}
	if nic := x.nic(); nic.VLAN != DisconnectedVLAN {
		t.Errorf("VLAN = %v, want %v", nic.VLAN, DisconnectedVLAN)
	}
}

func TestFCCloneIdentityFailure(t *testing.T) {
	x := setupFCClone(t, true)

	x.cc.setRespond(func(*ron.Command) (int, string, bool) {
		return 1, "no NIC has MAC " + fcCloneSrcMAC + "\n", true
	})
	x.cc.setClient(true)
	x.start(t)

	v := waitFCCloneTag(t, x.vm)
	if want := "cc command exited with status 1: no NIC has MAC " + fcCloneSrcMAC; v != want {
		t.Fatalf("%v tag = %q, want %q", fcCloneTag, v, want)
	}
	if got := waitFCCloneDeletes(t, x.cc, 2); !reflect.DeepEqual(got, []string{"command 1", "response 1"}) {
		t.Errorf("deletes = %q", got)
	}
	if _, connects := x.record(); len(connects) != 0 {
		t.Errorf("connects = %q, want none", connects)
	}
	if nic := x.nic(); nic.VLAN != DisconnectedVLAN {
		t.Errorf("VLAN = %v, want %v", nic.VLAN, DisconnectedVLAN)
	}
	if err := x.vm.Save(filepath.Join(t.TempDir(), "s")); err == nil || !strings.Contains(err.Error(), "is a clone without its identity") {
		t.Errorf("Save() of a failed clone = %v, want a clone error", err)
	}

	// A relaunch boots the disk with the configured MAC, so the NIC connects.
	if err := x.vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	waitFCState(t, x.vm, VM_QUIT)
	if err := x.vm.Start(); err != nil {
		t.Fatalf("Start() after Kill() = %v", err)
	}
	waitFCState(t, x.vm, VM_RUNNING)
	if _, connects := x.record(); !reflect.DeepEqual(connects, []string{"NIC 0 VLAN 101 bridge " + DefaultBridge}) {
		t.Errorf("connects after the relaunch = %q", connects)
	}
	if nic := x.nic(); nic.VLAN != 101 {
		t.Errorf("VLAN after the relaunch = %v, want 101", nic.VLAN)
	}
}

func TestFCCloneIdentityPaused(t *testing.T) {
	x := setupFCClone(t, true)
	fcCloneTimeout = 100 * time.Millisecond

	x.start(t)
	if err := x.vm.Stop(); err != nil {
		t.Fatalf("Stop() = %v", err)
	}
	waitFCState(t, x.vm, VM_PAUSED)

	// The time while the VM is paused does not count.
	time.Sleep(300 * time.Millisecond)
	if v := x.vm.Tag(fcCloneTag); v != "pending" {
		t.Fatalf("%v tag after a pause = %q, want pending", fcCloneTag, v)
	}

	x.cc.setRespond(func(*ron.Command) (int, string, bool) { return 0, "", true })
	x.cc.setClient(true)
	if err := x.vm.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	if v := waitFCCloneTag(t, x.vm); v != "ok" {
		t.Fatalf("%v tag = %q, want ok", fcCloneTag, v)
	}
}

func TestFCCloneIdentityNoResponse(t *testing.T) {
	x := setupFCClone(t, true)
	fcCloneTimeout = 100 * time.Millisecond

	x.cc.setClient(true)
	x.start(t)
	if v := waitFCCloneTag(t, x.vm); !strings.Contains(v, "no response to cc command 1 in 100ms") {
		t.Fatalf("%v tag = %q, want a timeout error", fcCloneTag, v)
	}
	if got := waitFCCloneDeletes(t, x.cc, 2); !reflect.DeepEqual(got, []string{"command 1", "response 1"}) {
		t.Errorf("deletes = %q", got)
	}
	if nic := x.nic(); nic.VLAN != DisconnectedVLAN {
		t.Errorf("VLAN = %v, want %v", nic.VLAN, DisconnectedVLAN)
	}
}

func TestFCCloneIdentityKill(t *testing.T) {
	x := setupFCClone(t, true)
	x.start(t)

	if err := x.vm.Kill(); err != nil {
		t.Fatalf("Kill() = %v", err)
	}
	if v := waitFCCloneTag(t, x.vm); v != "the VM is QUIT" {
		t.Fatalf("%v tag = %q, want the VM is QUIT", fcCloneTag, v)
	}

	// The guest is gone, so the NIC gets its VLAN back for the next start.
	if _, connects := x.record(); !reflect.DeepEqual(connects, []string{"NIC 0 VLAN 101 bridge " + DefaultBridge}) {
		t.Errorf("connects = %q", connects)
	}
	if nic := x.nic(); nic.VLAN != 101 {
		t.Errorf("VLAN = %v, want 101", nic.VLAN)
	}
}

func TestFCRestoreNotClone(t *testing.T) {
	x := setupFCClone(t, false)

	nic := x.nic()
	if nic.MAC != fcCloneSrcMAC || nic.VLAN != 101 {
		t.Errorf("NIC = MAC %v, VLAN %v, want the snapshot MAC %v and VLAN 101", nic.MAC, nic.VLAN, fcCloneSrcMAC)
	}
	if taps, _ := x.record(); !reflect.DeepEqual(taps, []int{101}) {
		t.Errorf("tap VLANs = %v, want [101]", taps)
	}

	x.cc.setClient(true)
	x.start(t)

	x.vm.lock.Lock()
	clone := x.vm.clone
	x.vm.lock.Unlock()
	if clone != nil {
		t.Errorf("a restore without firecracker-clone has a clone identity")
	}
	if v := x.vm.Tag(fcCloneTag); v != "" {
		t.Errorf("%v tag = %q, want none", fcCloneTag, v)
	}
	if got := x.cc.commands(); len(got) != 0 {
		t.Errorf("commands = %v, want none", got)
	}
}

func TestFCValidateClone(t *testing.T) {
	base := func() VMConfig {
		cfg := NewVMConfig()
		cfg.KernelPath = "/k/vmlinux"
		cfg.FirecrackerSnapshotPath = "/s/src"
		cfg.FirecrackerClone = true
		cfg.Backchannel = true
		return cfg
	}

	if w, err := fcValidateConfig("clone1", base()); err != nil || len(w) != 0 {
		t.Errorf("fcValidateConfig() = %q, %v, want no warnings and no error", w, err)
	}

	cfg := base()
	cfg.Backchannel = false
	if _, err := fcValidateConfig("clone1", cfg); err == nil || !strings.Contains(err.Error(), "firecracker-clone needs vm config backchannel true") {
		t.Errorf("fcValidateConfig() without backchannel = %v", err)
	}

	long := strings.Repeat("c", fcMaxHostname+1)
	if _, err := fcValidateConfig(long, base()); err == nil || !strings.Contains(err.Error(), "the limit is 64 bytes") {
		t.Errorf("fcValidateConfig() with a long name = %v", err)
	}

	cfg = base()
	cfg.FirecrackerSnapshotPath = ""
	cfg.Backchannel = false
	w, err := fcValidateConfig(long, cfg)
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "firecracker-clone has no effect without firecracker-snapshot") {
		t.Errorf("fcValidateConfig() without a snapshot = %q, %v", w, err)
	}

	// vm launch refuses a clone without backchannel before it reads the
	// snapshot.
	f := setupFCDriver(t)
	cfg = f.config(t)
	cfg.Backchannel = false
	cfg.FirecrackerSnapshotPath = "/snap/vm"
	cfg.FirecrackerClone = true
	vm := f.newVM(t, cfg)
	if err := vm.Launch(); err == nil || !strings.Contains(err.Error(), "needs vm config backchannel true") {
		t.Fatalf("Launch() = %v, want a backchannel error", err)
	}
	if err := vm.Flush(); err != nil {
		t.Fatalf("Flush() = %v", err)
	}
}

func TestFCCloneScripts(t *testing.T) {
	if got, want := fcCloneScript([]string{fcCloneSrcMAC}, []string{fcCloneMAC}, "fc1"), fcCloneWantScript(); got != want {
		t.Errorf("fcCloneScript() =\n%v\nwant\n%v", got, want)
	}

	if got := fcShellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("fcShellQuote() = %v", got)
	}

	long := strings.Repeat("c", fcMaxHostname)
	if got := fcCloneScript(nil, nil, long); !strings.Contains(got, "hostname -- '"+long[:63]+"' || true\n") {
		t.Errorf("fcCloneScript() with a %d-byte name =\n%v", len(long), got)
	}
	if got := fcCloneHostname(strings.Repeat("c", 62) + "é"); got != strings.Repeat("c", 62) {
		t.Errorf("fcCloneHostname() = %q, want no part of a character", got)
	}

	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skipf("no sh: %v", err)
	}
	for _, s := range []string{
		fcCloneScript([]string{"06:00:00:00:00:01", "06:00:00:00:00:02"}, []string{"06:00:00:00:01:01", "06:00:00:00:01:02"}, "it's"),
		fcCloneDHCPScript([]string{"06:00:00:00:01:01", "06:00:00:00:01:02"}),
	} {
		if out, err := exec.Command(sh, "-n", "-c", s).CombinedOutput(); err != nil {
			t.Errorf("sh -n:\n%v\n%s: %v", s, out, err)
		}
	}

	// A failed host name step does not stop the script.
	script := "set -e\nhostname() { return 1; }\n" + "hostname -- 'fc1' || true" + "\necho after\n"
	if out, err := exec.Command(sh, "-c", script).CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "after" {
		t.Errorf("host name step that fails = %q, %v, want after", out, err)
	}

	// mm_nic fails for a MAC that no NIC has.
	out, err := exec.Command(sh, "-c", fcCloneNICFunc+"mm_nic 06:00:00:00:00:fe").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "no NIC has MAC 06:00:00:00:00:fe") {
		t.Errorf("mm_nic of an unknown MAC = %q, %v", out, err)
	}
}
