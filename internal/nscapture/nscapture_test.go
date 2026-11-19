// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package nscapture

import (
	"bytes"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"

	"github.com/gopacket/gopacket/pcapgo"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1762351353, 0)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// frame returns a 60-byte frame with source MAC src and payload byte v.
func frame(src net.HardwareAddr, v byte) []byte {
	f := make([]byte, 60)
	copy(f[6:], src)
	f[59] = v
	return f
}

func rec(ts time.Time, data []byte) []byte {
	return pcapfile.AppendRecord(nil, ts, len(data), data)
}

// batch builds a batch of classic records at the given times.
func batch(host, bridge string, seq uint64, through time.Time, times ...time.Time) *Batch {
	b := &Batch{Host: host, Bridge: bridge, Seq: seq, Through: through.UnixNano(), Format: FormatClassic, Token: 7}
	for i, ts := range times {
		b.Data = append(b.Data, rec(ts, frame(net.HardwareAddr{0, 0, 0, 0, 0, byte(i)}, byte(seq)))...)
		b.Packets++
		b.Captured++
	}
	return b
}

func readTimes(t *testing.T, data []byte) []int64 {
	t.Helper()

	r, err := pcapgo.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}

	var res []int64
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			return res
		} else if err != nil {
			t.Fatal(err)
		}
		res = append(res, ci.Timestamp.UnixNano())
	}
}

func TestSplitRecords(t *testing.T) {
	now := time.Unix(100, 5)
	good := append(rec(now, make([]byte, 60)), rec(now, make([]byte, 70))...)

	if r, err := splitRecords(good, FormatClassic, 2, 1600); err != nil || len(r) != 2 || r[0].ts != now.UnixNano() {
		t.Errorf("good batch: %v %v", r, err)
	}

	ng := appendNgRecord(nil, rec(now, make([]byte, 60)), 3, 2)
	if r, err := splitRecords(ng, FormatNg, 1, 1600); err != nil || r[0].port != 3 || r[0].dropsB4 != 2 || len(r[0].rec) != 76 {
		t.Errorf("ng batch: %v %v", r, err)
	}

	for name, tc := range map[string]struct {
		data    []byte
		packets uint32
		snaplen uint32
	}{
		"count":   {good, 3, 1600},
		"snaplen": {good, 2, 64},
		"short":   {good[:10], 1, 1600},
		"past":    {good[:len(good)-1], 2, 1600},
	} {
		if _, err := splitRecords(tc.data, FormatClassic, tc.packets, tc.snaplen); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func FuzzSplitRecords(f *testing.F) {
	now := time.Unix(100, 5)
	f.Add(rec(now, make([]byte, 60)), uint8(1), uint32(1))
	f.Add(appendNgRecord(nil, rec(now, make([]byte, 60)), 1, 0), uint8(2), uint32(1))

	f.Fuzz(func(t *testing.T, data []byte, format uint8, packets uint32) {
		recs, err := splitRecords(data, format, packets, 1600)
		if err != nil {
			return
		}
		for _, r := range recs {
			if _, err := pcapfile.ParseRecordHeader(r.rec); err != nil {
				t.Fatal(err)
			}
		}
	})
}

func newTestMerger(t *testing.T, clock Clock, maxSize int64) (*Merger, *bytes.Buffer) {
	t.Helper()

	var buf bytes.Buffer
	w, err := pcapfile.NewClassic(&buf, 1600)
	if err != nil {
		t.Fatal(err)
	}

	return NewMerger(MergerConfig{Namespace: "exp", Token: 7, SnapLen: 1600, MaxSize: maxSize}, w, clock), &buf
}

func TestMergerOrder(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	t0 := clock.Now().Add(-time.Second)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }

	// c2 batch 2 comes before batch 1; c1 sends a duplicate and a heartbeat
	m.Add(batch("c1", "br", 1, at(30), at(10), at(30)))
	m.Add(batch("c2", "br", 2, at(60), at(50), at(60)))
	m.Add(batch("c2", "br", 1, at(40), at(20), at(40)))
	m.Add(batch("c1", "br", 1, at(30), at(10), at(30)))

	// the watermark is the oldest through of the live streams: c1 at 30
	m.Tick()
	if got := readTimes(t, buf.Bytes()); len(got) != 0 {
		t.Fatalf("wrote before flush: %v", got)
	}
	m.w.Flush()
	if got := readTimes(t, buf.Bytes()); len(got) != 3 {
		t.Fatalf("wrote %d records up to the watermark, want 3", len(got))
	}

	m.Add(batch("c1", "br", 2, at(70)))
	m.Tick()

	// a record older than the last written one is late and still written
	m.Add(batch("c1", "br", 3, at(80), at(5)))

	streams, sum := m.Status()
	if len(streams) != 2 || sum.Dup != 1 || sum.Late != 1 {
		t.Errorf("status %+v %+v", streams, sum)
	}

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	m.Add(&Batch{Host: "c2", Final: true, Token: 7})
	if !m.Done([]string{"c1", "c2"}) {
		t.Error("not done after the final batches")
	}

	if err := m.Finish([]string{"c1", "c2"}); err != nil {
		t.Fatal(err)
	}

	got := readTimes(t, buf.Bytes())
	want := []int64{at(10).UnixNano(), at(20).UnixNano(), at(30).UnixNano(), at(40).UnixNano(),
		at(50).UnixNano(), at(60).UnixNano(), at(5).UnixNano()}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d at %v, want %v", i, got[i], want[i])
		}
	}
}

