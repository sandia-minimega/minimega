// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package bridge

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/pcap"
	"github.com/gopacket/gopacket/pcapgo"
)

// fakeSource gives the frames sent on ch, a timeout when none is ready, and
// err (if set) after ch is closed.
type fakeSource struct {
	ch     chan []byte
	err    error
	block  chan struct{} // when set, reads wait until it is closed
	mu     sync.Mutex
	closed bool
}

func newFakeSource() *fakeSource {
	return &fakeSource{ch: make(chan []byte, 1024)}
}

func (s *fakeSource) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	if s.block != nil {
		<-s.block
	}

	select {
	case data, ok := <-s.ch:
		if !ok {
			if s.err != nil {
				return nil, gopacket.CaptureInfo{}, s.err
			}
			time.Sleep(time.Millisecond)
			return nil, gopacket.CaptureInfo{}, errSourceTimeout
		}
		ci := gopacket.CaptureInfo{
			Timestamp:     time.Unix(1762351353, 123456789),
			CaptureLength: len(data),
			Length:        len(data) + 10,
		}
		return data, ci, nil
	case <-time.After(5 * time.Millisecond):
		return nil, gopacket.CaptureInfo{}, errSourceTimeout
	}
}

func (s *fakeSource) Stats() (*pcap.Stats, error) {
	return &pcap.Stats{PacketsDropped: 2, PacketsIfDropped: 1}, nil
}

func (s *fakeSource) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
}

func (s *fakeSource) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func testBridge() *Bridge {
	return &Bridge{
		Name:      "test_bridge",
		taps:      map[string]*Tap{"tap0": {Name: "tap0", Bridge: "test_bridge"}},
		trunks:    map[string]bool{},
		tunnels:   map[string]bool{},
		mirrors:   map[string]bool{},
		bonds:     map[string]map[string]int{},
		captures:  map[int]*capture{},
		captureID: new(int),
		config:    map[string]string{},
	}
}

// useFakeSources makes captures read from the returned sources, in order.
func useFakeSources(t *testing.T, srcs ...*fakeSource) {
	t.Helper()

	old := openCaptureSource
	t.Cleanup(func() { openCaptureSource = old })

	var mu sync.Mutex
	openCaptureSource = func(string, CaptureConfig) (packetSource, error) {
		mu.Lock()
		defer mu.Unlock()

		s := srcs[0]
		srcs = srcs[1:]
		return s, nil
	}
}

func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()

	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %v", what)
}

func readFrames(t *testing.T, path string) [][]byte {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if magic := binary.LittleEndian.Uint32(data); magic != 0xa1b23c4d {
		t.Fatalf("magic %#x, want nanosecond pcap", magic)
	}

	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var res [][]byte
	for {
		d, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			return res
		} else if err != nil {
			t.Fatal(err)
		}
		if ci.Timestamp.Nanosecond() != 123456789 {
			t.Errorf("timestamp %v lost nanoseconds", ci.Timestamp)
		}
		res = append(res, d)
	}
}

func TestCaptureTapStopFlushes(t *testing.T) {
	src := newFakeSource()
	useFakeSources(t, src)

	b := testBridge()
	path := filepath.Join(t.TempDir(), "sub", "dir", "a.pcap")

	id, err := b.CaptureTap("tap0", path, CaptureConfig{SnapLen: 100})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		src.ch <- bytes.Repeat([]byte{byte(i)}, 60)
	}

	waitFor(t, "packets", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.Packets == 10
	})

	info, ok := b.CaptureInfo(id)
	if !ok || info.State != CaptureRunning || info.Bytes != 700 || info.Config.SnapLen != 100 || info.Format != "pcap" {
		t.Errorf("unexpected info: %+v", info)
	}

	waitFor(t, "kernel drops", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.KernelDrops == 3
	})

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	if !src.isClosed() {
		t.Error("source not closed after stop")
	}

	if _, ok := b.CaptureInfo(id); ok {
		t.Error("capture still listed after stop")
	}

	if got := readFrames(t, path); len(got) != 10 {
		t.Errorf("file has %d frames, want 10", len(got))
	}

	if err := b.StopCapture(id); err == nil {
		t.Error("second stop did not fail")
	}
}

func TestCaptureIDsNotReused(t *testing.T) {
	useFakeSources(t, newFakeSource(), newFakeSource(), newFakeSource())

	b := testBridge()
	dir := t.TempDir()

	var ids []int
	for i, name := range []string{"a", "b", "c"} {
		id, err := b.CaptureTap("tap0", filepath.Join(dir, name), CaptureConfig{})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)

		if i == 0 {
			if err := b.StopCapture(id); err != nil {
				t.Fatal(err)
			}
		}
	}

	if ids[0] != 0 || ids[1] != 1 || ids[2] != 2 {
		t.Errorf("IDs %v, want 0 1 2", ids)
	}

	infos := b.CaptureInfos()
	if len(infos) != 2 || infos[0].ID != 1 || infos[1].ID != 2 {
		t.Errorf("CaptureInfos() = %+v", infos)
	}

	for _, id := range ids[1:] {
		b.StopCapture(id)
	}
}

