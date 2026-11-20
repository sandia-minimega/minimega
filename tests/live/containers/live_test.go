// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

//go:build live

package containers

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/tests/live/harness"
)

// where minimega mounts cgroups (-cgroup) in the Docker deployment
const cgroupRoot = "/sys/fs/cgroup"

// minimega -base directory, as in the harness
var mmBase = func() string {
	if v := os.Getenv("LIVE_MM_BASE"); v != "" {
		return v
	}
	return "/tmp/minimega"
}()

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
	version := []string{"Version"}
	inv := []string{"Inventory"}
	cc := []string{"CCClients"}
	network := []string{"CCClients", "DHCP"}

	p := func(name string, fn func(*testing.T), needs []string) harness.Phase {
		return harness.Phase{Name: name, Fn: fn, Needs: needs}
	}

	harness.RunPhases(t, []harness.Phase{
		// control plane
		p("Version", testVersion, nil),
		p("Namespace", testNamespace, version),
		p("Inventory", testInventory, []string{"Namespace"}),
		p("VMConfig", testVMConfig, inv),
		p("Cgroups", testCgroups, inv),
		p("Bridge", testBridge, inv),

		// guests come up
		p("CCClients", testCCClients, inv),
		p("DHCP", testDHCP, inv),

		// what a container sees and is allowed to do
		p("CCExec", testCCExec, cc),
		p("Isolation", testIsolation, cc),
		p("Devices", testDevices, cc),
		p("Volume", testVolume, cc),
		p("Fifo", testFifo, cc),

		// networking
		p("PingSameLAN", testPingSameLAN, network),
		p("PingAcrossRouter", testPingAcrossRouter, network),
		p("HTTPProtonuke", testHTTPProtonuke, network),

		// VM operations
		p("Top", testTop, inv),
		p("StopStart", testStopStart, cc),
		p("KillRestart", testKillRestart, network),
		p("KillFlushRelaunch", testKillFlushRelaunch, network),

		// teardown
		p("ClearNamespace", testClearNamespace, version),
	})
}

// --- minimega host -----------------------------------------------------------

// cgroupUnified returns true if the minimega host has cgroup v2.
func cgroupUnified(t *testing.T) bool {
	t.Helper()
	out, err := harness.InContainer(t, "stat", "-f", "-c", "%T", cgroupRoot)
	if err != nil {
		t.Fatalf("stat %v: %v: %s", cgroupRoot, err, out)
	}
	return strings.TrimSpace(out) == "cgroup2fs"
}

// cgroupDirs returns the cgroups of a VM on the minimega host: one for each
// cgroup v1 controller that minimega uses, or the single one of cgroup v2.
func cgroupDirs(unified bool, id string) map[string]string {
	dirs := map[string]string{}
	for _, c := range []string{"freezer", "memory", "devices", "cpu"} {
		dirs[c] = path.Join(cgroupRoot, c, "minimega", id)
		if unified {
			dirs[c] = path.Join(cgroupRoot, "minimega", id)
		}
	}
	return dirs
}

// readCgroup reads a file of a VM's cgroup. controller selects the hierarchy
// on cgroup v1.
func readCgroup(t *testing.T, unified bool, id, controller, file string) string {
	t.Helper()
	f := path.Join(cgroupDirs(unified, id)[controller], file)
	out, err := harness.InContainer(t, "cat", f)
	if err != nil {
		t.Fatalf("read %v: %v: %s", f, err, out)
	}
	return strings.TrimSpace(out)
}

// frozen returns nil if the cgroup freezer of a VM is in the wanted state.
func frozen(t *testing.T, unified bool, id string, want bool) error {
	t.Helper()
	file, state := "freezer.state", map[bool]string{true: "FROZEN", false: "THAWED"}
	if unified {
		file, state = "cgroup.events", map[bool]string{true: "frozen 1", false: "frozen 0"}
	}
	if got := readCgroup(t, unified, id, "freezer", file); !strings.Contains(got, state[want]) {
		return fmt.Errorf("%v is %q, want %q", file, got, state[want])
	}
	return nil
}

// --- control plane -----------------------------------------------------------