func TestMergerTokenAndStall(t *testing.T) {
	clock := newFakeClock()
	m, _ := newTestMerger(t, clock, 0)

	b := batch("c1", "br", 1, clock.Now())
	b.Token = 8
	if _, ok := m.Add(b); ok {
		t.Error("batch with an unknown token accepted")
	}

	m.Add(batch("c1", "br", 1, clock.Now()))
	m.Add(batch("c2", "br", 1, clock.Now().Add(-500*time.Millisecond)))

	clock.Advance(StallTimeout)
	m.Add(batch("c1", "br", 2, clock.Now()))

	streams, _ := m.Status()
	if streams[0].State != StateRunning || streams[1].State != StateStalled {
		t.Errorf("states %v %v", streams[0].State, streams[1].State)
	}

	// the stalled stream does not hold back the watermark
	if w := m.Watermark(); w != clock.Now().UnixNano() {
		t.Errorf("watermark %v, want %v", w, clock.Now().UnixNano())
	}
}

func TestMergerGapAndWindow(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	now := clock.Now()
	m.Add(batch("c1", "br", 1, now.Add(-2*time.Second), now.Add(-3*time.Second)))
	m.Add(batch("c1", "br", 3, now, now.Add(-time.Millisecond)))

	// a slow stream does not hold back the others for more than the window
	m.Add(batch("c2", "br", 1, now.Add(-10*time.Second), now.Add(-10*time.Second)))
	m.Tick()
	m.w.Flush()
	if got := readTimes(t, buf.Bytes()); len(got) != 2 {
		t.Errorf("wrote %d records, want 2 within the window", len(got))
	}

	clock.Advance(DefaultWindow) // more than the gap timeout
	m.Tick()
	m.w.Flush()

	if got := readTimes(t, buf.Bytes()); len(got) != 3 {
		t.Errorf("wrote %d records after the gap timeout, want 3", len(got))
	}

	if m.streams["c1/br"].lostBatches != 1 {
		t.Errorf("lost batches %v, want 1", m.streams["c1/br"].lostBatches)
	}
}

// TestMergerGapInOrder checks that the batches after a lost batch are written
// in order and not late.
func TestMergerGapInOrder(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	for seq := uint64(1); seq <= 150; seq++ {
		clock.Advance(50 * time.Millisecond)
		now := clock.Now()
		for _, host := range []string{"c1", "c2"} {
			if host == "c1" && seq == 3 {
				continue // lost
			}
			m.Add(batch(host, "br", seq, now.Add(-5*time.Millisecond), now.Add(-20*time.Millisecond), now.Add(-10*time.Millisecond)))
		}
		m.Tick()
	}

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	m.Add(&Batch{Host: "c2", Final: true, Token: 7})
	if err := m.Finish([]string{"c1", "c2"}); err != nil {
		t.Fatal(err)
	}

	if _, sum := m.Status(); sum.Late != 0 {
		t.Errorf("%d late records, want 0", sum.Late)
	}

	got := readTimes(t, buf.Bytes())
	if len(got) != 2*(2*150-1) {
		t.Errorf("file has %d records, want %d", len(got), 2*(2*150-1))
	}
	if !slices.IsSorted(got) {
		t.Error("records out of order")
	}
}

