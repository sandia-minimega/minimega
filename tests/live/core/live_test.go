// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
	"github.com/twmb/murmur3"
)

func TestMain(m *testing.M) {
	if err := harness.Connect(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// TestLive runs every check against the topology loaded from topology.mm.
// Subtests run in order: early phases establish that the experiment is up,
// later phases mutate it, and the final phase tears it down.
func TestLive(t *testing.T) {
	// What each phase needs to have passed first. Phases that need nothing
	// beyond the inventory still run when the guests do not come up.
	version := []string{"Version"}
	inv := []string{"Inventory"}
	cc := []string{"CCClients"}
	network := []string{"CCClients", "DHCP"}

	// p builds a phase; harness.Phase is keyed so an unkeyed literal would fail vet.
	p := func(name string, fn func(*testing.T), needs []string) harness.Phase {
		return harness.Phase{Name: name, Fn: fn, Needs: needs}
	}

	harness.RunPhases(t, []harness.Phase{
		// control plane
		p("Version", testVersion, nil),
		p("Check", testCheck, version),
		p("Host", testHost, version),
		p("Namespace", testNamespace, version),
		p("Inventory", testInventory, []string{"Namespace"}),
		p("VMConfig", testVMConfig, inv),
		p("VMInfoSummary", testVMInfoSummary, inv),
		p("VLANs", testVLANs, inv),
		p("Bridge", testBridge, inv),
		p("FileList", testFileList, inv),
		p("FileHash", testFileHash, version),
		p("Disk", testDisk, inv),
		p("Miniweb", testMiniweb, inv),

		// guests come up
		p("CCClients", testCCClients, inv),
		p("DHCP", testDHCP, inv),

		// command and control
		p("CCExec", testCCExec, cc),
		p("CCFilterTag", testCCFilterTag, cc),
		p("CCExitCode", testCCExitCode, cc),
		p("CCSendRecv", testCCSendRecv, cc),
		p("CCBackgroundProcess", testCCBackgroundProcess, cc),
		p("CCResponses", testCCResponses, cc),

		// what the guest sees of its VM configuration
		p("GuestQemuAppend", testGuestQemuAppend, cc),
		p("GuestDisks", testGuestDisks, cc),
		p("GuestMachineType", testGuestMachineType, cc),
		p("GuestSMP", testGuestSMP, cc),
		p("GuestNICDrivers", testGuestNICDrivers, cc),
		p("GuestDualHomed", testGuestDualHomed, network),

		// networking
		p("PingSameLAN", testPingSameLAN, network),
		p("OSPF", testOSPF, network),
		p("PingAcrossRouters", testPingAcrossRouters, network),
		p("NoBackchannelHostReachable", testNoBackchannelHost, network),
		p("DNS", testDNS, network),
		p("HTTPProtonuke", testHTTPProtonuke, network),
		p("Firewall", testFirewall, network),
		p("HostTap", testHostTap, network),
		p("QoS", testQoS, network),
		p("CapturePcap", testCapturePcap, network),

		// VM operations
		p("Screenshot", testScreenshot, inv),
		p("Serial", testSerial, inv),
		p("QMP", testQMP, inv),
		p("VNCShim", testVNCShim, inv),
		p("VNCRecordFB", testVNCRecordFB, inv),
		p("VNCKeyboard", testVNCKeyboard, cc),
		p("VNCType", testVNCType, cc),
		p("VNCInjectLoadFile", testVNCInjectLoadFile, cc),
		p("VNCKill", testVNCKill, inv),
		p("Tags", testTags, cc),
		p("Top", testTop, inv),
		p("StopStart", testStopStart, cc),
		p("NetDisconnectConnect", testNetDisconnectConnect, network),
		p("Hotplug", testHotplug, cc),
		p("Save", testSave, cc),
		p("KillFlushRelaunch", testKillFlushRelaunch, cc),

		// tools shipped in the image
		p("Passwordify", testPasswordify, cc),

		// optimizations: last, so that their VMs do not change the VM counts
		// of the other phases
		p("Hugepages", testHugepages, inv),

		// teardown
		p("ClearNamespace", testClearNamespace, version),
	})
}

// --- control plane -----------------------------------------------------------

func testVersion(t *testing.T) {
	v := harness.MustText(t, "version")
	if !strings.Contains(v, "minimega") {
		t.Fatalf("unexpected version: %q", v)
	}
	t.Log(strings.TrimSpace(v))
}

func testCheck(t *testing.T) {
	// "check" reports missing external dependencies as errors.
	if _, err := harness.Run("check"); err != nil {
		t.Errorf("dependency check failed: %v", err)
	}
}

func testHost(t *testing.T) {
	rows := harness.MustTable(t, "host")
	if len(rows) != 1 {
		t.Fatalf("expected 1 host, got %d", len(rows))
	}
	h := rows[0]
	if n, _ := strconv.Atoi(h["cpus"]); n < 1 {
		t.Errorf("host cpus = %q", h["cpus"])
	}
	if vms, _ := strconv.Atoi(h["vms"]); vms < len(topology) {
		t.Errorf("host vms = %v, want >= %d", h["vms"], len(topology))
	}
	if mem, _ := strconv.Atoi(h["memused"]); mem <= 0 {
		t.Errorf("host memused = %q", h["memused"])
	}
}

func testNamespace(t *testing.T) {
	found := false
	for _, r := range harness.MustTable(t, "namespace") {
		if r["namespace"] == harness.Namespace {
			found = true
		}
	}
	if !found {
		t.Fatalf("namespace %q not listed", harness.Namespace)
	}
	if hosts := harness.MustText(t, "ns hosts"); strings.TrimSpace(hosts) == "" {
		t.Errorf("namespace has no hosts")
	}
	if q := strings.TrimSpace(harness.MustText(t, "ns queueing")); q != "true" {
		t.Errorf("ns queueing = %q, want true (set by topology.mm)", q)
	}
	// "vm launch" flushed the queue
	if q := strings.TrimSpace(harness.MustText(t, "ns queue")); q != "" {
		t.Errorf("queue not empty after flush: %q", q)
	}
}

func testInventory(t *testing.T) {
	harness.Eventually(t, 3*time.Minute, 5*time.Second, func() error {
		vms := harness.VMInfo(t)
		if len(vms) != len(topology) {
			return fmt.Errorf("got %d VMs, want %d (%v)", len(vms), len(topology), harness.Keys(vms))
		}
		for _, s := range topology {
			r, ok := vms[s.Name]
			if !ok {
				return fmt.Errorf("missing VM %v", s.Name)
			}
			if r["type"] != "kvm" {
				return fmt.Errorf("%v: type %v", s.Name, r["type"])
			}
			if r["state"] != "RUNNING" {
				return fmt.Errorf("%v: state %v (error tag: %v)", s.Name, r["state"], r["tags"])
			}
		}
		return nil
	})
}

func testVMConfig(t *testing.T) {
	vms := harness.VMInfo(t)

	for _, s := range topology {
		t.Run(s.Name, func(t *testing.T) {
			r := vms[s.Name]
			if r["memory"] != s.Memory {
				t.Errorf("memory = %v, want %v", r["memory"], s.Memory)
			}
			if r["vcpus"] != s.VCPUs {
				t.Errorf("vcpus = %v, want %v", r["vcpus"], s.VCPUs)
			}
			if r["snapshot"] != "true" {
				t.Errorf("snapshot = %v", r["snapshot"])
			}
			if !strings.HasSuffix(r["kernel"], "live/live.kernel") || !strings.HasSuffix(r["initrd"], "live/live.initrd") {
				t.Errorf("kernel/initrd = %v / %v", r["kernel"], r["initrd"])
			}

			vlans := harness.List(r["vlan"])
			if len(vlans) != len(s.VLANs) {
				t.Fatalf("vlan = %v, want aliases %v", r["vlan"], s.VLANs)
			}
			for i, alias := range s.VLANs {
				if !strings.HasPrefix(vlans[i], alias+" (") {
					t.Errorf("nic %d vlan = %v, want alias %v", i, vlans[i], alias)
				}
			}
			for _, b := range harness.List(r["bridge"]) {
				if b != "mega_bridge" {
					t.Errorf("bridge = %v", b)
				}
			}

			var tags map[string]string
			if err := json.Unmarshal([]byte(r["tags"]), &tags); err != nil {
				t.Fatalf("tags %q: %v", r["tags"], err)
			}
			for k, v := range s.Tags {
				if tags[k] != v {
					t.Errorf("tag %v = %q, want %q", k, tags[k], v)
				}
			}
			if tags["topology"] != "live" {
				t.Errorf("tag topology = %q (from saved config live-base)", tags["topology"])
			}

			wantCC := "live.role=host"
			if s.Router {
				wantCC = "live.role=router"
			}
			if !strings.Contains(r["append"], wantCC) {
				t.Errorf("append = %q", r["append"])
			}
		})
	}

	// one-off options
	check := func(vm, col, want string) {
		t.Helper()
		if got := vms[vm][col]; !strings.Contains(got, want) {
			t.Errorf("%v %v = %q, want it to contain %q", vm, col, got, want)
		}
	}
	check("host-a1", "serial-ports", "1")
	check("host-a1", "virtio-ports", "live.port0")
	check("host-c1", "disks", "data.qcow2")
	check("host-c1", "disks", "virtio")
	check("host-c2", "disks", "ide")
	check("host-c2", "disks", "writeback")
	check("host-c5", "usb-use-xhci", "true")
	check("host-d5", "uuid", d5UUID)
	for i := 1; i <= 5; i++ {
		check("host-b"+itoa(i), "mac", "00:1f:53:00:02:1"+itoa(i))
	}
	if got := vms["host-a2"]["serial-ports"]; got != "0" && got != "" {
		t.Errorf("host-a2 serial-ports = %q, want none", got)
	}
}

func testVMInfoSummary(t *testing.T) {
	rows := harness.MustTable(t, "vm info summary")
	if len(rows) != len(topology) {
		t.Fatalf("summary rows = %d", len(rows))
	}
	if _, ok := rows[0]["memory"]; ok {
		t.Errorf("summary should not include memory column")
	}
	for _, col := range []string{"name", "state", "uuid", "cc_active", "vlan"} {
		if _, ok := rows[0][col]; !ok {
			t.Errorf("summary missing column %v", col)
		}
	}
}

func testVLANs(t *testing.T) {
	got := map[string]string{}
	for _, r := range harness.MustTable(t, "vlans") {
		got[r["alias"]] = r["vlan"]
	}
	if got["lan-a"] != "1001" {
		t.Errorf("lan-a = %q, want 1001 (vlans add)", got["lan-a"])
	}
	seen := map[string]string{}
	for _, alias := range []string{"lan-a", "lan-b", "lan-c", "lan-d"} {
		v, ok := got[alias]
		if !ok {
			t.Errorf("alias %v missing: %v", alias, got)
			continue
		}
		if other, dup := seen[v]; dup {
			t.Errorf("%v and %v share VLAN %v", alias, other, v)
		}
		seen[v] = alias
	}
}

func testBridge(t *testing.T) {
	rows := harness.MustTable(t, "bridge")
	found := false
	for _, r := range rows {
		if r["bridge"] == "mega_bridge" {
			found = true
		}
	}
	if !found {
		t.Errorf("mega_bridge not listed: %v", rows)
	}

	out, err := harness.InContainer(t, "ovs-vsctl", "list-ports", "mega_bridge")
	if err != nil {
		t.Fatalf("ovs-vsctl: %v: %s", err, out)
	}
	ports := strings.Fields(out)
	// every VM NIC plus the live-mgmt host tap
	if len(ports) < totalNICs+1 {
		t.Errorf("mega_bridge has %d ports, want >= %d: %v", len(ports), totalNICs+1, ports)
	}

	// every VM tap is tagged with its VLAN in OVS
	vms := harness.VMInfo(t)
	vlanOf := map[string]string{}
	for _, r := range harness.MustTable(t, "vlans") {
		vlanOf[r["alias"]] = r["vlan"]
	}
	for _, s := range []string{"host-a1", "rtr-cd"} {
		checkOVSTags(t, s, harness.List(vms[s]["tap"]), vlanOf)
	}
}

// checkOVSTags checks that each of a VM's taps is tagged in OVS with the VLAN
// of the matching NIC in its spec.
func checkOVSTags(t *testing.T, vm string, taps []string, vlanOf map[string]string) {
	t.Helper()
	vlans := specFor(vm).VLANs
	if len(taps) != len(vlans) {
		t.Errorf("%v has taps %v, want one per VLAN in %v", vm, taps, vlans)
		return
	}
	for i, tap := range taps {
		out, err := harness.InContainer(t, "ovs-vsctl", "get", "port", tap, "tag")
		if err != nil {
			t.Errorf("ovs tag %v: %v %s", tap, err, out)
			continue
		}
		if want := vlanOf[vlans[i]]; strings.TrimSpace(out) != want {
			t.Errorf("%v nic %d tap %v tag = %q, want %v", vm, i, tap, strings.TrimSpace(out), want)
		}
	}
}

func testFileList(t *testing.T) {
	rows := harness.MustTable(t, "file list live")
	names := map[string]bool{}
	for _, r := range rows {
		names[strings.TrimPrefix(r["name"], "/")] = true
	}
	for _, f := range []string{"live/live.kernel", "live/live.initrd", "live/topology.mm", "live/data.qcow2"} {
		if !names[f] {
			t.Errorf("file list missing %v (have %v)", f, harness.Keys(names))
		}
	}
}

// fileHash returns the "file list" hash column for a file under live/, and
// whether the file is listed at all.
func fileHash(t *testing.T, name string) (string, bool) {
	t.Helper()
	for _, r := range harness.MustTable(t, "file list live") {
		if strings.TrimPrefix(r["name"], "/") == "live/"+name {
			if _, ok := r["hash"]; !ok {
				t.Fatalf("file list has no hash column: minimega must run with -hashfiles (tests/live/run.sh)")
			}
			return r["hash"], true
		}
	}
	return "", false
}

// waitFileHash waits for a file under live/ to be listed with the wanted hash.
func waitFileHash(t *testing.T, name, want string) {
	t.Helper()
	harness.Eventually(t, time.Minute, 500*time.Millisecond, func() error {
		if got, ok := fileHash(t, name); !ok || got != want {
			return fmt.Errorf("%v: hash %q (listed: %v), want %q", name, got, ok, want)
		}
		return nil
	})
}

func murmurHex(b []byte) string {
	h := murmur3.New64()
	h.Write(b)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// testFileHash checks -hashfiles on files copied into the files directory:
// a file is hashed once it stops changing, not on every write to it
// (sandia-minimega/minimega#1677), and the hash follows changes and renames.
func testFileHash(t *testing.T) {
	dir := harness.MMFiles + "/live/"
	names := []string{"hash-copy.img", "hash-moved.img", "hash-dir/sub/nested.img", "hash-dir"}
	defer func() {
		for _, n := range names {
			os.RemoveAll(dir + n)
		}
	}()

	// files staged before minimega started are hashed at startup
	initrd, err := os.ReadFile(dir + "live.initrd")
	if err != nil {
		t.Fatal(err)
	}
	waitFileHash(t, "live.initrd", murmurHex(initrd))

	// what minimega uses when no file is being copied
	const window = 8 * time.Second
	idle := harness.MinimegaCPU(t)
	time.Sleep(window)
	idle = harness.MinimegaCPU(t) - idle

	// Copy 256 MiB the way a copy from another file system or over the
	// network arrives: thousands of small writes spread over several seconds.
	const (
		chunks = 2048
		size   = 128 << 10
	)
	chunk := make([]byte, size)
	for i := range chunk {
		chunk[i] = byte(i * 7)
	}
	f, err := os.Create(dir + "hash-copy.img")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	sum := murmur3.New64()
	before, start := harness.MinimegaCPU(t), time.Now()
	for i := 0; i < chunks; i++ {
		chunk[0] = byte(i)
		if _, err := f.Write(chunk); err != nil {
			t.Fatal(err)
		}
		sum.Write(chunk)
		if i == chunks/2 {
			// a half-written file must not be served with a hash
			if got, ok := fileHash(t, "hash-copy.img"); !ok || got != "" {
				t.Errorf("mid-copy: hash %q (listed: %v), want listed with no hash", got, ok)
			}
		}
		// pace the copy over the same window the idle sample covered
		if wait := time.Duration(i+1)*window/chunks - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("%x", sum.Sum(nil))
	waitFileHash(t, "hash-copy.img", want)
	used := harness.MinimegaCPU(t) - before
	t.Logf("minimega CPU: %v idle, %v during a %v copy of %d MiB", idle, used, time.Since(start).Round(time.Second), chunks*size>>20)

	// Hashing the file once takes well under a second. Rehashing it on every
	// write kept every core busy for the whole copy.
	if used > idle+4*time.Second {
		t.Errorf("minimega used %v of CPU during the copy (%v when idle): is it hashing on every write?", used, idle)
	}

	// appending changes the hash
	a, err := os.OpenFile(dir+"hash-copy.img", os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	sum.Write([]byte("more"))
	if _, err := a.Write([]byte("more")); err != nil {
		t.Fatal(err)
	}
	a.Close()
	appended := fmt.Sprintf("%x", sum.Sum(nil))
	waitFileHash(t, "hash-copy.img", appended)

	// a rename moves the hash to the new name
	if err := os.Rename(dir+"hash-copy.img", dir+"hash-moved.img"); err != nil {
		t.Fatal(err)
	}
	waitFileHash(t, "hash-moved.img", appended)
	if _, ok := fileHash(t, "hash-copy.img"); ok {
		t.Errorf("renamed file is still listed under its old name")
	}

	// a file written into directories created a moment earlier is hashed too
	if err := os.MkdirAll(dir+"hash-dir/sub", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"hash-dir/sub/nested.img", []byte("nested"), 0644); err != nil {
		t.Fatal(err)
	}
	harness.Eventually(t, time.Minute, 500*time.Millisecond, func() error {
		for _, r := range harness.MustTable(t, "file list live/hash-dir/sub") {
			if strings.HasSuffix(r["name"], "nested.img") && r["hash"] == murmurHex([]byte("nested")) {
				return nil
			}
		}
		return fmt.Errorf("live/hash-dir/sub/nested.img has no hash yet")
	})
}

func testDisk(t *testing.T) {
	rows := harness.MustTable(t, "disk info live/data.qcow2")
	if len(rows) != 1 {
		t.Fatalf("disk info rows: %v", rows)
	}
	r := rows[0]
	if r["format"] != "qcow2" {
		t.Errorf("format = %q", r["format"])
	}
	if !strings.Contains(r["virtualsize"], "64") {
		t.Errorf("virtual size = %q", r["virtualsize"])
	}
	if r["inuse"] != "true" {
		t.Errorf("inuse = %q, want true (attached to host-c1 and host-c2)", r["inuse"])
	}

	// snapshot then inspect the backing chain
	harness.MustRun(t, "disk snapshot live/data.qcow2 live/data-snap.qcow2")
	snap := harness.MustTable(t, "disk info live/data-snap.qcow2")
	if len(snap) == 0 || !strings.Contains(snap[0]["backingfile"], "data.qcow2") {
		t.Errorf("snapshot backing file: %v", snap)
	}
}

func testMiniweb(t *testing.T) {
	get := func(path string) []byte {
		t.Helper()
		resp, err := http.Get(harness.MiniwebURL + path)
		if err != nil {
			t.Fatalf("GET %v: %v", path, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %v: %v", path, resp.Status)
		}
		return body
	}

	if body := get("/"); !bytes.Contains(bytes.ToLower(body), []byte("minimega")) {
		t.Errorf("index page does not mention minimega")
	}

	// miniweb talks to minimega over the same socket and lists our VMs
	body := get("/vms/info.json?namespace=" + harness.Namespace)
	for _, s := range topology {
		if !bytes.Contains(body, []byte(`"`+s.Name+`"`)) {
			t.Errorf("/vms/info.json missing %v", s.Name)
		}
	}
}

// --- guests come up ----------------------------------------------------------

func testCCClients(t *testing.T) {
	want := ccVMs()
	harness.Eventually(t, harness.SettleTimeout, 5*time.Second, func() error {
		vms := harness.VMInfo(t)
		var missing []string
		for _, s := range want {
			if vms[s.Name]["cc_active"] != "true" {
				missing = append(missing, s.Name)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("cc not active on %v", missing)
		}
		return nil
	})

	clients := harness.MustTable(t, "cc clients")
	if len(clients) != len(want) {
		t.Errorf("cc clients = %d, want %d", len(clients), len(want))
	}
	for _, c := range clients {
		if c["os"] != "linux" || c["arch"] != "amd64" {
			t.Errorf("client %v reports %v/%v", c["hostname"], c["os"], c["arch"])
		}
	}

	if got := harness.VMInfo(t)["host-d4"]["cc_active"]; got != "false" {
		t.Errorf("host-d4 (backchannel false) cc_active = %v", got)
	}
}

func testDHCP(t *testing.T) {
	// minimega learns guest addresses by snooping traffic on each tap, so
	// this also covers the no-backchannel host.
	harness.Eventually(t, harness.SettleTimeout, 5*time.Second, func() error {
		vms := harness.VMInfo(t)
		var bad []string
		for _, s := range topology {
			ips := harness.List(vms[s.Name]["ip"])
			for i, want := range s.IPs {
				got := ""
				if i < len(ips) {
					got = ips[i]
				}
				ok := got == want
				if want == "" {
					ok = strings.HasPrefix(got, s.Subnet)
				}
				if !ok {
					bad = append(bad, fmt.Sprintf("%v[%d]=%q", s.Name, i, got))
				}
			}
		}
		if len(bad) > 0 {
			return fmt.Errorf("unexpected addresses: %v", bad)
		}
		return nil
	})

	// dynamic leases fall inside the configured range
	for i := 1; i <= 5; i++ {
		name := "host-a" + itoa(i)
		ip := harness.List(harness.VMInfo(t)[name]["ip"])[0]
		last, _ := strconv.Atoi(ip[strings.LastIndex(ip, ".")+1:])
		if last < 100 || last > 199 {
			t.Errorf("%v leased %v outside 10.10.1.100-199", name, ip)
		}
	}
}

// --- command and control -----------------------------------------------------

func testCCExec(t *testing.T) {
	out := harness.CCOK(t, "host-b1", "hostname; uname -s")
	if !strings.Contains(out, "host-b1") || !strings.Contains(out, "Linux") {
		t.Errorf("unexpected output: %q", out)
	}

	// broadcast to every client
	res, _ := harness.CCRun(t, "", "exec hostname", len(ccVMs()), 2*time.Minute)
	for _, s := range ccVMs() {
		r, ok := res[s.Name]
		if !ok {
			t.Errorf("no response from %v", s.Name)
			continue
		}
		if got := strings.TrimSpace(r.Stdout); got != s.hostname() {
			t.Errorf("%v reports hostname %q, want %q", s.Name, got, s.hostname())
		}
	}
}

func testCCFilterTag(t *testing.T) {
	var servers []string
	for _, s := range ccVMs() {
		if s.Tags["role"] == "server" {
			servers = append(servers, s.Name)
		}
	}
	res, _ := harness.CCRun(t, "role=server", "exec true", len(servers), 2*time.Minute)
	if len(res) != len(servers) {
		t.Errorf("role=server reached %v, want %v", harness.Keys(res), servers)
	}
	for name := range res {
		if specFor(name).Tags["role"] != "server" {
			t.Errorf("filter leaked to %v", name)
		}
	}

	// filter on a "vm info" column
	res, _ = harness.CCRun(t, "name=host-c2", "exec true", 1, time.Minute)
	if _, ok := res["host-c2"]; !ok || len(res) != 1 {
		t.Errorf("name filter reached %v", harness.Keys(res))
	}
}

func testCCExitCode(t *testing.T) {
	res, id := harness.CCRun(t, "name=host-b2", `exec sh -c "exit 3"`, 1, time.Minute)
	if res["host-b2"].ExitCode != 3 {
		t.Errorf("exit code from responses = %d, want 3", res["host-b2"].ExitCode)
	}
	code := strings.TrimSpace(harness.MustText(t, fmt.Sprintf("cc exitcode %d host-b2", id)))
	if code != "3" {
		t.Errorf("cc exitcode = %q, want 3", code)
	}
}

func testCCSendRecv(t *testing.T) {
	payload := fmt.Sprintf("minimega live test %d\n", time.Now().UnixNano())
	src, _ := harness.Artifact(t, "send.txt")
	if err := os.WriteFile(src, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}

	// files are named relative to the namespace's files directory
	harness.CCRun(t, "name=host-c3", "send "+harness.Namespace+"/artifacts/send.txt", 1, time.Minute)
	out := harness.CCOK(t, "host-c3", "cat /tmp/miniccc/files/"+harness.Namespace+"/artifacts/send.txt")
	if out != payload {
		t.Errorf("guest read %q, want %q", out, payload)
	}

	harness.CCOK(t, "host-c3", "echo from-guest-$(hostname) > /tmp/recv.txt")
	_, id := harness.CCRun(t, "name=host-c3", "recv /tmp/recv.txt", 1, time.Minute)
	uuid := harness.VMInfo(t)["host-c3"]["uuid"]
	path := fmt.Sprintf("%v/%v/miniccc_responses/%d/%v/tmp/recv.txt", harness.MMFiles, harness.Namespace, id, uuid)
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.TrimSpace(string(b)) != "from-guest-host-c3" {
			return fmt.Errorf("received %q", b)
		}
		return nil
	})
}

func testCCBackgroundProcess(t *testing.T) {
	harness.CCRun(t, "name=host-a3", "background sleep 7777", 1, time.Minute)

	var pid string
	harness.Eventually(t, time.Minute, 3*time.Second, func() error {
		for _, r := range harness.MustTable(t, "cc process list host-a3") {
			if strings.Contains(r["command"], "sleep 7777") {
				pid = r["pid"]
				return nil
			}
		}
		return fmt.Errorf("sleep 7777 not listed")
	})
	harness.CCOK(t, "host-a3", "ps | grep -q '[s]leep 7777'")

	harness.CCRun(t, "name=host-a3", "process kill "+pid, 0, time.Minute)
	harness.Eventually(t, time.Minute, 3*time.Second, func() error {
		for _, r := range harness.MustTable(t, "cc process list host-a3") {
			if strings.Contains(r["command"], "sleep 7777") {
				return fmt.Errorf("process %v still listed", pid)
			}
		}
		return nil
	})
}

func testCCResponses(t *testing.T) {
	_, id := harness.CCRun(t, "name=host-b4", "exec echo cc-responses-check", 1, time.Minute)
	out := harness.MustText(t, fmt.Sprintf("cc responses %d", id))
	if !strings.Contains(out, "cc-responses-check") {
		t.Errorf("cc responses %d = %q", id, out)
	}
	harness.MustRun(t, fmt.Sprintf("cc delete response %d", id))
}

// --- guest view of VM configuration ----------------------------------------

func testGuestQemuAppend(t *testing.T) {
	out := harness.CCOK(t, "host-c1", "cat /sys/class/dmi/id/product_serial")
	if strings.TrimSpace(out) != "LIVE-C1" {
		t.Errorf("SMBIOS serial = %q, want LIVE-C1 (vm config qemu-append)", out)
	}
	// minimega passes the VM UUID through SMBIOS as well
	uuid := harness.CCOK(t, "host-d5", "cat /sys/class/dmi/id/product_uuid")
	if !strings.EqualFold(strings.TrimSpace(uuid), d5UUID) {
		t.Errorf("host-d5 SMBIOS uuid = %q, want %v", uuid, d5UUID)
	}
}

func testGuestDisks(t *testing.T) {
	// virtio disk shows up as vda, IDE as sda; both 64M (131072 sectors)
	if out := harness.CCOK(t, "host-c1", "cat /sys/block/vda/size"); strings.TrimSpace(out) != "131072" {
		t.Errorf("host-c1 vda size = %q", out)
	}
	if out := harness.CCOK(t, "host-c2", "cat /sys/block/sda/size"); strings.TrimSpace(out) != "131072" {
		t.Errorf("host-c2 sda size = %q", out)
	}
	// snapshot mode: the guest sees its own write, the shared base image on
	// the host does not change
	marker := fmt.Sprintf("live-snapshot-%d", time.Now().UnixNano())
	harness.CCOK(t, "host-c1", "echo "+marker+" | dd of=/dev/vda bs=512 count=1 2>/dev/null; sync")
	// drop the page cache so the read comes from the disk, not the write
	if out := harness.CCOK(t, "host-c1", "echo 3 > /proc/sys/vm/drop_caches; dd if=/dev/vda bs=512 count=1 2>/dev/null"); !strings.Contains(out, marker) {
		t.Errorf("host-c1 cannot read back its own write")
	}
	if out := harness.CCOK(t, "host-c2", "dd if=/dev/sda bs=512 count=1 2>/dev/null"); strings.Contains(out, marker) {
		t.Errorf("write on host-c1 is visible to host-c2")
	}
	base, err := os.ReadFile(harness.MMFiles + "/live/data.qcow2")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(base, []byte(marker)) {
		t.Errorf("snapshot=true VM wrote to the base image")
	}
}

func testGuestMachineType(t *testing.T) {
	if out := harness.CCOK(t, "host-c4", "cat /sys/class/dmi/id/product_name"); !strings.Contains(out, "Q35") {
		t.Errorf("host-c4 machine = %q, want Q35", out)
	}
	if out := harness.CCOK(t, "host-c3", "cat /sys/class/dmi/id/product_name"); !strings.Contains(out, "i440FX") {
		t.Errorf("host-c3 machine = %q, want i440FX default", out)
	}
	if out := harness.CCOK(t, "host-c5", "ls /sys/bus/pci/drivers/xhci_hcd/ | grep -c : || true"); strings.TrimSpace(out) == "0" {
		t.Errorf("host-c5 has no xHCI controller bound (usb-use-xhci)")
	}
}

func testGuestSMP(t *testing.T) {
	if out := harness.CCOK(t, "host-b3", "nproc"); strings.TrimSpace(out) != "2" {
		t.Errorf("host-b3 nproc = %q, want 2", out)
	}
	if out := harness.CCOK(t, "host-b4", "nproc"); strings.TrimSpace(out) != "1" {
		t.Errorf("host-b4 nproc = %q, want 1", out)
	}
	// Kernel overhead is the same on two single-vCPU guests, so their MemTotal
	// differs by the difference in "vm config memory", give or take some page
	// metadata (exactly 32768 kB on the arm64/TCG range). host-b3 is not used:
	// its second vCPU costs ~256 kB more.
	a, b := "host-a1", "host-b4"
	ma, _ := strconv.Atoi(specFor(a).Memory)
	mb, _ := strconv.Atoi(specFor(b).Memory)
	want := (ma - mb) * 1024
	if got := memTotalKB(t, a) - memTotalKB(t, b); got < want-2048 || got > want+1024 {
		t.Errorf("MemTotal %v - %v = %d kB, want about %d kB (memory %v vs %v MB)", a, b, got, want, ma, mb)
	}
}

// memTotalKB returns a guest's MemTotal from /proc/meminfo.
func memTotalKB(t *testing.T, vm string) int {
	t.Helper()
	out := harness.CCOK(t, vm, "grep MemTotal /proc/meminfo")
	f := strings.Fields(out)
	if len(f) < 2 {
		t.Fatalf("%v meminfo: %q", vm, out)
	}
	kb, err := strconv.Atoi(f[1])
	if err != nil {
		t.Fatalf("%v meminfo: %q: %v", vm, out, err)
	}
	return kb
}

func testGuestNICDrivers(t *testing.T) {
	driver := func(vm string) string {
		return strings.TrimSpace(harness.CCOK(t, vm, "basename $(readlink /sys/class/net/eth0/device/driver)"))
	}
	if d := driver("host-b1"); d != "e1000" {
		t.Errorf("host-b1 driver = %q, want e1000", d)
	}
	if d := driver("host-b3"); d != "virtio_net" {
		t.Errorf("host-b3 driver = %q, want virtio_net", d)
	}
	mac := strings.TrimSpace(harness.CCOK(t, "host-b2", "cat /sys/class/net/eth0/address"))
	if mac != "00:1f:53:00:02:12" {
		t.Errorf("host-b2 MAC = %q", mac)
	}
}

func testGuestDualHomed(t *testing.T) {
	out := harness.CCOK(t, "host-d5", "ip -4 -o addr")
	for _, ip := range []string{"10.10.4.15/24", "10.10.3.16/24"} {
		if !strings.Contains(out, ip) {
			t.Errorf("host-d5 missing %v:\n%s", ip, out)
		}
	}
	// the guest reaches both LANs directly
	harness.CCOK(t, "host-d5", "ping -c 2 -W 2 10.10.3.13 && ping -c 2 -W 2 10.10.4.12")
}

// --- networking --------------------------------------------------------------

func ipOf(t *testing.T, vm string) string {
	t.Helper()
	ips := harness.List(harness.VMInfo(t)[vm]["ip"])
	if len(ips) == 0 || ips[0] == "" {
		t.Fatalf("%v has no IP", vm)
	}
	return ips[0]
}

func testPingSameLAN(t *testing.T) {
	harness.CCOK(t, "host-a2", "ping -c 3 -W 2 "+ipOf(t, "host-a1"))
	harness.CCOK(t, "host-b1", "ping -c 3 -W 2 10.10.2.15")
	harness.CCOK(t, "host-c2", "ping -c 3 -W 2 10.10.3.1")
}

func testOSPF(t *testing.T) {
	// each router learns the other's LANs over lan-b
	harness.CCEventually(t, "rtr-ab", "ip route | grep -q '10.10.4.0/24 via 10.10.2.2'", harness.SettleTimeout)
	harness.CCEventually(t, "rtr-cd", "ip route | grep -q '10.10.1.0/24 via 10.10.2.1'", harness.SettleTimeout)
	out := harness.CCOK(t, "rtr-ab", "ip route")
	t.Logf("rtr-ab routes:\n%s", out)
}

func testPingAcrossRouters(t *testing.T) {
	// lan-a -> lan-d traverses rtr-ab, lan-b, and rtr-cd
	harness.CCEventually(t, "host-a1", "ping -c 3 -W 2 10.10.4.12", 2*time.Minute)
	harness.CCEventually(t, "host-c3", "ping -c 3 -W 2 "+ipOf(t, "host-a4"), 2*time.Minute)
	out := harness.CCOK(t, "host-a1", "traceroute -n -w 2 -q 1 10.10.4.12")
	for _, hop := range []string{"10.10.1.1", "10.10.2.2"} {
		if !strings.Contains(out, hop) {
			t.Errorf("traceroute missing hop %v:\n%s", hop, out)
		}
	}
}

func testNoBackchannelHost(t *testing.T) {
	harness.CCEventually(t, "host-d1", "ping -c 3 -W 2 10.10.4.14", time.Minute)
	harness.CCEventually(t, "host-b2", "ping -c 3 -W 2 10.10.4.14", time.Minute)
}

func testDNS(t *testing.T) {
	// busybox nslookup also asks for AAAA records, which minirouter's
	// dnsmasq refuses, so match the A record instead of the exit status
	harness.CCEventually(t, "host-a1", "nslookup web.live 2>&1 | grep -q 10.10.3.11", time.Minute)
	harness.CCEventually(t, "host-c2", "nslookup fw.live 2>&1 | grep -q 10.10.4.11", time.Minute)
	if out := harness.CCOK(t, "host-a1", "cat /etc/resolv.conf"); !strings.Contains(out, "10.10.1.1") {
		t.Errorf("DHCP did not hand out the router as DNS server:\n%s", out)
	}
}

func testHTTPProtonuke(t *testing.T) {
	// protonuke was started on host-c1 by topology.mm
	harness.CCEventually(t, "host-a2", "timeout 10 wget -q -O /tmp/index.html http://web.live/ && test -s /tmp/index.html", 2*time.Minute)
}

func testFirewall(t *testing.T) {
	// control: both ports answer from inside lan-d
	harness.CCEventually(t, "host-d2", "timeout 10 wget -q -O /dev/null http://10.10.4.11:8080/passwd", time.Minute)
	harness.CCEventually(t, "host-d2", "timeout 10 wget -q -O /dev/null http://10.10.4.11/", time.Minute)

	// through rtr-cd: 80 is open, 8080 is dropped by "router fw"
	harness.CCEventually(t, "host-a1", "timeout 10 wget -q -O /dev/null http://10.10.4.11/", time.Minute)
	r := harness.CCExec(t, "host-a1", "timeout 10 wget -q -O /dev/null http://10.10.4.11:8080/passwd")
	if r.ExitCode == 0 {
		t.Errorf("TCP/8080 to host-d1 should be blocked by rtr-cd")
	}
	out := harness.CCOK(t, "rtr-cd", "iptables -S")
	if !strings.Contains(out, "10.10.4.11") || !strings.Contains(out, "8080") {
		t.Errorf("rtr-cd iptables missing rule:\n%s", out)
	}
}

func testHostTap(t *testing.T) {
	found := false
	for _, r := range harness.MustTable(t, "tap") {
		if r["tap"] == "live-mgmt" {
			found = true
		}
	}
	if !found {
		t.Errorf("tap live-mgmt not listed")
	}
	// a guest reaches the minimega host's tap on lan-a ...
	harness.CCEventually(t, "host-a3", "ping -c 2 -W 2 10.10.1.250", time.Minute)

	// ... and the host reaches a service on lan-c through the tap and both
	// routers (the image has no ping, so use bash's /dev/tcp)
	if out, err := harness.InContainer(t, "ip", "route", "replace", "10.10.0.0/16", "via", "10.10.1.1", "dev", "live-mgmt"); err != nil {
		t.Fatalf("add route: %v: %s", err, out)
	}
	defer harness.InContainer(t, "ip", "route", "del", "10.10.0.0/16")
	harness.Eventually(t, time.Minute, 3*time.Second, func() error {
		out, err := harness.InContainer(t, "timeout", "10", "bash", "-c",
			`exec 3<>/dev/tcp/10.10.3.11/80 && printf 'GET / HTTP/1.0\r\n\r\n' >&3 && head -1 <&3`)
		if err != nil || !strings.HasPrefix(out, "HTTP/1.") {
			return fmt.Errorf("http via tap: %v: %q", err, out)
		}
		return nil
	})
}

var rttRe = regexp.MustCompile(`= [\d.]+/([\d.]+)/`)

func avgRTT(t *testing.T, from, to string) float64 {
	t.Helper()
	out := harness.CCOK(t, from, "ping -c 4 -W 3 "+to+" | tail -1")
	m := rttRe.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("cannot parse ping output: %q", out)
	}
	f, _ := strconv.ParseFloat(m[1], 64)
	return f
}

func testQoS(t *testing.T) {
	target := ipOf(t, "host-a2")
	harness.MustRun(t, "qos add host-a2 0 delay 250ms")
	defer harness.Run("clear qos host-a2 all")

	if q := harness.VMInfo(t)["host-a2"]["qos"]; !strings.Contains(q, "250ms") {
		t.Errorf("vm info qos = %q", q)
	}
	if rtt := avgRTT(t, "host-a4", target); rtt < 240 {
		t.Errorf("rtt with 250ms delay = %.1fms", rtt)
	}

	harness.MustRun(t, "clear qos host-a2 0")
	if rtt := avgRTT(t, "host-a4", target); rtt > 150 {
		t.Errorf("rtt after clear qos = %.1fms", rtt)
	}

	// packet loss (minimega accepts 0 <= loss < 100)
	harness.MustRun(t, "qos add host-a2 0 loss 99")
	out := harness.CCExec(t, "host-a4", "ping -c 10 -W 1 "+target+" | tail -2").Stdout
	m := regexp.MustCompile(`(\d+) packets received`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("cannot parse ping output: %q", out)
	}
	if n, _ := strconv.Atoi(m[1]); n > 2 {
		t.Errorf("%d of 10 pings answered with 99%% loss", n)
	}
	harness.MustRun(t, "clear qos host-a2 all")
	if q := harness.VMInfo(t)["host-a2"]["qos"]; strings.Contains(q, "loss") {
		t.Errorf("qos still set after clear: %q", q)
	}
}

func testCapturePcap(t *testing.T) {
	host, path := harness.Artifact(t, "host-a5.pcap")
	harness.MustRun(t, "capture pcap vm host-a5 0 "+path)
	harness.CCOK(t, "host-a5", "ping -c 5 -W 2 10.10.1.1")
	if rows := harness.MustTable(t, "capture"); len(rows) == 0 {
		t.Errorf("capture list empty while capturing")
	}
	harness.MustRun(t, "capture pcap delete vm host-a5")

	// the capture must hold the pings, not just background ARP and OSPF
	if n, err := harness.CountICMPEcho(host, "10.10.1.1"); err != nil || n < 5 {
		t.Errorf("pcap has %d ICMP echo requests to 10.10.1.1, want >= 5: %v", n, err)
	}
	harness.Keep(t, host)
}

// --- VM operations -----------------------------------------------------------

func testScreenshot(t *testing.T) {
	host, path := harness.Artifact(t, "host-a1.png")
	harness.MustRun(t, "vm screenshot host-a1 file "+path+" 800")
	b, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	// the console shows text, so a blank (one-color) frame is a failure
	if n, err := harness.DistinctColors(b); err != nil || n < 2 {
		t.Errorf("screenshot has %d colors, want a console with text: %v", n, err)
	}
	harness.Keep(t, host)

	// cirrus VGA
	host, path = harness.Artifact(t, "host-c5.png")
	harness.MustRun(t, "vm screenshot host-c5 file "+path)
	if b, err := os.ReadFile(host); err != nil {
		t.Errorf("host-c5 screenshot failed: %v", err)
	} else if _, err := harness.DistinctColors(b); err != nil {
		t.Errorf("host-c5 screenshot is not a PNG: %v", err)
	}
}

func testSerial(t *testing.T) {
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		out, err := harness.Text("vm serial host-a1 0 3000")
		if err != nil {
			return err
		}
		if !strings.Contains(out, "LIVE-SERIAL host-a1") {
			return fmt.Errorf("serial output: %q", out)
		}
		return nil
	})
}

func testQMP(t *testing.T) {
	out := harness.MustText(t, `vm qmp host-a1 '{ "execute": "query-status" }'`)
	if !strings.Contains(out, "running") {
		t.Errorf("query-status: %s", out)
	}
	out = harness.MustText(t, `vm qmp host-c3 '{ "execute": "query-name" }'`)
	if !strings.Contains(out, "return") {
		t.Errorf("query-name: %s", out)
	}
}

func testTags(t *testing.T) {
	harness.MustRun(t, "vm tag host-b5 stage tested")
	rows := harness.MustTable(t, "vm tag host-b5 stage")
	if len(rows) != 1 || rows[0]["value"] != "tested" {
		t.Errorf("vm tag get = %v", rows)
	}
	res, _ := harness.CCRun(t, "stage=tested", "exec hostname", 1, time.Minute)
	if len(res) != 1 || strings.TrimSpace(res["host-b5"].Stdout) != "host-b5" {
		t.Errorf("tag filter reached %v", harness.Keys(res))
	}
}

func testTop(t *testing.T) {
	rows := harness.MustTable(t, "vm top 2")
	if len(rows) < len(topology) {
		t.Errorf("vm top rows = %d", len(rows))
	}
}

func testStopStart(t *testing.T) {
	harness.MustRun(t, "vm stop host-a4")
	if s := harness.VMInfo(t)["host-a4"]["state"]; s != "PAUSED" {
		t.Errorf("state after stop = %v", s)
	}
	harness.MustRun(t, "vm start host-a4")
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		if s := harness.VMInfo(t)["host-a4"]["state"]; s != "RUNNING" {
			return fmt.Errorf("state = %v", s)
		}
		return nil
	})
	harness.CCEventually(t, "host-a4", "true", time.Minute)
}

