// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package main

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/bridge"
	"github.com/sandia-minimega/minimega/v2/internal/meshage"
	"github.com/sandia-minimega/minimega/v2/internal/nscapture"
	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	"github.com/sandia-minimega/minimega/v2/pkg/minicli"

	"github.com/gopacket/gopacket/pcapgo"
)

func TestParseNSCaptureOptions(t *testing.T) {
	ns := &Namespace{Name: "nscapture-test"}

	o, err := parseNSCaptureOptions(ns, []string{"ports=edge", "vlans=101,102", `filter=tcp port 80`, "snaplen=128",
		"format=pcapng", "attribute=stream", "names=false", "window=500ms", "maxsize=10240M", "bridges=a,b", "vms=web"}, false)
	if err != nil {
		t.Fatal(err)
	}

	if len(o.vlanIDs) != 2 || o.filter != "tcp port 80" || o.snaplen != 128 || o.format != "pcapng" ||
		o.attribute != "stream" || o.names || o.window != 500*time.Millisecond || o.maxSize != 10240<<20 ||
		len(o.bridges) != 2 || o.vms[0] != "web" {
		t.Errorf("options %+v", o)
	}

	if o, err := parseNSCaptureOptions(ns, nil, false); err != nil || o.snaplen != 1600 || o.window != time.Second {
		t.Errorf("defaults %+v %v", o, err)
	}

	for _, opts := range [][]string{
		{"ports=all"},
		{"head=x"},
		{"token=1"},
		{`filter=host "a"`},
		{`filter=a\b`},
		{"filter=tcp port eighty"},
		{"snaplen=13"},
		{"snaplen=65536"},
		{"format=pcapx"},
		{"attribute=port", "format=pcap"},
		{"names=maybe"},
		{"window=50ms"},
		{"window=11s"},
		{"maxsize=1T"},
		{"colour=blue"},
		{"vlans=0"},
	} {
		if _, err := parseNSCaptureOptions(ns, opts, false); err == nil {
			t.Errorf("options %q did not fail", opts)
		}
	}

	if _, err := parseNSCaptureOptions(ns, []string{"head=h"}, true); err == nil {
		t.Error("member options without id and token did not fail")
	}

	o.head, o.id, o.token, o.gen = "head node", 3, 42, 2
	o.vlanIDs = []int{101}
	args := []string{"capture", "pcap", "namespace", quoteArg("/f/a b.pcap")}
	for _, a := range o.memberArgs() {
		args = append(args, quoteArg(a))
	}

	c, err := minicli.Compile(strings.Join(args, " "))
	if err != nil || c == nil {
		t.Fatalf("member command does not compile: %v", err)
	}

	o2, err := parseNSCaptureOptions(ns, c.ListArgs["option"], true)
	if err != nil {
		t.Fatal(err)
	}
	if o2.head != "head node" || o2.id != 3 || o2.token != 42 || o2.gen != 2 || o2.filter != o.filter || c.StringArgs["filename"] != "/f/a b.pcap" {
		t.Errorf("member options %+v from %q", o2, c.ListArgs["option"])
	}

	// a member namespace with other defaults uses the filter and format of
	// the head, also when they are empty
	member := &Namespace{Name: "nscapture-test"}
	member.captures.Filter = "tcp"
	member.captures.Format = pcapfile.FormatPcapng

	o, err = parseNSCaptureOptions(ns, []string{"filter="}, false)
	if err != nil {
		t.Fatal(err)
	}
	o.head, o.id, o.token = "head", 1, 2

	o2, err = parseNSCaptureOptions(member, o.memberArgs(), true)
	if err != nil {
		t.Fatal(err)
	}
	if o2.filter != "" || o2.format != pcapfile.FormatPcap {
		t.Errorf("member filter %q and format %q, want no filter and pcap", o2.filter, o2.format)
	}
}

// TestNSCaptureNamespaceSnapLen checks that a start refuses a namespace
// snaplen above the limit of namespace captures, unless snaplen= replaces it.
func TestNSCaptureNamespaceSnapLen(t *testing.T) {
	ns := &Namespace{Name: "nscapture-test"}
	ns.captures.SnapLen = nsCaptureMaxSnapLen + 1

	if o, err := parseNSCaptureOptions(ns, nil, false); err == nil {
		t.Errorf("namespace snaplen %v accepted as %v", ns.captures.SnapLen, o.snaplen)
	}

	if o, err := parseNSCaptureOptions(ns, []string{"snaplen=128"}, false); err != nil || o.snaplen != 128 {
		t.Errorf("snaplen=128 with namespace snaplen %v: %v %v", ns.captures.SnapLen, o.snaplen, err)
	}
}

