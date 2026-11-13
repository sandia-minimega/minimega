// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package nscapture

import (
	"bytes"
	"encoding/gob"
	"errors"
	"io"
	"net"
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

func TestWireGob(t *testing.T) {
	gob.Register(Batch{})
	gob.Register(Control{})

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()

	in := Batch{Namespace: "exp", Head: "head", ID: 3, Token: 9, Host: "c1", Bridge: "br", Seq: 4,
		Data: []byte{1, 2, 3}, PortTable: []PortInfo{{Index: 1, Port: "mega_tap1", Addrs: []string{"10.0.0.1"}}}}

	go func() {
		var v interface{} = in
		gob.NewEncoder(a).Encode(&v)
	}()

	var out interface{}
	if err := gob.NewDecoder(b).Decode(&out); err != nil {
		t.Fatal(err)
	}

	got, ok := out.(Batch)
	if !ok || got.Seq != 4 || !bytes.Equal(got.Data, in.Data) || got.PortTable[0].Addrs[0] != "10.0.0.1" {
		t.Errorf("round trip: %+v", out)
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

	clock.Advance(GapTimeout)
	m.Tick()
	m.w.Flush()

	if got := readTimes(t, buf.Bytes()); len(got) != 3 {
		t.Errorf("wrote %d records after the gap timeout, want 3", len(got))
	}

	if m.streams["c1/br"].lostBatches != 1 {
		t.Errorf("lost batches %v, want 1", m.streams["c1/br"].lostBatches)
	}
}

func TestMergerMaxSize(t *testing.T) {
	clock := newFakeClock()
	m, _ := newTestMerger(t, clock, 24+2*(16+60)+64)

	now := clock.Now()
	m.Add(batch("c1", "br", 1, now, now.Add(-3*time.Second), now.Add(-2*time.Second), now.Add(-time.Second)))
	clock.Advance(2 * time.Second)
	m.Tick()

	if !m.Full() {
		t.Error("not full")
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
		PortTable: []PortInfo{{Index: 1, Port: "mega_tap3", MAC: mac.String(), VLAN: 101, VM: "web01", Addrs: []string{"10.0.0.7"}}}}
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