func testVersion(t *testing.T) {
	v := harness.MustText(t, "version")
	if !strings.Contains(v, "minimega") {
		t.Fatalf("unexpected version: %q", v)
	}
	t.Log(strings.TrimSpace(v))
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
			want := "kvm"
			if s.Container {
				want = "container"
			}
			if r["type"] != want {
				return fmt.Errorf("%v: type %v, want %v", s.Name, r["type"], want)
			}
			if r["state"] != "RUNNING" {
				return fmt.Errorf("%v: state %v (tags: %v)", s.Name, r["state"], r["tags"])
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
			if r["memory"] != strconv.Itoa(s.Memory) {
				t.Errorf("memory = %v, want %v", r["memory"], s.Memory)
			}
			if r["vcpus"] != strconv.Itoa(s.VCPUs) {
				t.Errorf("vcpus = %v, want %v", r["vcpus"], s.VCPUs)
			}
			if r["snapshot"] != "true" {
				t.Errorf("snapshot = %v", r["snapshot"])
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

			if !s.Container {
				return
			}
			if !strings.HasSuffix(r["filesystem"], "live/rootfs") {
				t.Errorf("filesystem = %v", r["filesystem"])
			}
			if want := "[/usr/local/sbin/live-container " + s.Init + "]"; r["init"] != want {
				t.Errorf("init = %v, want %v", r["init"], want)
			}
			if r["hostname"] != s.Hostname {
				t.Errorf("hostname = %v, want %v", r["hostname"], s.Hostname)
			}
			if pid, _ := strconv.Atoi(r["pid"]); pid <= 0 {
				t.Errorf("pid = %q", r["pid"])
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
	check("ctr-rtr", "preinit", "live/rootfs/usr/local/sbin/live-preinit")
	check("ctr-a1", "uuid", a1UUID)
	check("ctr-a1", "volume", "live/scratch-a1")
	check("ctr-b1", "mac", "00:1f:53:00:12:11")
	check("kvm-b1", "kernel", "live/live.kernel")
}

// testCgroups checks that minimega gave each container a cgroup with the
// limits of its configuration.
func testCgroups(t *testing.T) {
	unified := cgroupUnified(t)
	if unified {
		t.Log("the minimega host has cgroup v2")
	} else {
		t.Log("the minimega host has cgroup v1")
	}

	vms := harness.VMInfo(t)

	for _, s := range containerVMs() {
		t.Run(s.Name, func(t *testing.T) {
			r := vms[s.Name]
			read := func(controller, file string) string {
				t.Helper()
				return readCgroup(t, unified, r["id"], controller, file)
			}

			// memory in bytes, and CPU time in microseconds for each second
			memory := strconv.Itoa(s.Memory << 20)
			quota := strconv.Itoa(s.VCPUs * 1000000)

			if unified {
				if got := read("memory", "memory.max"); got != memory {
					t.Errorf("memory.max = %v, want %v", got, memory)
				}
				if got, want := read("cpu", "cpu.max"), quota+" 1000000"; got != want {
					t.Errorf("cpu.max = %q, want %q", got, want)
				}
				if got := read("freezer", "cgroup.events"); !strings.Contains(got, "populated 1") {
					t.Errorf("cgroup.events = %q", got)
				}
			} else {
				if got := read("memory", "memory.limit_in_bytes"); got != memory {
					t.Errorf("memory.limit_in_bytes = %v, want %v", got, memory)
				}
				if got := read("cpu", "cpu.cfs_quota_us"); got != quota {
					t.Errorf("cpu.cfs_quota_us = %v, want %v", got, quota)
				}
				if got := read("cpu", "cpu.cfs_period_us"); got != "1000000" {
					t.Errorf("cpu.cfs_period_us = %v", got)
				}
				if got := read("devices", "devices.list"); !strings.Contains(got, "c 1:3 rwm") || strings.Contains(got, "a *:* rwm") {
					t.Errorf("devices.list = %q", got)
				}
			}
			if err := frozen(t, unified, r["id"], false); err != nil {
				t.Error(err)
			}

			// the init process of the container is in every cgroup
			for controller := range cgroupDirs(unified, r["id"]) {
				if procs := strings.Fields(read(controller, "cgroup.procs")); !slices.Contains(procs, r["pid"]) {
					t.Errorf("%v cgroup.procs = %v, want it to contain pid %v", controller, procs, r["pid"])
				}
			}
		})
	}

	// KVM VMs have no cgroup
	for _, s := range topology {
		if s.Container {
			continue
		}
		for _, dir := range cgroupDirs(unified, vms[s.Name]["id"]) {
			if out, err := harness.InContainer(t, "ls", dir); err == nil {
				t.Errorf("%v has a cgroup %v: %s", s.Name, dir, out)
			}
		}
	}
}

func testBridge(t *testing.T) {
	out, err := harness.InContainer(t, "ovs-vsctl", "list-ports", "mega_bridge")
	if err != nil {
		t.Fatalf("ovs-vsctl: %v: %s", err, out)
	}
	if ports := strings.Fields(out); len(ports) != totalNICs {
		t.Errorf("mega_bridge has %d ports, want %d: %v", len(ports), totalNICs, ports)
	}

	// each container has a network namespace that minimega names for iproute2
	out, err = harness.InContainer(t, "ip", "netns", "list")
	if err != nil {
		t.Fatalf("ip netns: %v: %s", err, out)
	}
	vms := harness.VMInfo(t)
	for _, s := range containerVMs() {
		if ns := "meganet_" + vms[s.Name]["id"]; !strings.Contains(out, ns) {
			t.Errorf("%v: network namespace %v not listed: %s", s.Name, ns, out)
		}
	}
}

// --- guests come up ----------------------------------------------------------

func testCCClients(t *testing.T) {
	// containers reach minimega over a unix socket in their filesystem, the
	// KVM VM over virtio-serial
	harness.Eventually(t, harness.SettleTimeout, 5*time.Second, func() error {
		vms := harness.VMInfo(t)
		var missing []string
		for _, s := range topology {
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
	if len(clients) != len(topology) {
		t.Errorf("cc clients = %d, want %d", len(clients), len(topology))
	}
}

func testDHCP(t *testing.T) {
	// minimega learns guest addresses by snooping traffic on each tap
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
}

// --- what a container sees and is allowed to do ------------------------------

func testCCExec(t *testing.T) {
	// broadcast to every client
	res, _ := harness.CCRun(t, "", "exec hostname", len(topology), 2*time.Minute)
	for _, s := range topology {
		r, ok := res[s.Name]
		if !ok {
			t.Errorf("no response from %v", s.Name)
			continue
		}
		if got := strings.TrimSpace(r.Stdout); got != s.Hostname {
			t.Errorf("%v reports hostname %q, want %q", s.Name, got, s.Hostname)
		}
	}

	if r := harness.CCExec(t, "ctr-a2", "exit 7"); r.ExitCode != 7 {
		t.Errorf("exit code = %d, want 7", r.ExitCode)
	}
}

func testIsolation(t *testing.T) {
	// a PID namespace of its own: the init program is PID 1
	if r := harness.CCExec(t, "ctr-a2", "grep -q live-container /proc/1/cmdline"); r.ExitCode != 0 {
		t.Errorf("PID 1 is not the init program of the container")
	}
	// and the only processes are its own (init, miniccc, udhcpc, this command)
	out := harness.CCOK(t, "ctr-a2", "ls /proc | grep -c '^[0-9]'")
	if n, err := strconv.Atoi(strings.TrimSpace(out)); err != nil || n < 2 || n > 20 {
		t.Errorf("container sees %q processes, want a handful", strings.TrimSpace(out))
	}

	// /proc/sys is read-only, so only the preinit program of the router could
	// turn on forwarding
	if r := harness.CCExec(t, "ctr-a2", "echo 1 > /proc/sys/net/ipv4/ip_forward"); r.ExitCode == 0 {
		t.Errorf("container can write /proc/sys")
	}
	if out := harness.CCOK(t, "ctr-rtr", "cat /proc/sys/net/ipv4/ip_forward"); strings.TrimSpace(out) != "1" {
		t.Errorf("ctr-rtr ip_forward = %q, want 1 (vm config preinit)", out)
	}
	// A new network namespace inherits IPv4 forwarding from the host (Docker
	// turns it on) but not IPv6 forwarding, so that one shows that preinit ran.
	const forwarding6 = "cat /proc/sys/net/ipv6/conf/all/forwarding"
	if out := harness.CCOK(t, "ctr-a2", forwarding6); strings.TrimSpace(out) != "0" {
		t.Errorf("ctr-a2 IPv6 forwarding = %q, want 0", out)
	}
	if out := harness.CCOK(t, "ctr-rtr", forwarding6); strings.TrimSpace(out) != "1" {
		t.Errorf("ctr-rtr IPv6 forwarding = %q, want 1 (vm config preinit)", out)
	}

	// no CAP_SYS_ADMIN (21), CAP_SYS_MODULE (16), CAP_SYS_RAWIO (17), CAP_SYS_BOOT (22) or CAP_SYS_TIME (25)
	out = harness.CCOK(t, "ctr-a2", "grep CapEff /proc/self/status")
	caps, err := strconv.ParseUint(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(out), "CapEff:")), 16, 64)
	if err != nil {
		t.Fatalf("CapEff %q: %v", out, err)
	}
	for _, c := range []uint{16, 17, 21, 22, 25} {
		if caps&(1<<c) != 0 {
			t.Errorf("container has capability %d: CapEff %x", c, caps)
		}
	}
	if r := harness.CCExec(t, "ctr-a2", "mount -t tmpfs none /mnt"); r.ExitCode == 0 {
		t.Errorf("container can mount a filesystem")
	}

	// the UUID of the VM replaces that of the host, where the host has DMI
	out = harness.CCOK(t, "ctr-a1", "cat /sys/devices/virtual/dmi/id/product_uuid 2>/dev/null || echo none")
	if got := strings.TrimSpace(out); got != a1UUID && got != "none" {
		t.Errorf("product_uuid = %q, want %v", got, a1UUID)
	}

	// snapshot: each container writes to an overlay of its own
	harness.CCOK(t, "ctr-a2", "echo ctr-a2 > /root/snapshot-check")
	if r := harness.CCExec(t, "ctr-a3", "cat /root/snapshot-check"); r.ExitCode == 0 {
		t.Errorf("ctr-a3 sees a file that ctr-a2 wrote: %q", r.Stdout)
	}
	if _, err := os.Stat(filepath.Join(harness.MMFiles, "live", "rootfs", "root", "snapshot-check")); err == nil {
		t.Errorf("ctr-a2 wrote to the filesystem that the containers share")
	}
}

// testDevices checks the device rules that minimega gives each container: the
// devices cgroup controller on cgroup v1, an eBPF program on cgroup v2.
func testDevices(t *testing.T) {
	// allowed devices work
	if out := harness.CCOK(t, "ctr-a2", "head -c 4 /dev/urandom | wc -c; echo live > /dev/null"); strings.TrimSpace(out) != "4" {
		t.Errorf("read /dev/urandom: %q", out)
	}

	// A container may make a node for any device, but may not open one that
	// is not allowed, such as a disk of the host. minimega expands variables
	// in commands itself, so the script has none: sed turns "8:0" into "8 0".
	harness.CCOK(t, "ctr-a2", "rm -f /dev/live-disk; mknod /dev/live-disk b $(sed 's/:/ /' /sys/block/$(ls /sys/block | head -n 1)/dev) && test -b /dev/live-disk")

	r := harness.CCExec(t, "ctr-a2", "head -c 1 /dev/live-disk")
	if r.ExitCode == 0 {
		t.Fatalf("container read a block device of the host")
	}
	if !strings.Contains(r.Stderr, "not permitted") {
		t.Errorf("read of a block device failed with %q, want the device rules to deny it", r.Stderr)
	}
}

func testVolume(t *testing.T) {
	// "vm config volume /scratch <dir>": minimega creates the directory
	dir := filepath.Join(harness.MMFiles, "live", "scratch-a1")

	if err := os.WriteFile(filepath.Join(dir, "from-host"), []byte("live-volume-host\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if out := harness.CCOK(t, "ctr-a1", "cat /scratch/from-host"); strings.TrimSpace(out) != "live-volume-host" {
		t.Errorf("container read %q from the volume", out)
	}

	harness.CCOK(t, "ctr-a1", "echo live-volume-container > /scratch/from-container")
	b, err := os.ReadFile(filepath.Join(dir, "from-container"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(b)); got != "live-volume-container" {
		t.Errorf("host read %q from the volume", got)
	}

	// other containers have no /scratch
	if r := harness.CCExec(t, "ctr-a2", "test -e /scratch"); r.ExitCode == 0 {
		t.Errorf("ctr-a2 has a volume")
	}
}

func testFifo(t *testing.T) {
	// "vm config fifos 1": a named pipe in the instance directory on the
	// host, and at /dev/fifos/fifo0 in the container
	fifo := filepath.Join(mmBase, harness.VMInfo(t)["ctr-a1"]["id"], "fifo0")
	out := filepath.Join(harness.MMFiles, "live", "scratch-a1", "fifo.out")

	harness.CCRun(t, "name=ctr-a1", "background sh -c 'cat /dev/fifos/fifo0 > /scratch/fifo.out'", 1, time.Minute)

	// without a reader in the container, the open fails with ENXIO
	var f *os.File
	harness.Eventually(t, time.Minute, time.Second, func() (err error) {
		f, err = os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		return err
	})
	_, err := f.WriteString("live-fifo\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}

	harness.Eventually(t, time.Minute, time.Second, func() error {
		b, err := os.ReadFile(out)
		if err != nil {
			return err
		}
		if got := strings.TrimSpace(string(b)); got != "live-fifo" {
			return fmt.Errorf("container read %q from the fifo", got)
		}
		return nil
	})
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
	// container to container, container to its router, KVM to container
	harness.CCOK(t, "ctr-a2", "ping -c 3 -W 2 "+ipOf(t, "ctr-a1"))
	harness.CCOK(t, "ctr-a1", "ping -c 3 -W 2 10.20.1.1")
	harness.CCOK(t, "kvm-b1", "ping -c 3 -W 2 10.20.2.11")
	harness.CCOK(t, "ctr-b1", "ping -c 3 -W 2 10.20.2.21")
}

func testPingAcrossRouter(t *testing.T) {
	// lan-a -> lan-b through the container that runs minirouter
	harness.CCEventually(t, "ctr-a1", "ping -c 3 -W 2 10.20.2.11", 2*time.Minute)
	harness.CCEventually(t, "ctr-a3", "ping -c 3 -W 2 10.20.2.21", 2*time.Minute)
	harness.CCEventually(t, "kvm-b1", "ping -c 3 -W 2 "+ipOf(t, "ctr-a2"), 2*time.Minute)

	out := harness.CCOK(t, "ctr-a1", "traceroute -n -w 2 -q 1 10.20.2.11")
	if !strings.Contains(out, "10.20.1.1") {
		t.Errorf("traceroute missing hop 10.20.1.1:\n%s", out)
	}
}

func testHTTPProtonuke(t *testing.T) {
	// protonuke was started in ctr-b1 by topology.mm
	harness.CCEventually(t, "kvm-b1", "timeout 10 wget -q -O /tmp/index.html http://10.20.2.11/ && test -s /tmp/index.html", 2*time.Minute)
	harness.CCEventually(t, "ctr-a3", "timeout 10 wget -q -O /tmp/index.html http://10.20.2.11/ && test -s /tmp/index.html", 2*time.Minute)
}

// --- VM operations -----------------------------------------------------------

func testTop(t *testing.T) {
	// minimega finds the processes of a container in its cgroup
	procs := map[string]int{}
	for _, r := range harness.MustTable(t, "vm top 2") {
		procs[r["name"]], _ = strconv.Atoi(r["procs"])
	}
	for _, s := range topology {
		if procs[s.Name] < 1 {
			t.Errorf("vm top: %v has %d processes", s.Name, procs[s.Name])
		}
	}
	// init, miniccc and the heartbeat loop at the least
	if procs["ctr-a1"] < 3 {
		t.Errorf("vm top: ctr-a1 has %d processes, want >= 3", procs["ctr-a1"])
	}
}

// heartbeat reads the counter that ctr-a1 writes to its volume each second.
func heartbeat(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(harness.MMFiles, "live", "scratch-a1", "heartbeat"))
	if err != nil {
		t.Fatal(err)
	}
	// the container may be between truncating the file and writing to it
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

// testStopStart pauses a container with the cgroup freezer.
func testStopStart(t *testing.T) {
	unified := cgroupUnified(t)
	id := harness.VMInfo(t)["ctr-a1"]["id"]

	beats := func(want bool) {
		t.Helper()
		harness.Eventually(t, 30*time.Second, 500*time.Millisecond, func() error {
			before := heartbeat(t)
			time.Sleep(3 * time.Second)
			if after := heartbeat(t); (after > before) != want {
				return fmt.Errorf("heartbeat went from %d to %d in 3s, want running = %v", before, after, want)
			}
			return nil
		})
	}
	beats(true)

	harness.MustRun(t, "vm stop ctr-a1")
	if s := harness.VMInfo(t)["ctr-a1"]["state"]; s != "PAUSED" {
		t.Errorf("state after stop = %v", s)
	}
	harness.Eventually(t, 30*time.Second, time.Second, func() error { return frozen(t, unified, id, true) })
	beats(false)

	harness.MustRun(t, "vm start ctr-a1")
	if s := harness.VMInfo(t)["ctr-a1"]["state"]; s != "RUNNING" {
		t.Errorf("state after start = %v", s)
	}
	harness.Eventually(t, 30*time.Second, time.Second, func() error { return frozen(t, unified, id, false) })
	beats(true)
	harness.CCEventually(t, "ctr-a1", "true", time.Minute)
}

// gone returns an error if a container that was killed left anything on the
// minimega host.
func gone(t *testing.T, unified bool, id, pid string) error {
	t.Helper()
	for _, dir := range cgroupDirs(unified, id) {
		if _, err := harness.InContainer(t, "ls", "-d", dir); err == nil {
			return fmt.Errorf("cgroup %v still exists", dir)
		}
	}
	if out, _ := harness.InContainer(t, "ip", "netns", "list"); strings.Contains(out, "meganet_"+id+"\n") || strings.Contains(out, "meganet_"+id+" ") {
		return fmt.Errorf("network namespace meganet_%v still exists", id)
	}
	if _, err := harness.InContainer(t, "ls", "-d", "/proc/"+pid); err == nil {
		return fmt.Errorf("process %v still exists", pid)
	}
	return nil
}

// testKillRestart kills a container and starts it again, which launches a new
// init process in a new cgroup on the overlay that the container had.
func testKillRestart(t *testing.T) {
	unified := cgroupUnified(t)
	before := harness.VMInfo(t)["ctr-a2"]

	harness.CCOK(t, "ctr-a2", "echo kept > /root/restart-check")

	harness.MustRun(t, "vm kill ctr-a2")
	harness.Eventually(t, 30*time.Second, time.Second, func() error {
		if s := harness.VMInfo(t)["ctr-a2"]["state"]; s != "QUIT" {
			return fmt.Errorf("state = %v", s)
		}
		return gone(t, unified, before["id"], before["pid"])
	})

	harness.MustRun(t, "vm start ctr-a2")
	harness.Eventually(t, 2*time.Minute, 3*time.Second, func() error {
		r := harness.VMInfo(t)["ctr-a2"]
		if r["state"] != "RUNNING" || r["cc_active"] != "true" {
			return fmt.Errorf("ctr-a2 state=%v cc=%v tags=%v", r["state"], r["cc_active"], r["tags"])
		}
		if r["pid"] == before["pid"] {
			return fmt.Errorf("ctr-a2 still has pid %v", r["pid"])
		}
		return nil
	})

	after := harness.VMInfo(t)["ctr-a2"]
	if after["id"] != before["id"] || after["uuid"] != before["uuid"] {
		t.Errorf("restart changed the VM: id %v -> %v, uuid %v -> %v", before["id"], after["id"], before["uuid"], after["uuid"])
	}
	if procs := strings.Fields(readCgroup(t, unified, after["id"], "freezer", "cgroup.procs")); !slices.Contains(procs, after["pid"]) {
		t.Errorf("cgroup.procs = %v, want it to contain pid %v", procs, after["pid"])
	}
	if out := harness.CCOK(t, "ctr-a2", "cat /root/restart-check"); strings.TrimSpace(out) != "kept" {
		t.Errorf("the overlay of ctr-a2 did not survive the restart: %q", out)
	}
	harness.CCEventually(t, "ctr-a2", "ping -c 2 -W 2 10.20.1.1", 2*time.Minute)
}

func testKillFlushRelaunch(t *testing.T) {
	unified := cgroupUnified(t)
	before := harness.VMInfo(t)["ctr-a3"]
	overlay := "megamount_" + before["id"] + " "

	if out, _ := harness.InContainer(t, "cat", "/proc/mounts"); !strings.Contains(out, overlay) {
		t.Errorf("no overlay mount for ctr-a3 (%v):\n%s", overlay, out)
	}

	harness.MustRun(t, "vm kill ctr-a3")
	harness.Eventually(t, 30*time.Second, time.Second, func() error {
		if s := harness.VMInfo(t)["ctr-a3"]["state"]; s != "QUIT" {
			return fmt.Errorf("state = %v", s)
		}
		return gone(t, unified, before["id"], before["pid"])
	})
	harness.MustRun(t, "vm flush")
	if _, ok := harness.VMInfo(t)["ctr-a3"]; ok {
		t.Fatalf("ctr-a3 still present after flush")
	}
	if out, _ := harness.InContainer(t, "cat", "/proc/mounts"); strings.Contains(out, overlay) {
		t.Errorf("overlay of ctr-a3 is still mounted after flush")
	}

	// relaunch through the queue from a clone of a running container's config
	harness.MustRun(t, "vm config clone ctr-a2")
	harness.MustRun(t, "vm config memory 160")
	harness.MustRun(t, "vm launch container ctr-a4")
	harness.MustRun(t, "vm launch")
	harness.MustRun(t, "vm start ctr-a4")
	harness.MustRun(t, "clear vm config")

	harness.Eventually(t, 3*time.Minute, 5*time.Second, func() error {
		r := harness.VMInfo(t)["ctr-a4"]
		if r["cc_active"] != "true" {
			return fmt.Errorf("ctr-a4 state=%v cc=%v tags=%v", r["state"], r["cc_active"], r["tags"])
		}
		if ips := harness.List(r["ip"]); len(ips) == 0 || !strings.HasPrefix(ips[0], "10.20.1.") {
			return fmt.Errorf("ctr-a4 ip=%v", r["ip"])
		}
		return nil
	})
	harness.CCOK(t, "ctr-a4", "ping -c 2 -W 2 10.20.2.21")

	r := harness.VMInfo(t)["ctr-a4"]
	if r["type"] != "container" || !strings.Contains(r["tags"], `"lan":"a"`) {
		t.Errorf("ctr-a4 did not inherit ctr-a2's config: type=%v tags=%v", r["type"], r["tags"])
	}
	file, want := "memory.limit_in_bytes", strconv.Itoa(160<<20)
	if unified {
		file = "memory.max"
	}
	if got := readCgroup(t, unified, r["id"], "memory", file); got != want {
		t.Errorf("ctr-a4 %v = %v, want %v", file, got, want)
	}
}

// --- teardown ----------------------------------------------------------------

func testClearNamespace(t *testing.T) {
	if _, err := harness.Raw("clear namespace " + harness.Namespace); err != nil {
		t.Fatalf("clear namespace: %v", err)
	}

	resps, err := harness.Raw("namespace")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range harness.ToRows(resps) {
		if r["namespace"] == harness.Namespace {
			t.Errorf("namespace %v still exists", harness.Namespace)
		}
	}

	unified := cgroupUnified(t)
	harness.Eventually(t, time.Minute, 3*time.Second, func() error {
		for _, proc := range []string{"qemu", "live-container"} {
			out, _ := harness.InContainer(t, "pgrep", "-f", "-c", proc)
			if n := strings.TrimSpace(out); n != "0" && n != "" {
				return fmt.Errorf("%v %v processes still running", n, proc)
			}
		}
		out, err := harness.InContainer(t, "ovs-vsctl", "list-ports", "mega_bridge")
		if err == nil && strings.TrimSpace(out) != "" {
			return fmt.Errorf("ports left on mega_bridge: %v", strings.Fields(out))
		}
		if out, _ := harness.InContainer(t, "cat", "/proc/mounts"); strings.Contains(out, "megamount_") {
			return fmt.Errorf("overlays still mounted:\n%s", out)
		}
		if out, _ := harness.InContainer(t, "ip", "netns", "list"); strings.Contains(out, "meganet_") {
			return fmt.Errorf("network namespaces left: %s", out)
		}
		// minimega keeps its own cgroup until it exits, without any VM in it
		for _, dir := range cgroupDirs(unified, "") {
			out, _ := harness.InContainer(t, "find", dir, "-mindepth", "1", "-type", "d")
			if strings.TrimSpace(out) != "" {
				return fmt.Errorf("cgroups left: %s", out)
			}
		}
		return nil
	})
}