// TestPortTableIndexes checks that a port keeps its index when another port
// leaves, and that a new port never gets the index of an old one.
func TestPortTableIndexes(t *testing.T) {
	tap := func(name, mac string) bridge.Tap {
		return bridge.Tap{Name: name, MAC: mac, VLAN: 101}
	}
	indexes := func(ports []nscapture.PortInfo) map[string]uint32 {
		res := map[string]uint32{}
		for _, p := range ports {
			res[p.Port+"/"+p.MAC] = p.Index
		}
		return res
	}

	idx := &portIndexes{}

	noIface := func(string, string) int { return -1 }
	first := indexes(portTable([]bridge.Tap{tap("a", "m1"), tap("b", "m2"), tap("c", "m3")}, idx, noIface))
	later := indexes(portTable([]bridge.Tap{tap("b", "m2"), tap("c", "m3"), tap("d", "m4"), tap("a", "m5")}, idx, noIface))

	for _, p := range []string{"b/m2", "c/m3"} {
		if later[p] != first[p] {
			t.Errorf("index of %v changed from %v to %v", p, first[p], later[p])
		}
	}

	seen := map[uint32]string{}
	for _, m := range []map[string]uint32{first, later} {
		for p, i := range m {
			if old, ok := seen[i]; ok && old != p {
				t.Errorf("index %v is used by %v and %v", i, old, p)
			}
			seen[i] = p
		}
	}
}

// TestPortTableVMIface checks that the port table gives the interface of the
// VM that uses each tap, and -1 for a host tap.
func TestPortTableVMIface(t *testing.T) {
	taps := []bridge.Tap{
		{Name: "mega_tap1", MAC: "m1", VLAN: 101, Owner: "web01"},
		{Name: "mega_tap2", MAC: "m2", VLAN: 102, Owner: "web01"},
		{Name: "mega_tap3", MAC: "m3", VLAN: 101},
	}
	vmIface := func(vm, tap string) int {
		if vm == "web01" && tap == "mega_tap2" {
			return 1
		}
		return 0
	}

	ports := portTable(taps, &portIndexes{}, vmIface)

	var got []int
	for _, p := range ports {
		got = append(got, p.VMIface)
	}
	if want := []int{0, 1, -1}; !slices.Equal(got, want) {
		t.Errorf("VM interfaces %v, want %v", got, want)
	}
}

func TestNSCapturePatterns(t *testing.T) {
	for in, want := range map[string]string{
		"capture pcap namespace":                     "",
		"capture pcap namespace exp/files/a.pcap":    "exp/files/a.pcap",
		"capture pcap delete namespace all":          "",
		`capture pcap namespace a.pcap filter="tcp"`: "a.pcap",
	} {
		c, err := minicli.Compile(in)
		if err != nil || c == nil {
			t.Fatalf("Compile(%q): %v", in, err)
		}
		if c.StringArgs["filename"] != want {
			t.Errorf("%q: filename %q", in, c.StringArgs["filename"])
		}
	}
}