func testNetDisconnectConnect(t *testing.T) {
	harness.CCOK(t, "host-b4", "ping -c 2 -W 2 10.10.2.15")

	harness.MustRun(t, "vm net disconnect host-b5 0")
	if v := harness.VMInfo(t)["host-b5"]["vlan"]; !strings.Contains(v, "disconnected") {
		t.Errorf("vlan after disconnect = %v", v)
	}
	if r := harness.CCExec(t, "host-b4", "ping -c 2 -W 2 10.10.2.15"); r.ExitCode == 0 {
		t.Errorf("host-b5 still reachable after disconnect")
	}

	harness.MustRun(t, "vm net connect host-b5 0 lan-b")
	harness.CCEventually(t, "host-b4", "ping -c 2 -W 2 10.10.2.15", time.Minute)
}

func testHotplug(t *testing.T) {
	// unlike "disk create", "vm hotplug add" does not resolve paths against
	// the files directory, so pass an absolute path
	harness.MustRun(t, "disk create qcow2 live/hotplug.qcow2 32M")
	harness.MustRun(t, "vm hotplug add host-c3 "+harness.MMFiles+"/live/hotplug.qcow2")

	rows := harness.MustTable(t, "vm hotplug")
	if len(rows) == 0 || !strings.Contains(fmt.Sprint(rows), "hotplug.qcow2") {
		t.Errorf("vm hotplug list: %v", rows)
	}
	// USB mass storage appears as a SCSI disk of 32M (65536 sectors)
	harness.CCEventually(t, "host-c3", "grep -l 65536 /sys/block/sd*/size", time.Minute)

	harness.MustRun(t, "vm hotplug remove host-c3 all")
	harness.CCEventually(t, "host-c3", "! grep -l 65536 /sys/block/sd*/size 2>/dev/null", time.Minute)
}