// TestMergerSkew checks that the records of a host whose clock lags more than
// the window are in order on the clock of the head, and not late. The file
// keeps the timestamps of the host.
func TestMergerSkew(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	const skew = 3 * time.Second
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	t0 := clock.Now()
	at := func(ms int) time.Time { return t0.Add(-time.Duration(ms) * time.Millisecond) }

	// add adds a batch with a record for each time in ms before t0, on the
	// head clock. The payload of a record is its time in tens of ms.
	add := func(host string, seq uint64, through int, times ...int) {
		var hostSkew time.Duration
		if host == "c2" {
			hostSkew = skew
		}
		b := &Batch{Host: host, Bridge: "br", Seq: seq, Through: at(through).Add(-hostSkew).UnixNano(),
			Format: FormatClassic, Token: 7, SkewNS: int64(hostSkew)}
		for _, ms := range times {
			b.Data = append(b.Data, rec(at(ms).Add(-hostSkew), frame(mac, byte(ms/10)))...)
			b.Packets++
		}
		m.Add(b)
	}

	add("c1", 1, 750, 900, 800)
	add("c2", 1, 700, 850, 750)
	m.Tick()

	clock.Advance(200 * time.Millisecond)
	add("c1", 2, 550, 600)
	add("c2", 2, 500, 650)
	m.Tick()

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	m.Add(&Batch{Host: "c2", Final: true, Token: 7})
	if err := m.Finish([]string{"c1", "c2"}); err != nil {
		t.Fatal(err)
	}

	if _, sum := m.Status(); sum.Late != 0 {
		t.Errorf("%d late records, want 0", sum.Late)
	}

	r, err := pcapgo.NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	var order []int
	var c2 int64
	for {
		data, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		order = append(order, int(data[59]))
		if data[59] == 85 {
			c2 = ci.Timestamp.UnixNano()
		}
	}

	if want := []int{90, 85, 80, 75, 65, 60}; !slices.Equal(order, want) {
		t.Errorf("records in order %v, want %v (tens of ms before the start)", order, want)
	}
	if want := at(850).Add(-skew).UnixNano(); c2 != want {
		t.Errorf("record of c2 at %v, want the host time %v", c2, want)
	}
}

// TestMergerMaxSize checks that a classic merge stays within maxsize: two of
// the three records fit.
func TestMergerMaxSize(t *testing.T) {
	clock := newFakeClock()
	const maxSize = 24 + 2*(16+60) + 64
	m, buf := newTestMerger(t, clock, maxSize)

	now := clock.Now()
	m.Add(batch("c1", "br", 1, now, now.Add(-3*time.Second), now.Add(-2*time.Second), now.Add(-time.Second)))
	clock.Advance(2 * time.Second)
	m.Tick()

	if !m.Full() {
		t.Error("not full")
	}

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	if buf.Len() > maxSize {
		t.Errorf("file size = %d, more than maxsize %d", buf.Len(), maxSize)
	}
	if got := readTimes(t, buf.Bytes()); len(got) != 2 {
		t.Errorf("file has %d records, want 2", len(got))
	}
}

func TestMergerPcapng(t *testing.T) {
	clock := newFakeClock()

	var buf bytes.Buffer
	w, err := pcapfile.NewNg(&buf, pcapfile.Section{UserAppl: "minimega test"})
	if err != nil {
		t.Fatal(err)
	}

	m := NewMerger(MergerConfig{Namespace: "exp", Token: 7, SnapLen: 1600, Names: true}, w, clock)

	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	now := clock.Now().Add(-time.Second)

	b := &Batch{Host: "c1", Bridge: "br", Seq: 1, Through: now.UnixNano(), Format: FormatNg, Token: 7, Packets: 2,
		PortTable: []PortInfo{{Index: 1, Port: "mega_tap3", MAC: mac.String(), VLAN: 101, VM: "web01", VMIface: 2, Addrs: []string{"10.0.0.7"}}}}
	b.Data = appendNgRecord(b.Data, rec(now, frame(mac, 1)), 1, 0)
	b.Data = appendNgRecord(b.Data, rec(now, frame(net.HardwareAddr{9, 9, 9, 9, 9, 9}, 2)), 0, 4)
	m.Add(b)
	m.Add(&Batch{Host: "c1", Final: true, Token: 7})

	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewNgReader(bytes.NewReader(buf.Bytes()), pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}

	var ifaces []int
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		ifaces = append(ifaces, ci.InterfaceIndex)
	}

	if len(ifaces) != 2 || ifaces[0] == ifaces[1] {
		t.Fatalf("interfaces %v, want two different", ifaces)
	}

	i0, _ := r.Interface(ifaces[0])
	i1, _ := r.Interface(ifaces[1])
	if i0.Name != "c1/br/mega_tap3" || i1.Name != "c1/br" {
		t.Errorf("interface names %q %q", i0.Name, i1.Name)
	}
	if want := "web01 interface 2, VLAN 101"; i0.Description != want {
		t.Errorf("interface description %q, want %q", i0.Description, want)
	}
}

// fakeSender records the batches it gets, and fails while fail is set.
type fakeSender struct {
	mu      sync.Mutex
	batches []Batch
	fail    bool
}

func (f *fakeSender) Send(host string, body interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.fail {
		return errors.New("no route")
	}
	f.batches = append(f.batches, body.(Batch))
	return nil
}

func (f *fakeSender) get() []Batch {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Batch(nil), f.batches...)
}

func waitFor(t *testing.T, what string, fn func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if fn() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %v", what)
}