// TestNSCaptureInProcess runs a capture with this node as head and member:
// member streams send through the local sender, the head merges and stops.
func TestNSCaptureInProcess(t *testing.T) {
	if hostname == "" {
		hostname = "testhost"
	}

	ns := "nscapture-inprocess"
	path := filepath.Join(t.TempDir(), "merged.pcap")

	o := nsCaptureOptions{snaplen: 1600, window: 100 * time.Millisecond, attribute: nscapture.AttributePort,
		head: hostname, id: 1, token: 99}

	hc, err := newHeadCapture(ns, []string{hostname}, o, path, "test")
	if err != nil {
		t.Fatal(err)
	}
	hc.setState(nscapture.StateRunning, "")

	// a member with two bridges, registered as the fan-out would
	m := &memberCapture{
		key:      memberKey{ns: ns, head: hostname, id: 1, token: 99},
		opts:     o,
		streams:  map[string]*memberStream{},
		headSeen: time.Now(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	nsMembers.mu.Lock()
	nsMembers.m[m.key] = m
	nsMembers.mu.Unlock()

	base := time.Now().Add(-time.Second)
	for i, br := range []string{"br_a", "br_b"} {
		s := nscapture.NewStream(nscapture.StreamConfig{Namespace: ns, Head: hostname, Host: hostname, Bridge: br,
			ID: 1, Token: 99}, nsSender{}, nil)
		m.streams[br] = &memberStream{bridge: br, stream: s}

		for j := 0; j < 50; j++ {
			ts := base.Add(time.Duration(j*2+i) * time.Millisecond)
			data := make([]byte, 60)
			s.WriteRecord(0, pcapfile.AppendRecord(nil, ts, 60, data), pcapfile.RecordOptions{})
		}
	}

	time.Sleep(60 * time.Millisecond)
	for _, s := range m.streams {
		s.stream.Flush()
	}

	waitForCond(t, "packets", func() bool {
		hc.mu.Lock()
		defer hc.mu.Unlock()
		return hc.sum.Packets == 100
	})

	rows := nsCaptureStopHead(ns, "1", 10*time.Second)
	if len(rows) != 3 || rows[0][3] != "stopped" || rows[0][5] != "100" {
		t.Fatalf("final rows %q", rows)
	}
	for _, row := range rows {
		if len(row) != len(nsCaptureHeader) {
			t.Errorf("row has %d columns", len(row))
		}
	}
	if rows[1][1] != hostname || rows[1][2] != "br_a" || rows[2][2] != "br_b" {
		t.Errorf("stream rows %q", rows[1:])
	}

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Error("member did not stop on OpStop")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var last int64
	var n int
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if ts := ci.Timestamp.UnixNano(); ts < last {
			t.Errorf("record %d out of order", n)
		} else {
			last = ts
		}
		n++
	}
	if n != 100 {
		t.Errorf("file has %d records, want 100", n)
	}

	if len(nsHeads.list(ns)) != 0 {
		t.Error("capture still listed after delete")
	}
}

func waitForCond(t *testing.T, what string, fn func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %v", what)
}

// newTestHead starts a head capture of ns on hosts that answer nothing. Call
// endTestHead to send the final batches of hosts.
func newTestHead(t *testing.T, ns string, id int, hosts []string) *headCapture {
	t.Helper()

	setTestHostname()

	o := nsCaptureOptions{snaplen: 1600, window: 100 * time.Millisecond, attribute: nscapture.AttributePort,
		head: hostname, id: id, token: uint64(1000 + id)}

	hc, err := newHeadCapture(ns, hosts, o, filepath.Join(t.TempDir(), "merged.pcap"), "test")
	if err != nil {
		t.Fatal(err)
	}
	hc.setState(nscapture.StateRunning, "")

	t.Cleanup(func() {
		hc.signalStop()
		endTestHead(hc, hosts...)
		<-hc.done
		nsHeads.remove(ns, id)
	})

	return hc
}

// setTestHostname sets hostname when the test runs without one. Call it
// before a test uses hostname.
func setTestHostname() {
	if hostname == "" {
		hostname = "testhost"
	}
}

// endTestHead gives hc the host-level final batches of hosts.
func endTestHead(hc *headCapture, hosts ...string) {
	for _, h := range hosts {
		hc.inbox <- &nscapture.Batch{Namespace: hc.ns, Head: hostname, ID: hc.id, Token: hc.token, Host: h, Final: true}
	}
}

func cpuTime(t *testing.T) time.Duration {
	t.Helper()

	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		t.Fatal(err)
	}

	return time.Duration(ru.Utime.Nano() + ru.Stime.Nano())
}

// TestNSCaptureStopNoSpin checks that the head does not use the CPU while it
// waits for the final batches after a stop.
func TestNSCaptureStopNoSpin(t *testing.T) {
	hc := newTestHead(t, "nscapture-spin", 1, []string{"ghost"})

	hc.setState(nscapture.StateStopping, "")
	hc.stopOnce.Do(func() { close(hc.stop) })

	start := cpuTime(t)
	time.Sleep(500 * time.Millisecond)
	if used := cpuTime(t) - start; used > 200*time.Millisecond {
		t.Errorf("used %v of CPU in 500ms while it waits for the final batches", used)
	}
}

// TestNSCaptureStopHeadParallel checks that a stop of many captures waits
// for all of them at the same time.
func TestNSCaptureStopHeadParallel(t *testing.T) {
	ns := "nscapture-parallel"
	for id := 1; id <= 3; id++ {
		newTestHead(t, ns, id, []string{"ghost"})
	}

	start := time.Now()
	rows := nsCaptureStopHead(ns, Wildcard, 300*time.Millisecond)
	if d := time.Since(start); d > 600*time.Millisecond {
		t.Errorf("stop of 3 captures took %v, want about 300ms", d)
	}
	if len(rows) != 3 {
		t.Errorf("got %d rows, want 3", len(rows))
	}
}