func testSave(t *testing.T) {
	harness.MustRun(t, "vm save host-c2 live/saved-c2")
	harness.Eventually(t, 3*time.Minute, 3*time.Second, func() error {
		for _, r := range harness.MustTable(t, "vm save") {
			if r["name"] == "host-c2" && r["status"] != "completed" {
				return fmt.Errorf("save status: %v", r)
			}
		}
		return nil
	})
	for _, ext := range []string{".state", ".hdd"} {
		harness.Eventually(t, time.Minute, 2*time.Second, func() error {
			fi, err := os.Stat(fmt.Sprintf("%v/live/saved-c2%v", harness.MMFiles, ext))
			if err != nil {
				return err
			}
			if fi.Size() == 0 {
				return fmt.Errorf("%v is empty", ext)
			}
			return nil
		})
	}
	// the VM keeps running (or can be resumed) after the save
	if s := harness.VMInfo(t)["host-c2"]["state"]; s != "RUNNING" {
		harness.MustRun(t, "vm start host-c2")
	}
	harness.CCEventually(t, "host-c2", "true", time.Minute)
}

func testKillFlushRelaunch(t *testing.T) {
	harness.MustRun(t, "vm kill host-a5")
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		if s := harness.VMInfo(t)["host-a5"]["state"]; s != "QUIT" {
			return fmt.Errorf("state = %v", s)
		}
		return nil
	})
	harness.MustRun(t, "vm flush")
	if _, ok := harness.VMInfo(t)["host-a5"]; ok {
		t.Fatalf("host-a5 still present after flush")
	}

	// relaunch through the queue from a clone of a running VM's config
	// (clone needs a launched VM, so it can't be used for queued ones)
	harness.MustRun(t, "vm config clone host-a4")
	harness.MustRun(t, "vm config append live.role=host live.hostname=host-a6")
	harness.MustRun(t, "vm launch kvm host-a6")
	if q := harness.MustText(t, "ns queue"); !strings.Contains(q, "host-a6") {
		t.Errorf("host-a6 not queued: %q", q)
	}
	harness.MustRun(t, "vm launch")
	harness.MustRun(t, "vm start host-a6")
	harness.MustRun(t, "clear vm config")

	harness.Eventually(t, 3*time.Minute, 5*time.Second, func() error {
		r := harness.VMInfo(t)["host-a6"]
		if r["cc_active"] != "true" {
			return fmt.Errorf("host-a6 state=%v cc=%v", r["state"], r["cc_active"])
		}
		if ips := harness.List(r["ip"]); len(ips) == 0 || !strings.HasPrefix(ips[0], "10.10.1.") {
			return fmt.Errorf("host-a6 ip=%v", r["ip"])
		}
		return nil
	})
	harness.CCOK(t, "host-a6", "ping -c 2 -W 2 10.10.1.1")

	r := harness.VMInfo(t)["host-a6"]
	if r["memory"] != specFor("host-a4").Memory || !strings.Contains(r["tags"], `"lan":"a"`) || !strings.HasPrefix(harness.List(r["vlan"])[0], "lan-a") {
		t.Errorf("host-a6 did not inherit host-a4's config: memory=%v tags=%v vlan=%v", r["memory"], r["tags"], r["vlan"])
	}
	if r["uuid"] == harness.VMInfo(t)["host-a4"]["uuid"] {
		t.Errorf("clone kept the source UUID")
	}
}