func TestStreamBatches(t *testing.T) {
	clock := newFakeClock()
	sender := &fakeSender{}
	s := NewStream(StreamConfig{Namespace: "exp", Head: "head", Host: "c1", Bridge: "br", ID: 1, Token: 7}, sender, clock)

	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	// a young batch waits; an old one goes
	s.WriteRecord(0, rec(clock.Now(), frame(mac, 1)), pcapfile.RecordOptions{})
	s.Flush()
	clock.Advance(BatchAge)
	s.Flush()

	waitFor(t, "first batch", func() bool { return len(sender.get()) == 1 })

	b := sender.get()[0]
	if b.Seq != 1 || b.Packets != 1 || b.Captured != 1 || b.Token != 7 {
		t.Errorf("first batch %+v", b)
	}

	// an idle stream sends heartbeats
	clock.Advance(HeartbeatPeriod)
	s.Flush()
	waitFor(t, "heartbeat", func() bool { return len(sender.get()) == 2 })
	if hb := sender.get()[1]; hb.Packets != 0 || hb.Seq != 2 || hb.Through <= b.Through {
		t.Errorf("heartbeat %+v", hb)
	}

	// a full batch goes at once
	big := make([]byte, 1500)
	copy(big[6:], mac)
	for i := 0; i < BatchTarget/1516+1; i++ {
		s.WriteRecord(0, rec(clock.Now(), big), pcapfile.RecordOptions{})
	}
	waitFor(t, "size flush", func() bool { return len(sender.get()) == 3 })

	s.Fail("bridge destroyed")
	s.Close()

	got := sender.get()
	last := got[len(got)-1]
	if !last.Final || last.Err != "bridge destroyed" {
		t.Errorf("final batch %+v", last)
	}
}

func TestStreamWindowAndAck(t *testing.T) {
	clock := newFakeClock()
	sender := &fakeSender{}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7}, sender, clock)

	for i := 0; i < 12; i++ {
		clock.Advance(HeartbeatPeriod)
		s.Flush()
	}

	waitFor(t, "window", func() bool { return len(sender.get()) == DefaultAckLimit })
	time.Sleep(20 * time.Millisecond)
	if n := len(sender.get()); n != DefaultAckLimit {
		t.Fatalf("sent %d batches without acks, want %d", n, DefaultAckLimit)
	}

	s.Ack(Control{Op: OpAck, Seq: 4, Window: DefaultAckLimit})
	waitFor(t, "more after ack", func() bool { return len(sender.get()) == 12 })

	// an ack stall lets the stream go on
	for i := 0; i < 10; i++ {
		clock.Advance(HeartbeatPeriod)
		s.Flush()
	}
	clock.Advance(AckStall)
	waitFor(t, "after stall", func() bool { return len(sender.get()) == 20 })
	clock.Advance(AckStall)
	waitFor(t, "after second stall", func() bool { return len(sender.get()) == 22 })

	unknown := make(chan struct{})
	s.OnUnknown = func() { close(unknown) }
	s.Ack(Control{Op: OpAck, Unknown: true})
	<-unknown

	s.Close()
}

func TestStreamRetrySameSeq(t *testing.T) {
	old := sendRetryLimit
	sendRetryLimit = 300 * time.Millisecond
	t.Cleanup(func() { sendRetryLimit = old })

	clock := newFakeClock()
	sender := &fakeSender{fail: true}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7}, sender, clock)

	s.WriteRecord(0, rec(clock.Now(), make([]byte, 60)), pcapfile.RecordOptions{})
	clock.Advance(BatchAge)
	s.Flush()

	// fails until the retry limit: the batch is dropped and counted
	waitFor(t, "send drop", func() bool {
		_, _, drops := s.Counters()
		return drops == 1
	})

	sender.mu.Lock()
	sender.fail = false
	sender.mu.Unlock()

	clock.Advance(HeartbeatPeriod)
	s.Flush()
	waitFor(t, "next batch", func() bool { return len(sender.get()) == 1 })

	if b := sender.get()[0]; b.Seq != 2 || b.SendDrops != 1 {
		t.Errorf("batch after drop %+v", b)
	}

	s.Close()
}