// TestNSCaptureMemberNewToken checks that a member starts a capture that has
// the ID of an old capture of the same head, with a new token.
func TestNSCaptureMemberNewToken(t *testing.T) {
	if hostname == "" {
		hostname = "testhost"
	}

	ns := &Namespace{Name: "nscapture-token"}
	start := func(token string) (*memberCapture, error) {
		c := &minicli.Command{
			StringArgs: map[string]string{"filename": "a.pcap"},
			ListArgs:   map[string][]string{"option": {"head=" + hostname, "id=1", "token=" + token}},
		}
		if err := cliNSCaptureMember(ns, c, &minicli.Response{}); err != nil {
			return nil, err
		}

		nsMembers.mu.Lock()
		defer nsMembers.mu.Unlock()
		for _, m := range nsMembers.m {
			if m.key.ns == ns.Name && m.opts.token == mustParseUint(t, token) {
				return m, nil
			}
		}
		t.Fatalf("no member with token %v", token)
		return nil, nil
	}

	old, err := start("1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { old.requestStop(""); <-old.done })

	m, err := start("2")
	if err != nil {
		t.Fatalf("start with a new token: %v", err)
	}
	t.Cleanup(func() { m.requestStop(""); <-m.done })

	if _, err := start("2"); err == nil {
		t.Error("second start with the same token did not fail")
	}
}

func mustParseUint(t *testing.T, s string) uint64 {
	t.Helper()

	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// flakySender fails the first fails sends, then records the bodies.
type flakySender struct {
	mu     sync.Mutex
	fails  int
	bodies []interface{}
}

func (f *flakySender) Send(_ string, body interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fails > 0 {
		f.fails--
		return errors.New("no route")
	}
	f.bodies = append(f.bodies, body)
	return nil
}

// TestNSCaptureMemberFinalRetry checks that a member sends its host-level
// final batch again when the send fails.
func TestNSCaptureMemberFinalRetry(t *testing.T) {
	sender := &flakySender{fails: 2}
	old := nsCaptureSender
	nsCaptureSender = sender
	t.Cleanup(func() { nsCaptureSender = old })

	m := &memberCapture{
		key:     memberKey{ns: "nscapture-final", head: "head", id: 1, token: 5},
		opts:    nsCaptureOptions{token: 5},
		streams: map[string]*memberStream{},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	m.requestStop("")

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("member did not stop")
	}

	waitForCond(t, "the final batch", func() bool {
		sender.mu.Lock()
		defer sender.mu.Unlock()
		return len(sender.bodies) > 0
	})

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.bodies) != 1 {
		t.Fatalf("sent %d messages, want the final batch", len(sender.bodies))
	}
	if b, ok := sender.bodies[0].(nscapture.Batch); !ok || !b.Final || b.Bridge != "" {
		t.Errorf("sent %+v, want the host-level final batch", sender.bodies[0])
	}
}

// fakeEdgeBridge is an edgeBridge with one edge tap. start gives the result
// of each StartEdgeStream call; taps blocks EdgeTaps when it is not nil.
// stops records the IDs that StopCapture gets.
type fakeEdgeBridge struct {
	mu     sync.Mutex
	start  []error
	starts int
	stops  []int
	taps   chan struct{}
}

func (b *fakeEdgeBridge) EdgeTaps(bridge.EdgeSelector) []bridge.Tap {
	if b.taps != nil {
		<-b.taps
	}
	return []bridge.Tap{{Name: "mega_tap1", MAC: "00:00:00:00:00:01", VLAN: 101}}
}

func (b *fakeEdgeBridge) StartEdgeStream(bridge.EdgeSelector, bridge.CaptureConfig, pcapfile.Writer, bridge.MirrorTags) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.starts++
	var err error
	if len(b.start) > 0 {
		err, b.start = b.start[0], b.start[1:]
	}
	return b.starts, err
}

func (b *fakeEdgeBridge) CaptureInfo(int) (bridge.CaptureInfo, bool) {
	return bridge.CaptureInfo{}, false
}
func (b *fakeEdgeBridge) MirrorApplied(int) (bool, error) { return true, nil }

func (b *fakeEdgeBridge) StopCapture(id int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stops = append(b.stops, id)
	return nil
}

func useFakeEdgeBridges(t *testing.T, brs map[string]*fakeEdgeBridge) {
	t.Helper()

	oldNames, oldLookup := nsBridgeNames, nsLookupBridge
	t.Cleanup(func() { nsBridgeNames, nsLookupBridge = oldNames, oldLookup })

	nsBridgeNames = func() []string {
		var res []string
		for name := range brs {
			res = append(res, name)
		}
		sort.Strings(res)
		return res
	}
	nsLookupBridge = func(name string) (edgeBridge, bool) {
		br, ok := brs[name]
		return br, ok
	}
}

// newTestMember registers a member of the capture of hc on this host.
func newTestMember(hc *headCapture) *memberCapture {
	m := &memberCapture{
		key:      memberKey{ns: hc.ns, head: hostname, id: hc.id, token: hc.token},
		opts:     hc.opts,
		streams:  map[string]*memberStream{},
		headSeen: time.Now(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}

	nsMembers.mu.Lock()
	nsMembers.m[m.key] = m
	nsMembers.mu.Unlock()

	return m
}

// TestNSCaptureReconcileStartFails checks that a stream that did not start
// does not start again with the same sequence numbers, which the head drops
// as duplicates.
func TestNSCaptureReconcileStartFails(t *testing.T) {
	failed := &fakeEdgeBridge{start: []error{errors.New("mirror limit")}}
	gone := &fakeEdgeBridge{start: []error{bridge.ErrNoEdgePorts}}
	useFakeEdgeBridges(t, map[string]*fakeEdgeBridge{"br_failed": failed, "br_gone": gone})
	setTestHostname()

	hc := newTestHead(t, "nscapture-startfails", 1, []string{hostname})
	m := newTestMember(hc)

	if err := m.reconcile(); err == nil || !strings.Contains(err.Error(), "mirror limit") {
		t.Errorf("first reconcile: %v, want the mirror error", err)
	}
	if err := m.reconcile(); err != nil {
		t.Errorf("second reconcile: %v", err)
	}

	if failed.starts != 1 || gone.starts != 2 {
		t.Errorf("starts: br_failed %d, br_gone %d, want 1 and 2", failed.starts, gone.starts)
	}

	nsCaptureStopHead(hc.ns, "1", 5*time.Second)
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("member did not stop")
	}

	hc.mu.Lock()
	defer hc.mu.Unlock()
	if hc.state != nscapture.StateStopped || hc.sum.Dup != 0 {
		t.Errorf("state %v with %d duplicate batches, want stopped and 0", hc.state, hc.sum.Dup)
	}

	var found bool
	for _, s := range hc.streams {
		if s.Bridge != "br_failed" {
			continue
		}
		found = true
		if s.State != nscapture.StateFailed || !strings.Contains(s.Error, "mirror limit") {
			t.Errorf("br_failed stream %v %q, want failed with the mirror error", s.State, s.Error)
		}
	}
	if !found {
		t.Errorf("no br_failed stream in %+v", hc.streams)
	}
}