func TestCaptureMaxSizeFull(t *testing.T) {
	src := newFakeSource()
	useFakeSources(t, src)

	b := testBridge()
	path := filepath.Join(t.TempDir(), "full.pcap")

	// header (24) + 2 records of 16+100 bytes
	id, err := b.CaptureTap("tap0", path, CaptureConfig{MaxSize: 24 + 2*116})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 5; i++ {
		src.ch <- make([]byte, 100)
	}

	waitFor(t, "full", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.State == CaptureFull
	})

	info, _ := b.CaptureInfo(id)
	if info.Packets != 2 || info.Size != 24+2*116 || info.Error == "" {
		t.Errorf("unexpected info: %+v", info)
	}

	if fi, err := os.Stat(path); err != nil || fi.Size() != 24+2*116 {
		t.Errorf("file size: %v %v", fi, err)
	}

	// the path is free again for a new capture
	waitFor(t, "path release", func() bool {
		release, err := ClaimCapturePath(path)
		if err == nil {
			release()
		}
		return err == nil
	})

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureReadErrorFails(t *testing.T) {
	src := newFakeSource()
	src.err = errors.New("The interface went down")
	useFakeSources(t, src)

	b := testBridge()

	id, err := b.CaptureTap("tap0", filepath.Join(t.TempDir(), "e.pcap"), CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}

	close(src.ch)

	waitFor(t, "failed", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.State == CaptureFailed
	})

	info, _ := b.CaptureInfo(id)
	if !strings.Contains(info.Error, "went down") {
		t.Errorf("error %q", info.Error)
	}

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureTapRemovedFails(t *testing.T) {
	useFakeSources(t, newFakeSource())

	b := testBridge()

	id, err := b.CaptureTap("tap0", filepath.Join(t.TempDir(), "t.pcap"), CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}

	bridgeLock.Lock()
	b.failTapCaptures("tap0")
	bridgeLock.Unlock()

	waitFor(t, "failed", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.State == CaptureFailed && info.Error == "tap tap0 removed"
	})

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}
}

func TestCapturePathInUse(t *testing.T) {
	useFakeSources(t, newFakeSource(), newFakeSource(), newFakeSource(), newFakeSource())

	b := testBridge()
	path := filepath.Join(t.TempDir(), "same.pcap")

	id, err := b.CaptureTap("tap0", path, CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := b.CaptureTap("tap0", path, CaptureConfig{}); err == nil || !strings.Contains(err.Error(), "in use") {
		t.Errorf("second capture to the same file: %v", err)
	}

	// device files can be used more than once
	id2, err := b.CaptureTap("tap0", os.DevNull, CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}
	id3, err := b.CaptureTap("tap0", os.DevNull, CaptureConfig{})
	if err != nil {
		t.Errorf("second capture to %v: %v", os.DevNull, err)
	}

	for _, v := range []int{id, id2, id3} {
		b.StopCapture(v)
	}
}

func TestStopCaptureReleasesLock(t *testing.T) {
	src := newFakeSource()
	src.block = make(chan struct{})
	useFakeSources(t, src)

	b := testBridge()

	id, err := b.CaptureTap("tap0", filepath.Join(t.TempDir(), "l.pcap"), CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		b.StopCapture(id)
		close(done)
	}()

	// the reader is blocked, so the stop waits; bridgeLock must be free
	waitFor(t, "capture removed", func() bool {
		if !bridgeLock.TryLock() {
			return false
		}
		_, ok := b.captures[id]
		bridgeLock.Unlock()
		return !ok
	})

	select {
	case <-done:
		t.Fatal("stop returned before the reader ended")
	default:
	}

	close(src.block)
	<-done
}

func TestCaptureConfigValidate(t *testing.T) {
	tests := []struct {
		c  CaptureConfig
		ok bool
	}{
		{CaptureConfig{}, true},
		{CaptureConfig{SnapLen: 14, Filter: "tcp port 80", Direction: "in"}, true},
		{CaptureConfig{SnapLen: 262144, Direction: "both"}, true},
		{CaptureConfig{SnapLen: 13}, false},
		{CaptureConfig{SnapLen: 262145}, false},
		{CaptureConfig{Filter: "tcp port eighty"}, false},
		{CaptureConfig{Direction: "sideways"}, false},
		{CaptureConfig{MaxSize: -1}, false},
	}

	for _, tc := range tests {
		if err := tc.c.Validate(); (err == nil) != tc.ok {
			t.Errorf("Validate(%+v) = %v, want ok=%v", tc.c, err, tc.ok)
		}
	}
}

func TestParseSize(t *testing.T) {
	tests := map[string]int64{
		"0":      0,
		"1000":   1000,
		"2K":     2048,
		"10240M": 10240 << 20,
		"1g":     1 << 30,
	}

	for in, want := range tests {
		if got, err := ParseSize(in); err != nil || got != want {
			t.Errorf("ParseSize(%q) = %v, %v; want %v", in, got, err, want)
		}
	}

	for _, in := range []string{"", "M", "-1", "1T", "1.5G", "99999999999G"} {
		if _, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) did not fail", in)
		}
	}
}