// TestStreamSharedMAC checks that the packets of a MAC that two ports have
// use the stream interface (index 0).
func TestStreamSharedMAC(t *testing.T) {
	clock := newFakeClock()
	sender := &fakeSender{}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7, Format: FormatNg}, sender, clock)

	shared := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	other := net.HardwareAddr{2, 0, 0, 0, 0, 2}
	s.SetPorts([]PortInfo{
		{Index: 1, Port: "mega_tap1", MAC: shared.String(), VLAN: 101},
		{Index: 2, Port: "mega_tap2", MAC: other.String(), VLAN: 101},
		{Index: 3, Port: "mega_tap3", MAC: shared.String(), VLAN: 102},
	})

	s.WriteRecord(0, rec(clock.Now(), frame(shared, 1)), pcapfile.RecordOptions{})
	s.WriteRecord(0, rec(clock.Now(), frame(other, 2)), pcapfile.RecordOptions{})
	clock.Advance(BatchAge)
	s.Flush()
	waitFor(t, "batch", func() bool { return len(sender.get()) == 1 })
	s.Close()

	b := sender.get()[0]
	recs, err := splitRecords(b.Data, b.Format, b.Packets, 0)
	if err != nil {
		t.Fatal(err)
	}

	var ports []uint32
	for _, r := range recs {
		ports = append(ports, r.port)
	}
	if !slices.Equal(ports, []uint32{0, 2}) {
		t.Errorf("ports %v, want [0 2]", ports)
	}
}

// TestStreamResendsPortTable checks that the port table goes in the next
// batch when the batch that has it is dropped.
func TestStreamResendsPortTable(t *testing.T) {
	old := sendRetryLimit
	sendRetryLimit = 300 * time.Millisecond
	t.Cleanup(func() { sendRetryLimit = old })

	clock := newFakeClock()
	sender := &fakeSender{fail: true}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7, Format: FormatNg}, sender, clock)

	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	s.SetPorts([]PortInfo{{Index: 1, Port: "mega_tap1", MAC: mac.String()}})

	s.WriteRecord(0, rec(clock.Now(), frame(mac, 1)), pcapfile.RecordOptions{})
	clock.Advance(BatchAge)
	s.Flush()
	waitFor(t, "send drop", func() bool {
		_, _, drops := s.Counters()
		return drops == 1
	})

	sender.mu.Lock()
	sender.fail = false
	sender.mu.Unlock()

	s.WriteRecord(0, rec(clock.Now(), frame(mac, 2)), pcapfile.RecordOptions{})
	clock.Advance(BatchAge)
	s.Flush()
	waitFor(t, "next batch", func() bool { return len(sender.get()) == 1 })
	s.Close()

	if b := sender.get()[0]; len(b.PortTable) != 1 || b.PortTable[0].Port != "mega_tap1" {
		t.Errorf("port table after a drop %+v", b.PortTable)
	}
}

func TestStreamQueueDrops(t *testing.T) {
	clock := newFakeClock()
	sender := &fakeSender{fail: true}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7, Format: FormatNg}, sender, clock)

	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	s.SetPorts([]PortInfo{{Index: 1, Port: "mega_tap1", MAC: mac.String()}})

	big := make([]byte, 60000)
	copy(big[6:], mac)
	for i := 0; i < QueueCap/60000+600; i++ {
		s.WriteRecord(0, rec(clock.Now(), big), pcapfile.RecordOptions{})
	}

	captured, qdrops, _ := s.Counters()
	if qdrops == 0 || captured != uint64(QueueCap/60000+600) {
		t.Errorf("captured %v, queue drops %v", captured, qdrops)
	}

	s.mu.Lock()
	s.unknown = true
	s.cond.Broadcast()
	s.mu.Unlock()
	s.Close()
}

// TestMergerMaxSizePcapng checks that a pcapng merge stays within maxsize,
// with its interface blocks and the statistics block at the end, for each
// maxsize in a range.
func TestMergerMaxSizePcapng(t *testing.T) {
	for extra := int64(0); extra < 1200; extra++ {
		clock := newFakeClock()

		var buf bytes.Buffer
		w, err := pcapfile.NewNg(&buf, pcapfile.Section{UserAppl: "minimega test"})
		if err != nil {
			t.Fatal(err)
		}

		maxSize := w.Size() + extra
		m := NewMerger(MergerConfig{Namespace: "exp", Token: 7, SnapLen: 1600, MaxSize: maxSize}, w, clock)

		now := clock.Now()
		var times []time.Time
		for i := 10; i > 0; i-- {
			times = append(times, now.Add(-time.Duration(i)*time.Second))
		}

		m.Add(batch("c1", "br", 1, now, times...))
		m.Add(&Batch{Host: "c1", Final: true, Token: 7})

		if err := m.Finish([]string{"c1"}); err != nil {
			t.Fatal(err)
		}

		if int64(buf.Len()) > maxSize {
			t.Fatalf("file size = %d, more than maxsize %d", buf.Len(), maxSize)
		}
		if extra < 500 && !m.Full() {
			t.Fatalf("not full with maxsize %d", maxSize)
		}
	}
}