// TestNSCaptureAckDuringReconcile checks that an ack reaches a stream while
// a reconcile waits for Open vSwitch.
func TestNSCaptureAckDuringReconcile(t *testing.T) {
	slow := &fakeEdgeBridge{taps: make(chan struct{})}
	useFakeEdgeBridges(t, map[string]*fakeEdgeBridge{"br_slow": slow})

	hc := newTestHead(t, "nscapture-ackwait", 1, nil)
	m := newTestMember(hc)
	t.Cleanup(func() {
		nsMembers.mu.Lock()
		delete(nsMembers.m, m.key)
		nsMembers.mu.Unlock()
	})

	s := nscapture.NewStream(nscapture.StreamConfig{Namespace: hc.ns, Head: hostname, Host: hostname, Bridge: "br_slow",
		ID: hc.id, Token: hc.token}, &flakySender{}, nil)
	unknown := make(chan struct{})
	s.OnUnknown = func() { close(unknown) }
	m.streams["br_slow"] = &memberStream{bridge: "br_slow", stream: s, indexes: &portIndexes{}, checked: true}

	reconciled := make(chan struct{})
	go func() {
		defer close(reconciled)
		m.reconcile()
	}()
	t.Cleanup(func() {
		close(slow.taps)
		<-reconciled
		s.Close()
	})

	// wait until the reconcile waits in EdgeTaps
	time.Sleep(50 * time.Millisecond)

	nsCaptureControls <- nscapture.Control{Op: nscapture.OpAck, Namespace: hc.ns, Head: hostname, ID: hc.id,
		Token: hc.token, Bridge: "br_slow", Unknown: true}

	select {
	case <-unknown:
	case <-time.After(2 * time.Second):
		t.Error("the ack did not reach the stream while a reconcile runs")
	}
}

// TestNSCaptureDelHosts checks that ns del-hosts stops the member captures of
// the hosts that it removes, and that the head does not wait for them.
func TestNSCaptureDelHosts(t *testing.T) {
	setTestHostname()

	ns := &Namespace{Name: "nscapture-delhosts", Hosts: map[string]bool{hostname: true, "ghost": true}}
	hc := newTestHead(t, ns.Name, 1, []string{hostname, "ghost"})
	m := newTestMember(hc)

	del := func(host string) {
		c := &minicli.Command{StringArgs: map[string]string{"hostname": host}}
		if err := cliNamespaceDelHost(ns, c, &minicli.Response{}); err != nil {
			t.Fatal(err)
		}
	}

	del(hostname)
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Error("the member capture of a removed host did not stop")
	}

	del("ghost")
	rows := nsCaptureStopHead(ns.Name, "1", 3*time.Second)
	if len(rows) == 0 || rows[0][3] != nscapture.StateStopped {
		t.Errorf("final rows %q, want a stopped capture", rows)
	}
}

// TestNSCaptureDelHostsAll checks that ns del-hosts all stops the member
// captures of every host.
func TestNSCaptureDelHostsAll(t *testing.T) {
	setTestHostname()

	ns := &Namespace{Name: "nscapture-delhosts-all", Hosts: map[string]bool{hostname: true, "ghost": true}}
	hc := newTestHead(t, ns.Name, 1, []string{hostname, "ghost"})
	m := newTestMember(hc)

	c := &minicli.Command{StringArgs: map[string]string{"hostname": Wildcard}}
	if err := cliNamespaceDelHost(ns, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Error("the member capture did not stop")
	}

	if hosts := hc.getHosts(); len(hosts) != 0 {
		t.Errorf("capture hosts %v after del-hosts all, want none", hosts)
	}
}

// headBatch returns a batch of the stream br_a of this host for hc, with a
// record at each of times.
func headBatch(hc *headCapture, gen, seq uint64, through time.Time, times ...time.Time) *nscapture.Batch {
	b := &nscapture.Batch{Namespace: hc.ns, Head: hostname, ID: hc.id, Token: hc.token, Host: hostname, Bridge: "br_a",
		Gen: gen, Seq: seq, Through: through.UnixNano(), Format: nscapture.FormatClassic}
	for _, ts := range times {
		b.Data = pcapfile.AppendRecord(b.Data, ts, 60, make([]byte, 60))
		b.Packets++
		b.Captured++
	}

	return b
}