// --- tools -------------------------------------------------------------------

// testPasswordify boots a VM from a passwordified copy of the guest initrd and
// checks that root's new password authenticates in the guest (issue #1213).
func testPasswordify(t *testing.T) {
	const password = "live-passw0rd"
	const authKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIEtc2QDYSXnUzZkRBTh7gjLWuGmQYDPfAxV+CbH7LJ6v live-test"

	// passwordify chroots into the guest image, so it needs the guest's architecture
	hostArch, err := harness.InContainer(t, "uname", "-m")
	if err != nil {
		t.Fatalf("uname: %v: %s", err, hostArch)
	}
	if guestArch := harness.CCOK(t, "host-a4", "uname -m"); strings.TrimSpace(guestArch) != strings.TrimSpace(hostArch) {
		t.Skipf("container is %v but the guest is %v; passwordify cannot run guest binaries", strings.TrimSpace(hostArch), strings.TrimSpace(guestArch))
	}

	dir := filepath.Join(harness.MMFiles, "live")
	keys := filepath.Join(dir, "passwordify.keys")
	if err := os.WriteFile(keys, []byte(authKey+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// The guest has no sshd, so add the legacy config that broke ssh logins (#1213),
	// quoted as sshd allows, then run passwordify with relative paths, the
	// password on stdin, and both key options.
	script := fmt.Sprintf(`set -e
cd %v
rm -rf pwroot && mkdir pwroot && cd pwroot
zcat ../live.initrd | cpio -id --quiet
mkdir -p etc/ssh && printf 'PasswordAuthentication yes\nPermitEmptyPasswords "yes"\n' > etc/ssh/sshd_config
find . | cpio -o -H newc --quiet | gzip > ../live-ssh.initrd
cd .. && rm -rf pwroot
echo %v | passwordify -keys passwordify.keys live-ssh.initrd live-pw.initrd`, dir, password)
	if out, err := harness.InContainer(t, "sh", "-c", script); err != nil {
		t.Fatalf("passwordify: %v\n%s", err, out)
	}

	harness.MustRun(t, "vm config clone host-a4")
	harness.MustRun(t, "vm config initrd live/live-pw.initrd")
	harness.MustRun(t, "vm config append live.role=host live.hostname=host-pw")
	harness.MustRun(t, "vm launch kvm host-pw")
	harness.MustRun(t, "vm launch")
	harness.MustRun(t, "clear vm config")
	t.Cleanup(func() {
		harness.Run("vm kill host-pw")
		harness.Run("vm flush")
	})
	harness.MustRun(t, "vm start host-pw")
	harness.CCEventually(t, "host-pw", "true", 3*time.Minute)

	// su reads the password from stdin when it is not a terminal; as root it would not ask
	su := func(vm, pw string) harness.CCResult {
		return harness.CCExec(t, vm, fmt.Sprintf("echo %v | su nobody -s /bin/sh -c 'su -c true root'", pw))
	}
	if r := su("host-pw", password); r.ExitCode != 0 {
		t.Errorf("root password rejected on host-pw: exit %d: %s%s", r.ExitCode, r.Stdout, r.Stderr)
	}
	if r := su("host-pw", "wrong-"+password); r.ExitCode == 0 {
		t.Errorf("wrong root password accepted on host-pw")
	}
	// the stock image has no root password, so the change came from passwordify
	if r := su("host-a4", password); r.ExitCode == 0 {
		t.Errorf("root password accepted on host-a4, which runs the stock initrd")
	}

	ak := harness.CCOK(t, "host-pw", "cat /root/.ssh/authorized_keys")
	pub := harness.CCOK(t, "host-pw", "cat /root/.ssh/id_rsa.pub")
	if !strings.Contains(ak, authKey) || !strings.Contains(ak, strings.TrimSpace(pub)) || !strings.HasPrefix(pub, "ssh-rsa ") {
		t.Errorf("authorized_keys lacks the -keys key or the generated id_rsa.pub:\n%s\nid_rsa.pub: %s", ak, pub)
	}
	if cfg := harness.CCOK(t, "host-pw", "cat /etc/ssh/sshd_config"); !strings.Contains(cfg, "PermitEmptyPasswords no") || strings.Contains(cfg, "yes\"") {
		t.Errorf("sshd_config still allows empty passwords:\n%s", cfg)
	}
	if mode := strings.TrimSpace(harness.CCOK(t, "host-pw", "stat -c %a /")); mode != "755" {
		t.Errorf("/ has mode %v, want 755", mode)
	}
}

// --- optimizations -----------------------------------------------------------

const (
	hugeVM        = "huge-a"
	hugeOddVM     = "huge-odd"
	hugeMemory    = 384 // MB, a multiple of 2 MiB that boots the live guest
	hugeOddMemory = 257 // MB, not a multiple of 2 MiB
	hugePageSize  = 2 << 20
	hugeMount     = "/mnt/live-hugepages"
)

// testHugepages launches a VM backed by hugepages through "optimize
// hugepages", then checks that minimega rejects a VM whose memory is not a
// multiple of the hugepage size. LIVE_HUGEPAGES is the number of 2 MiB
// hugepages that the host reserved for the phase.
func testHugepages(t *testing.T) {
	env := os.Getenv("LIVE_HUGEPAGES")
	reserved, err := strconv.Atoi(env)
	if env != "" && err != nil {
		t.Fatalf("LIVE_HUGEPAGES=%q is not a number", env)
	}
	if reserved <= 0 {
		t.Skip("LIVE_HUGEPAGES is not set: reserve 2 MiB hugepages on the host (vm.nr_hugepages) and set LIVE_HUGEPAGES to their number")
	}
	need := hugeMemory * (1 << 20) / hugePageSize
	if reserved < need {
		t.Fatalf("LIVE_HUGEPAGES=%d, but a %d MB VM needs %d hugepages of 2 MiB", reserved, hugeMemory, need)
	}

	path := hugetlbfsMount(t)
	t.Logf("hugetlbfs mount in the container: %v", path)

	before := hugePages(t)
	if avail := before["HugePages_Free"] - before["HugePages_Rsvd"]; avail < need {
		t.Fatalf("host has %d available hugepages, want >= %d: %v", avail, need, before)
	}

	done := false
	t.Cleanup(func() {
		if !done {
			harness.Run("ns flush")
			harness.Run("vm kill " + hugeVM)
			harness.Run("vm flush " + hugeVM + "," + hugeOddVM)
			harness.Run("clear optimize hugepages")
			harness.Run("clear vm config")
		}
	})

	harness.MustRun(t, "optimize hugepages "+path)
	if got := strings.TrimSpace(harness.MustText(t, "optimize hugepages")); got != path {
		t.Errorf("optimize hugepages = %q, want %q", got, path)
	}

	// live-base holds the kernel, the initrd, and the run.sh overrides. The VM
	// has no NIC and no cc backchannel, so it does not change the cc clients.
	harness.MustRun(t, "vm config restore live-base")
	harness.MustRun(t, "vm config backchannel false")
	harness.MustRun(t, "vm config tags role hugepages")
	harness.MustRun(t, fmt.Sprintf("vm config memory %d", hugeMemory))
	harness.MustRun(t, "vm config append live.role=host live.dhcp=0 live.hostname="+hugeVM)
	harness.MustRun(t, "vm launch kvm "+hugeVM)
	harness.MustRun(t, "vm launch")
	harness.MustRun(t, "vm start "+hugeVM)

	var pid string
	harness.Eventually(t, time.Minute, 2*time.Second, func() error {
		r, ok := harness.VMInfo(t)[hugeVM]
		if !ok {
			return fmt.Errorf("%v not listed", hugeVM)
		}
		if r["state"] != "RUNNING" {
			return fmt.Errorf("%v: state %v (tags: %v)", hugeVM, r["state"], r["tags"])
		}
		pid = r["pid"]
		return nil
	})

	out, err := harness.InContainer(t, "cat", "/proc/"+pid+"/cmdline")
	if err != nil {
		t.Fatalf("QEMU command line of %v (pid %v): %v: %s", hugeVM, pid, err, out)
	}
	if args := strings.Split(strings.TrimRight(out, "\x00"), "\x00"); !hasMemPath(args, path) {
		t.Errorf("QEMU command line of %v has no -mem-path %v: %q", hugeVM, path, args)
	}

	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		now := hugePages(t)
		if now["HugePages_Free"] < before["HugePages_Free"] || now["HugePages_Rsvd"] > before["HugePages_Rsvd"] {
			return nil
		}
		return fmt.Errorf("%v uses no hugepages: before %v, now %v", hugeVM, before, now)
	})

	// The scheduler logs launch errors, so the error is in the tags of the VM.
	harness.MustRun(t, fmt.Sprintf("vm config memory %d", hugeOddMemory))
	harness.MustRun(t, "vm config append live.role=host live.dhcp=0 live.hostname="+hugeOddVM)
	harness.MustRun(t, "vm launch kvm "+hugeOddVM)
	want := fmt.Sprintf("vm memory %d MB is not a multiple of the 2 MiB hugepage size of %v", hugeOddMemory, path)
	if _, err := harness.Run("vm launch"); err != nil && !strings.Contains(err.Error(), want) {
		t.Errorf("vm launch %v: %v, want an error that contains %q", hugeOddVM, err, want)
	}
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		r, ok := harness.VMInfo(t)[hugeOddVM]
		if !ok {
			return fmt.Errorf("%v not listed", hugeOddVM)
		}
		if r["state"] != "ERROR" || !strings.Contains(r["tags"], want) {
			return fmt.Errorf("%v: state %v, tags %v; want ERROR with %q", hugeOddVM, r["state"], r["tags"], want)
		}
		return nil
	})

	harness.MustRun(t, "vm kill "+hugeVM)
	harness.Eventually(t, 30*time.Second, 2*time.Second, func() error {
		if s := harness.VMInfo(t)[hugeVM]["state"]; s != "QUIT" {
			return fmt.Errorf("%v: state %v", hugeVM, s)
		}
		return nil
	})
	harness.MustRun(t, "vm flush "+hugeVM+","+hugeOddVM)
	harness.MustRun(t, "clear optimize hugepages")
	harness.MustRun(t, "clear vm config")
	done = true

	vms := harness.VMInfo(t)
	for _, name := range []string{hugeVM, hugeOddVM} {
		if _, ok := vms[name]; ok {
			t.Errorf("%v still present after flush", name)
		}
	}
	if got := strings.TrimSpace(harness.MustText(t, "optimize hugepages")); got != "" {
		t.Errorf("optimize hugepages = %q after clear", got)
	}
}