// fakeCommands records the ovs-vsctl and other commands that the bridge runs.
type fakeCommands struct {
	mu   sync.Mutex
	cmds []string
}

func useFakeCommands(t *testing.T) *fakeCommands {
	t.Helper()

	f := &fakeCommands{}
	oldOVS, oldProc := ovsExec, processExec
	t.Cleanup(func() { ovsExec, processExec = oldOVS, oldProc })

	ovsExec = func(args []string) (string, error) {
		f.add("ovs-vsctl " + strings.Join(args, " "))
		return "", nil
	}
	processExec = func(args []string) (string, error) {
		f.add(strings.Join(args, " "))
		return "", nil
	}

	return f
}

func (f *fakeCommands) add(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cmds = append(f.cmds, s)
}

func (f *fakeCommands) has(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, v := range f.cmds {
		if strings.Contains(v, sub) {
			return true
		}
	}

	return false
}

func testTapChan() chan string {
	ch := make(chan string, 4)
	for _, v := range []string{"mega_tap42", "mega_tap43", "mega_tap44", "mega_tap45"} {
		ch <- v
	}
	return ch
}

func TestBridgeCaptureRemovesTap(t *testing.T) {
	cmds := useFakeCommands(t)
	useFakeSources(t, newFakeSource())

	b := testBridge()
	b.tapChan = testTapChan()

	var got CaptureConfig
	old := openCaptureSource
	openCaptureSource = func(dev string, c CaptureConfig) (packetSource, error) {
		got = c
		return old(dev, c)
	}

	id, err := b.Capture(filepath.Join(t.TempDir(), "br.pcap"), CaptureConfig{})
	if err != nil {
		t.Fatal(err)
	}

	if got.Direction != DirectionIn {
		t.Errorf("bridge capture direction %q, want in", got.Direction)
	}

	for _, want := range []string{
		"ovs-vsctl add-port test_bridge mega_tap42 -- set Interface mega_tap42 type=internal",
		"ip link set mega_tap42 up promisc on",
		"name=mirror-mega_tap42",
		"select-all=true",
	} {
		if !cmds.has(want) {
			t.Errorf("missing command %q in %q", want, cmds.cmds)
		}
	}

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"get mirror mirror-mega_tap42 -- remove bridge test_bridge mirrors @m",
		"del-port test_bridge mega_tap42",
	} {
		if !cmds.has(want) {
			t.Errorf("missing command %q after stop in %q", want, cmds.cmds)
		}
	}

	if len(b.taps) != 1 || len(b.mirrors) != 0 {
		t.Errorf("taps %v, mirrors %v left after stop", b.taps, b.mirrors)
	}
}

func TestDestroyRemovesMirrors(t *testing.T) {
	cmds := useFakeCommands(t)
	useFakeSources(t, newFakeSource())

	b := testBridge()
	b.preExist = true
	b.tapChan = testTapChan()
	b.mirrors["tap0"] = true // a tap mirror

	path := filepath.Join(t.TempDir(), "d.pcap")
	if _, err := b.Capture(path, CaptureConfig{}); err != nil {
		t.Fatal(err)
	}

	bridgeLock.Lock()
	err := b.destroy()
	bridgeLock.Unlock()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"get mirror mirror-mega_tap42", "get mirror mirror-tap0"} {
		if !cmds.has(want) {
			t.Errorf("missing command %q in %q", want, cmds.cmds)
		}
	}

	if len(b.captures) != 0 || len(b.mirrors) != 0 {
		t.Errorf("captures %v, mirrors %v left after destroy", b.captures, b.mirrors)
	}

	if cmds.has("del-br") {
		t.Error("deleted a pre-existing bridge")
	}

	if got := readFrames(t, path); len(got) != 0 {
		t.Errorf("file has %d frames", len(got))
	}
}

func TestCapturePcapng(t *testing.T) {
	src := newFakeSource()
	useFakeSources(t, src)

	b := testBridge()
	path := filepath.Join(t.TempDir(), "a.pcapng")

	id, err := b.CaptureTap("tap0", path, CaptureConfig{
		Format:      "pcapng",
		Direction:   DirectionIn,
		Filter:      "tcp",
		Description: "web01 interface 0, VLAN EXP_1 (101)",
		Names:       []pcapfile.Name{{Addr: "10.0.0.7", Names: []string{"web01"}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		src.ch <- make([]byte, 60)
	}

	waitFor(t, "packets", func() bool {
		info, _ := b.CaptureInfo(id)
		return info.Packets == 3 && info.Format == "pcapng"
	})

	if err := b.StopCapture(id); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r, err := pcapgo.NewNgReader(f, pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}

	var n int
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if ci.Timestamp.Nanosecond() != 123456789 {
			t.Errorf("timestamp %v", ci.Timestamp)
		}
		n++
	}

	if n != 3 {
		t.Errorf("read %d frames, want 3", n)
	}

	intf, err := r.Interface(0)
	if err != nil {
		t.Fatal(err)
	}
	if intf.Name != "tap0" || intf.Description != "web01 interface 0, VLAN EXP_1 (101)" || intf.Filter != "tcp" {
		t.Errorf("interface %+v", intf)
	}
}