// TestNSCaptureReAddHost checks that a host that ns del-hosts removed and ns
// add-hosts added again gets a new member, whose batches go into the file,
// while the late batches of the old member do not.
func TestNSCaptureReAddHost(t *testing.T) {
	useFakeEdgeBridges(t, nil)
	setTestHostname()

	ns := "nscapture-readd"
	hc := newTestHead(t, ns, 1, []string{hostname})
	old := newTestMember(hc)

	packets := func(n uint64) func() bool {
		return func() bool {
			hc.mu.Lock()
			defer hc.mu.Unlock()
			return hc.sum.Packets == n
		}
	}

	now := time.Now()
	hc.inbox <- headBatch(hc, 0, 1, now, now.Add(-time.Second))
	waitForCond(t, "the first record", packets(1))

	nsCaptureDelHosts(ns, []string{hostname})
	select {
	case <-old.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the old member did not stop")
	}

	waitForCond(t, "no streams of the removed host", func() bool {
		hc.mu.Lock()
		defer hc.mu.Unlock()
		return len(hc.streams) == 0 && hc.state == nscapture.StateRunning
	})

	add, gen := hc.addHosts([]string{hostname})
	if !slices.Equal(add, []string{hostname}) || gen != 1 {
		t.Fatalf("addHosts = %v %v, want this host and start 1", add, gen)
	}

	c := &minicli.Command{
		StringArgs: map[string]string{"filename": "a.pcap"},
		ListArgs: map[string][]string{"option": {"head=" + hostname, "id=1", "token=" + strconv.FormatUint(hc.token, 10),
			"gen=1"}},
	}
	if err := cliNSCaptureMember(&Namespace{Name: ns}, c, &minicli.Response{}); err != nil {
		t.Fatal(err)
	}

	// the late batches of the old member, then a batch of the new member
	now = time.Now()
	hc.inbox <- headBatch(hc, 0, 2, now, now.Add(-time.Second))
	hc.inbox <- &nscapture.Batch{Namespace: ns, Head: hostname, ID: hc.id, Token: hc.token, Host: hostname, Final: true}
	hc.inbox <- headBatch(hc, 1, 1, now, now.Add(-time.Second))
	waitForCond(t, "the record of the new member", packets(2))

	hc.mu.Lock()
	var streams []string
	for _, s := range hc.streams {
		streams = append(streams, s.Host+"/"+s.Bridge+" "+s.State)
	}
	hc.mu.Unlock()
	if want := []string{hostname + "/br_a " + nscapture.StateRunning}; !slices.Equal(streams, want) {
		t.Errorf("streams %q, want %q", streams, want)
	}

	rows := nsCaptureStopHead(ns, "1", 5*time.Second)
	if len(rows) < 2 || rows[0][3] != nscapture.StateStopped || rows[0][5] != "2" || rows[1][3] != nscapture.StateStopped {
		t.Errorf("final rows %q, want a stopped capture and stream with 2 packets", rows)
	}
}

// TestNSCaptureMemberNewStart checks that the member of a new start, after ns
// del-hosts and add-hosts, stops the member of the older start, and that a
// start that is not newer fails.
func TestNSCaptureMemberNewStart(t *testing.T) {
	useFakeEdgeBridges(t, nil)
	setTestHostname()

	hc := newTestHead(t, "nscapture-newstart", 1, nil)
	old := newTestMember(hc)

	ns := &Namespace{Name: hc.ns}
	start := func(gen string) error {
		c := &minicli.Command{
			StringArgs: map[string]string{"filename": "a.pcap"},
			ListArgs: map[string][]string{"option": {"head=" + hostname, "id=1", "token=" + strconv.FormatUint(hc.token, 10),
				"gen=" + gen}},
		}
		return cliNSCaptureMember(ns, c, &minicli.Response{})
	}

	if err := start("1"); err != nil {
		t.Fatalf("start 1: %v", err)
	}

	select {
	case <-old.done:
	default:
		t.Error("the member of the older start still runs")
	}

	m := nsMembers.get(old.key)
	if m == nil || m.opts.gen != 1 {
		t.Fatalf("registered member %+v, want start 1", m)
	}
	t.Cleanup(func() { m.requestStop(""); <-m.done })

	for _, gen := range []string{"0", "1"} {
		if err := start(gen); err == nil || !strings.Contains(err.Error(), "already runs") {
			t.Errorf("start %v: %v, want an error", gen, err)
		}
	}

	// a stop for the older start does not stop the new member
	nsCaptureControls <- nscapture.Control{Op: nscapture.OpStop, Namespace: hc.ns, Head: hostname, ID: hc.id, Token: hc.token}
	time.Sleep(100 * time.Millisecond)
	select {
	case <-m.done:
		t.Fatal("the stop for start 0 stopped the member of start 1")
	default:
	}

	nsCaptureControls <- nscapture.Control{Op: nscapture.OpStop, Namespace: hc.ns, Head: hostname, ID: hc.id, Token: hc.token, Gen: 1}
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stop for start 1 did not stop its member")
	}
}