// TestStreamCloseDrainTimeout checks that Close reports a queue that it could
// not send, also to a second caller.
func TestStreamCloseDrainTimeout(t *testing.T) {
	oldRetry, oldDrain := sendRetryLimit, drainLimit
	sendRetryLimit, drainLimit = 300*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { sendRetryLimit, drainLimit = oldRetry, oldDrain })

	clock := newFakeClock()
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7}, &fakeSender{fail: true}, clock)

	s.WriteRecord(0, rec(clock.Now(), make([]byte, 60)), pcapfile.RecordOptions{})

	err := s.Close()
	if err == nil {
		t.Fatal("Close() = nil with a queue that the sender cannot send")
	}
	if err2 := s.Close(); err2 != err {
		t.Errorf("second Close() = %v, want %v", err2, err)
	}
	if seq := s.FinalSeq(); seq != 1 {
		t.Errorf("FinalSeq() = %v, want 1", seq)
	}

	<-s.Done()
}

// TestMergerWaitsForStreamFinal checks that the head waits for the batches up
// to the final batch of each stream that the host-level final batch gives, and
// marks a stream failed when they do not arrive.
func TestMergerWaitsForStreamFinal(t *testing.T) {
	clock := newFakeClock()
	m, _ := newTestMerger(t, clock, 0)

	now := clock.Now()
	m.Add(batch("c1", "br", 1, now, now.Add(-time.Second)))
	m.Add(&Batch{Host: "c1", Final: true, Token: 7, Finals: map[string]uint64{"br": 3}})

	if m.Done([]string{"c1"}) {
		t.Fatal("Done() before the final batch of the stream")
	}

	b := batch("c1", "br", 3, now)
	b.Final = true
	m.Add(b)

	if m.Done([]string{"c1"}) {
		t.Fatal("Done() with batch 2 missing")
	}

	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	streams, _ := m.Status()
	if len(streams) != 1 || !strings.Contains(streams[0].Error, "batches 2 to 3 did not arrive") {
		t.Errorf("streams %+v, want an error for batches 2 to 3", streams)
	}

	// with every batch, the merge is done
	m, _ = newTestMerger(t, clock, 0)
	m.Add(batch("c1", "br", 1, now))
	b = batch("c1", "br", 2, now)
	b.Final = true
	m.Add(b)
	m.Add(&Batch{Host: "c1", Final: true, Token: 7, Finals: map[string]uint64{"br": 2}})

	if !m.Done([]string{"c1"}) {
		t.Error("not Done() with every batch")
	}
}

// TestMergerSkewThrough checks that the through time of a host with a clock
// ahead of the head is on the head clock: the head does not write the records
// of the other hosts before the records of that host.
func TestMergerSkewThrough(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	const ahead = 3 * time.Second
	now := clock.Now()
	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}

	// c2 batch with records at head times, on its clock that is ahead
	c2 := func(seq uint64, through time.Time, times ...time.Time) *Batch {
		b := &Batch{Host: "c2", Bridge: "br", Seq: seq, Through: through.Add(ahead).UnixNano(), Format: FormatClassic,
			Token: 7, SkewNS: -int64(ahead)}
		for _, ts := range times {
			b.Data = append(b.Data, rec(ts.Add(ahead), frame(mac, 2))...)
			b.Packets++
		}
		return b
	}

	m.Add(batch("c1", "br", 1, now, now.Add(-100*time.Millisecond)))
	m.Add(c2(1, now.Add(-500*time.Millisecond)))
	m.Tick()

	m.Add(c2(2, now.Add(-200*time.Millisecond), now.Add(-300*time.Millisecond)))
	m.Tick()

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	m.Add(&Batch{Host: "c2", Final: true, Token: 7})
	if err := m.Finish([]string{"c1", "c2"}); err != nil {
		t.Fatal(err)
	}

	if _, sum := m.Status(); sum.Late != 0 {
		t.Errorf("%d late records, want 0", sum.Late)
	}

	want := []int64{now.Add(-300 * time.Millisecond).Add(ahead).UnixNano(), now.Add(-100 * time.Millisecond).UnixNano()}
	if got := readTimes(t, buf.Bytes()); !slices.Equal(got, want) {
		t.Errorf("record times %v, want %v (c2, then c1)", got, want)
	}
}

// TestMergerSkewSmaller checks that a smaller skew estimate does not put the
// records of a stream out of order.
func TestMergerSkewSmaller(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	now := clock.Now()
	low := now.Add(-5 * time.Second)

	b1 := batch("c1", "br", 1, low, now.Add(-500*time.Millisecond))
	b2 := batch("c1", "br", 2, low, now.Add(-400*time.Millisecond))
	b2.SkewNS = -int64(200 * time.Millisecond)
	m.Add(b1)
	m.Add(b2)

	m.Add(&Batch{Host: "c1", Final: true, Token: 7})
	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	want := []int64{now.Add(-500 * time.Millisecond).UnixNano(), now.Add(-400 * time.Millisecond).UnixNano()}
	if got := readTimes(t, buf.Bytes()); !slices.Equal(got, want) {
		t.Errorf("record times %v, want %v", got, want)
	}
}