// hugetlbfsMount returns a hugetlbfs mount with 2 MiB pages in the container.
// It uses the host /dev/hugepages, which the /dev bind mount of run.sh can
// show, or it mounts one that it removes at the end of the test.
func hugetlbfsMount(t *testing.T) string {
	t.Helper()
	if hugetlbfsPageSize(t, "/dev/hugepages") == hugePageSize {
		return "/dev/hugepages"
	}

	out, err := harness.InContainer(t, "sh", "-c", "mkdir -p "+hugeMount+" && mount -t hugetlbfs -o pagesize=2M none "+hugeMount)
	if err != nil {
		t.Fatalf("no hugetlbfs mount: /dev/hugepages in the container is not hugetlbfs with 2 MiB pages, and the mount failed: %v: %s", err, out)
	}
	t.Cleanup(func() {
		out, err := harness.InContainer(t, "sh", "-c", "umount "+hugeMount+" && rmdir "+hugeMount)
		switch {
		case err == nil:
		case t.Failed():
			// a VM of the failed test can still use the mount
			t.Logf("remove %v: %v: %s", hugeMount, err, out)
		default:
			t.Errorf("remove %v: %v: %s", hugeMount, err, out)
		}
	})

	if size := hugetlbfsPageSize(t, hugeMount); size != hugePageSize {
		t.Fatalf("%v in the container has page size %d, want %d", hugeMount, size, hugePageSize)
	}
	return hugeMount
}