// TestNSCaptureMemberStopUnreachableHead checks that a member stop does not
// wait for the final batch when the head is not reachable.
func TestNSCaptureMemberStopUnreachableHead(t *testing.T) {
	setTestHostname()

	sender := &flakySender{fails: 1 << 30}
	old := nsCaptureSender
	nsCaptureSender = sender
	t.Cleanup(func() { nsCaptureSender = old })

	m := &memberCapture{
		key:     memberKey{ns: "nscapture-unreachable", head: "gone", id: 1, token: 5},
		opts:    nsCaptureOptions{token: 5},
		streams: map[string]*memberStream{},
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	nsMembers.mu.Lock()
	nsMembers.m[m.key] = m
	nsMembers.mu.Unlock()

	start := time.Now()
	m.requestStop("")

	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Fatal("member did not stop")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("stop took %v, want no wait for the head", d)
	}
	if nsMembers.get(m.key) != nil {
		t.Error("the stopped member is still registered")
	}
}

// TestNSCaptureShutdownFailedStream checks that a member stop does not stop
// a capture for a stream that did not start: its capture ID is not its own.
func TestNSCaptureShutdownFailedStream(t *testing.T) {
	setTestHostname()

	failed := &fakeEdgeBridge{}
	ok := &fakeEdgeBridge{}
	useFakeEdgeBridges(t, map[string]*fakeEdgeBridge{"br_failed": failed, "br_ok": ok})

	sender := &flakySender{}
	oldSender := nsCaptureSender
	nsCaptureSender = sender
	t.Cleanup(func() { nsCaptureSender = oldSender })

	newStream := func(br string) *nscapture.Stream {
		return nscapture.NewStream(nscapture.StreamConfig{Namespace: "nscapture-failedstream", Head: "head", Host: "member",
			Bridge: br, ID: 1, Token: 5}, sender, nil)
	}

	m := &memberCapture{
		key:  memberKey{ns: "nscapture-failedstream", head: "head", id: 1, token: 5},
		opts: nsCaptureOptions{token: 5},
		streams: map[string]*memberStream{
			"br_failed": {bridge: "br_failed", stream: newStream("br_failed"), failed: true},
			"br_ok":     {bridge: "br_ok", capID: 7, stream: newStream("br_ok")},
		},
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	m.requestStop("")
	<-m.done

	failed.mu.Lock()
	defer failed.mu.Unlock()
	ok.mu.Lock()
	defer ok.mu.Unlock()
	if len(failed.stops) != 0 || !slices.Equal(ok.stops, []int{7}) {
		t.Errorf("StopCapture calls: br_failed %v, br_ok %v, want none and [7]", failed.stops, ok.stops)
	}
}

// TestNSCaptureAckRecv checks that an ack has the receive time only when it
// acks the batch that arrived: the skew estimate pairs it with the send time
// of that batch.
func TestNSCaptureAckRecv(t *testing.T) {
	setTestHostname()

	hc := &headCapture{ns: "exp", id: 1, token: 5}
	b := &nscapture.Batch{Host: "member", Bridge: "br", Gen: 2, Seq: 4, SentAt: 1234}

	if a := hc.ack(b, 4); a.HeadRecv != 1234 || a.Seq != 4 || a.Gen != 2 || a.Bridge != "br" {
		t.Errorf("ack of batch 4: %+v", a)
	}
	for _, seq := range []uint64{3, 5} {
		if a := hc.ack(b, seq); a.HeadRecv != 0 || a.Seq != seq {
			t.Errorf("ack of %v after batch 4: %+v, want no receive time", seq, a)
		}
	}
}

// TestMemberVMIface checks that a member finds the interface of the VM that
// uses a tap.
func TestMemberVMIface(t *testing.T) {
	ns := &Namespace{Name: "exp", VMs: VMs{m: map[int]VM{
		1: &KvmVM{BaseVM: &BaseVM{ID: 1, Name: "web", BaseConfig: BaseConfig{
			Networks: NetConfigs{{Tap: "mega_tap1"}, {Tap: "mega_tap2"}},
		}}},
	}}}
	m := &memberCapture{ns: ns}

	for _, tc := range []struct {
		vm, tap string
		want    int
	}{
		{"web", "mega_tap1", 0},
		{"web", "mega_tap2", 1},
		{"web", "mega_tap9", -1},
		{"db", "mega_tap1", -1},
	} {
		if got := m.vmIface(tc.vm, tc.tap); got != tc.want {
			t.Errorf("vmIface(%v, %v) = %v, want %v", tc.vm, tc.tap, got, tc.want)
		}
	}
}

// TestNSCaptureWireGob checks that the capture messages pass the mesh
// encoding, with the types that minimega registers.
func TestNSCaptureWireGob(t *testing.T) {
	batch := nscapture.Batch{Namespace: "exp", Head: "head", ID: 3, Token: 9, Host: "c1", Bridge: "br", Gen: 2, Seq: 4,
		Data: []byte{1, 2, 3}, Format: nscapture.FormatNg, Finals: map[string]uint64{"br": 4},
		PortTable: []nscapture.PortInfo{
			{Index: 1, Port: "mega_tap1", MAC: "00:00:00:00:00:01", VLAN: 101, VM: "web", VMIface: 0, Addrs: []string{"10.0.0.1"}},
			{Index: 2, Port: "mega_tap2", VLAN: 101, VMIface: -1},
		}}
	ctrl := nscapture.Control{Op: nscapture.OpAck, Namespace: "exp", Head: "head", ID: 3, Token: 9, Gen: 2, Bridge: "br",
		Seq: 4, Window: 8, HeadRecv: 5, HeadSend: 6}

	for _, body := range []interface{}{batch, ctrl} {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(&meshage.Message{Body: body}); err != nil {
			t.Fatalf("encode %T: %v", body, err)
		}

		var got meshage.Message
		if err := gob.NewDecoder(&buf).Decode(&got); err != nil {
			t.Fatalf("decode %T: %v", body, err)
		}
		if !reflect.DeepEqual(got.Body, body) {
			t.Errorf("round trip of %T:\n%+v\nwant\n%+v", body, got.Body, body)
		}
	}
}

// TestNSCaptureStopKeepsTail checks that a stop right after the members write
// records keeps all the records in the file.
func TestNSCaptureStopKeepsTail(t *testing.T) {
	useFakeEdgeBridges(t, nil)
	setTestHostname()

	hc := newTestHead(t, "nscapture-tail", 1, []string{hostname})
	m := newTestMember(hc)

	base := time.Now()
	for i, br := range []string{"br_a", "br_b"} {
		s := nscapture.NewStream(nscapture.StreamConfig{Namespace: hc.ns, Head: hostname, Host: hostname, Bridge: br,
			ID: hc.id, Token: hc.token}, nsSender{}, nil)
		m.smu.Lock()
		m.streams[br] = &memberStream{bridge: br, stream: s}
		m.smu.Unlock()

		for j := 0; j < 50; j++ {
			ts := base.Add(time.Duration(j*2+i) * time.Microsecond)
			s.WriteRecord(0, pcapfile.AppendRecord(nil, ts, 60, make([]byte, 60)), pcapfile.RecordOptions{})
		}
	}

	rows := nsCaptureStopHead(hc.ns, "1", 10*time.Second)
	if len(rows) == 0 || rows[0][3] != nscapture.StateStopped || rows[0][5] != "100" {
		t.Fatalf("final rows %q, want 100 packets", rows)
	}

	data, err := os.ReadFile(hc.path)
	if err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var n int
	for {
		if _, _, err := r.ReadPacketData(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n != 100 {
		t.Errorf("file has %d records, want 100", n)
	}
}

// TestNSCaptureStaleMemberStops checks that the member of a removed host
// stops when the stop to it is lost: the head answers its batches with an
// Unknown ack. The member of a newer start of the host ignores the Unknown ack
// for the older start.
func TestNSCaptureStaleMemberStops(t *testing.T) {
	useFakeEdgeBridges(t, nil)
	setTestHostname()

	ns := "nscapture-stalemember"
	hc := newTestHead(t, ns, 1, []string{hostname})
	old := newTestMember(hc)

	// ns del-hosts, but the stop does not reach the member
	hc.mu.Lock()
	hc.hosts = nil
	hc.mu.Unlock()
	hc.hostOp(hostOp{host: hostname})

	now := time.Now()
	hc.inbox <- headBatch(hc, 0, 1, now, now.Add(-time.Second))
	select {
	case <-old.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the member of the removed host did not stop")
	}

	add, gen := hc.addHosts([]string{hostname})
	if !slices.Equal(add, []string{hostname}) || gen != 1 {
		t.Fatalf("addHosts = %v %v, want this host and start 1", add, gen)
	}

	m := &memberCapture{
		key:      old.key,
		opts:     hc.opts,
		streams:  map[string]*memberStream{},
		headSeen: time.Now(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
	m.opts.gen = gen
	nsMembers.mu.Lock()
	nsMembers.m[m.key] = m
	nsMembers.mu.Unlock()

	// a late batch of the old member
	hc.inbox <- headBatch(hc, 0, 2, now, now.Add(-time.Second))
	time.Sleep(200 * time.Millisecond)
	select {
	case <-m.done:
		t.Fatal("the Unknown ack for start 0 stopped the member of start 1")
	default:
	}

	nsCaptureStopHead(ns, "1", 5*time.Second)
	select {
	case <-m.done:
	case <-time.After(5 * time.Second):
		t.Error("the member of start 1 did not stop")
	}
}