// TestStreamAckNegativeRTT checks that an ack with a negative round trip, for
// example from a head clock step, does not change the skew.
func TestStreamAckNegativeRTT(t *testing.T) {
	clock := newFakeClock()
	sender := &fakeSender{}
	s := NewStream(StreamConfig{Head: "head", Host: "c1", Bridge: "br", Token: 7}, sender, clock)
	defer s.Close()

	s.WriteRecord(0, rec(clock.Now(), make([]byte, 60)), pcapfile.RecordOptions{})
	clock.Advance(BatchAge)
	s.Flush()
	waitFor(t, "first batch", func() bool { return len(sender.get()) == 1 })

	t1 := sender.get()[0].SentAt
	recv := t1 + int64(time.Second)
	s.Ack(Control{Op: OpAck, Seq: 1, HeadRecv: recv, HeadSend: recv + int64(10*time.Millisecond)})

	s.mu.Lock()
	skew := s.skew
	s.mu.Unlock()
	if skew != 0 {
		t.Errorf("skew %v after an ack with a negative round trip, want 0", time.Duration(skew))
	}
}

// TestMergerForgetAndAddHost checks that a host that ns del-hosts removes
// does not show or stall the capture, and that a new member of the host
// starts its streams again from Seq 1 while the merger drops the batches of
// the old member.
func TestMergerForgetAndAddHost(t *testing.T) {
	clock := newFakeClock()
	m, buf := newTestMerger(t, clock, 0)

	now := clock.Now()
	m.Add(batch("c1", "br", 1, now, now.Add(-3*time.Second)))
	m.Add(batch("c1", "br", 2, now, now.Add(-2900*time.Millisecond)))
	m.Add(batch("c2", "br", 1, now, now.Add(-2800*time.Millisecond)))

	// c2 is gone; c1 still sends
	clock.Advance(StallTimeout)
	now = clock.Now()
	m.Add(batch("c1", "br", 3, now))

	states := func() map[string]string {
		res := map[string]string{}
		streams, _ := m.Status()
		for _, s := range streams {
			res[s.Host] = s.State
		}
		return res
	}

	if got := states(); got["c2"] != StateStalled {
		t.Fatalf("states %v, want c2 stalled", got)
	}

	m.ForgetHost("c2")
	if got := states(); len(got) != 1 || got["c1"] != StateRunning {
		t.Errorf("states %v after c2 is removed, want only c1 running", got)
	}

	// c1 is removed and added again with a new member
	m.ForgetHost("c1")
	if _, ok := m.Add(batch("c1", "br", 4, now)); ok {
		t.Error("batch of a removed host accepted")
	}

	m.AddHost("c1", 1)

	if _, ok := m.Add(batch("c1", "br", 5, now, now.Add(-time.Second))); ok {
		t.Error("late batch of the old member accepted")
	}
	if _, ok := m.Add(&Batch{Host: "c1", Final: true, Token: 7}); ok {
		t.Error("late final batch of the old member accepted")
	}

	b := batch("c1", "br", 1, now, now.Add(-500*time.Millisecond))
	b.Gen = 1
	if seq, ok := m.Add(b); !ok || seq != 1 {
		t.Errorf("first batch of the new member: ack %v %v, want 1", seq, ok)
	}

	if got := states(); len(got) != 1 || got["c1"] != StateRunning {
		t.Errorf("states %v, want c1 running", got)
	}
	if m.Done([]string{"c1"}) {
		t.Error("Done() before the final batch of the new member")
	}

	m.Add(&Batch{Host: "c1", Final: true, Token: 7, Gen: 1})
	if !m.Done([]string{"c1"}) {
		t.Error("not Done() after the final batch of the new member")
	}
	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	if got := readTimes(t, buf.Bytes()); len(got) != 4 {
		t.Errorf("file has %d records, want 4", len(got))
	}
	if _, sum := m.Status(); sum.Packets != 4 {
		t.Errorf("total packets %v, want 4", sum.Packets)
	}
}

// TestMergerPcapngNoVMIface checks that a port of a VM without a known
// interface gets the port description.
func TestMergerPcapngNoVMIface(t *testing.T) {
	clock := newFakeClock()

	var buf bytes.Buffer
	w, err := pcapfile.NewNg(&buf, pcapfile.Section{UserAppl: "minimega test"})
	if err != nil {
		t.Fatal(err)
	}

	m := NewMerger(MergerConfig{Namespace: "exp", Token: 7, SnapLen: 1600}, w, clock)

	mac := net.HardwareAddr{2, 0, 0, 0, 0, 1}
	now := clock.Now().Add(-time.Second)

	b := &Batch{Host: "c1", Bridge: "br", Seq: 1, Through: now.UnixNano(), Format: FormatNg, Token: 7, Packets: 1,
		PortTable: []PortInfo{{Index: 1, Port: "mega_tap3", MAC: mac.String(), VLAN: 101, VM: "web01", VMIface: -1}}}
	b.Data = appendNgRecord(b.Data, rec(now, frame(mac, 1)), 1, 0)
	m.Add(b)
	m.Add(&Batch{Host: "c1", Final: true, Token: 7})

	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewNgReader(bytes.NewReader(buf.Bytes()), pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}
	_, ci, err := r.ReadPacketData()
	if err != nil {
		t.Fatal(err)
	}

	i, _ := r.Interface(ci.InterfaceIndex)
	if want := "c1 port mega_tap3, VLAN 101"; i.Description != want {
		t.Errorf("interface description %q, want %q", i.Description, want)
	}
}