// hugetlbfsPageSize returns the hugepage size of the hugetlbfs at path in the
// container, or 0 when path is not on hugetlbfs.
func hugetlbfsPageSize(t *testing.T, path string) int {
	t.Helper()
	out, err := harness.InContainer(t, "stat", "-f", "-c", "%T %s", path)
	f := strings.Fields(out)
	if err != nil || len(f) != 2 || f[0] != "hugetlbfs" {
		return 0
	}
	size, _ := strconv.Atoi(f[1])
	return size
}

// hugePages returns the HugePages_* counters from /proc/meminfo of the host.
// The kernel is shared with the container.
func hugePages(t *testing.T) map[string]int {
	t.Helper()
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		t.Fatal(err)
	}
	res := map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || !strings.HasPrefix(f[0], "HugePages_") {
			continue
		}
		res[strings.TrimSuffix(f[0], ":")], _ = strconv.Atoi(f[1])
	}
	return res
}

// hasMemPath reports whether args contain "-mem-path path".
func hasMemPath(args []string, path string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-mem-path" && args[i+1] == path {
			return true
		}
	}
	return false
}

// --- teardown ----------------------------------------------------------------

func testClearNamespace(t *testing.T) {
	if _, err := harness.Raw("clear namespace " + harness.Namespace); err != nil {
		t.Fatalf("clear namespace: %v", err)
	}

	for _, r := range toRowsMust(t, "namespace") {
		if r["namespace"] == harness.Namespace {
			t.Errorf("namespace %v still exists", harness.Namespace)
		}
	}

	harness.Eventually(t, time.Minute, 3*time.Second, func() error {
		out, _ := harness.InContainer(t, "pgrep", "-c", "qemu")
		if n := strings.TrimSpace(out); n != "0" && n != "" {
			return fmt.Errorf("%v qemu processes still running", n)
		}
		out, err := harness.InContainer(t, "ovs-vsctl", "list-ports", "mega_bridge")
		if err == nil && strings.TrimSpace(out) != "" {
			return fmt.Errorf("ports left on mega_bridge: %v", strings.Fields(out))
		}
		return nil
	})
}

func toRowsMust(t *testing.T, cmd string) []harness.Row {
	t.Helper()
	resps, err := harness.Raw(cmd)
	if err != nil {
		t.Fatal(err)
	}
	return harness.ToRows(resps)
}