// TestMergerAddHostPorts checks that the new member of a host that ns
// add-hosts added again gets new port interfaces, because its port indexes
// start again from 1, and that the records of the old member keep theirs.
func TestMergerAddHostPorts(t *testing.T) {
	clock := newFakeClock()

	var buf bytes.Buffer
	w, err := pcapfile.NewNg(&buf, pcapfile.Section{UserAppl: "minimega test"})
	if err != nil {
		t.Fatal(err)
	}

	m := NewMerger(MergerConfig{Namespace: "exp", Token: 7, SnapLen: 1600}, w, clock)

	now := clock.Now().Add(-time.Second)
	portBatch := func(gen uint64, p PortInfo, ts time.Time) *Batch {
		b := &Batch{Host: "c1", Bridge: "br", Gen: gen, Seq: 1, Through: ts.UnixNano(), Format: FormatNg, Token: 7,
			Packets: 1, PortTable: []PortInfo{p}}
		b.Data = appendNgRecord(nil, rec(ts, frame(net.HardwareAddr{2, 0, 0, 0, 0, byte(gen)}, 1)), 1, 0)
		return b
	}

	m.Add(portBatch(0, PortInfo{Index: 1, Port: "mega_tap3", VLAN: 101, VM: "web01", VMIface: 0}, now))
	m.ForgetHost("c1")
	m.AddHost("c1", 1)
	m.Add(portBatch(1, PortInfo{Index: 1, Port: "mega_tap7", VLAN: 102, VM: "db01", VMIface: 1}, now.Add(time.Millisecond)))
	m.Add(&Batch{Host: "c1", Final: true, Token: 7, Gen: 1})

	if err := m.Finish([]string{"c1"}); err != nil {
		t.Fatal(err)
	}

	r, err := pcapgo.NewNgReader(bytes.NewReader(buf.Bytes()), pcapgo.DefaultNgReaderOptions)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for {
		_, ci, err := r.ReadPacketData()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		i, _ := r.Interface(ci.InterfaceIndex)
		got = append(got, i.Name+": "+i.Description)
	}

	want := []string{"c1/br/mega_tap3: web01 interface 0, VLAN 101", "c1/br/mega_tap7: db01 interface 1, VLAN 102"}
	if !slices.Equal(got, want) {
		t.Errorf("record interfaces %q, want %q", got, want)
	}
}

// TestMergerAddHostCounters checks that the counters of the old member of a
// host that ns add-hosts added again stay in the drops, and that the lost
// count compares the frames that all members read with all records.
func TestMergerAddHostCounters(t *testing.T) {
	clock := newFakeClock()
	m, _ := newTestMerger(t, clock, 0)

	now := clock.Now()
	times := func(n int) []time.Time {
		var res []time.Time
		for i := 0; i < n; i++ {
			res = append(res, now.Add(-3*time.Second+time.Duration(i)*time.Millisecond))
		}
		return res
	}

	b := batch("c1", "br", 1, now, times(100)...)
	b.Captured = 102
	b.KernelDrops, b.IfDrops, b.PathDrops, b.QueueDrops, b.SendDrops = 3, 1, 2, 1, 1
	m.Add(b)

	m.ForgetHost("c1")
	m.AddHost("c1", 1)

	b = batch("c1", "br", 1, now, times(5)...)
	b.Gen, b.Captured, b.KernelDrops = 1, 10, 4
	m.Add(b)
	m.Add(&Batch{Host: "c1", Final: true, Token: 7, Gen: 1})
	m.Tick()

	streams, sum := m.Status()
	if len(streams) != 1 {
		t.Fatalf("streams %+v, want one", streams)
	}

	want := StreamStatus{Packets: 105, KDrops: 8, PDrops: 2, QDrops: 2, Lost: 5}
	for _, st := range []StreamStatus{streams[0], sum} {
		got := StreamStatus{Packets: st.Packets, KDrops: st.KDrops, PDrops: st.PDrops, QDrops: st.QDrops, Lost: st.Lost}
		if got != want {
			t.Errorf("counters %+v, want %+v", got, want)
		}
	}
}
