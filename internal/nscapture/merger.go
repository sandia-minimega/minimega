// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package nscapture

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
)

// Default values for the head.
const (
	DefaultWindow   = time.Second
	MinWindow       = 100 * time.Millisecond
	MaxWindow       = 10 * time.Second
	StallTimeout    = 10 * time.Second
	GapTimeout      = 5 * time.Second
	StatsInterval   = 10 * time.Second
	FlushInterval   = 100 * time.Millisecond
	MemoryCap       = 256 << 20
	MemoryLow       = 192 << 20
	DefaultAckLimit = 8
	SlowAckLimit    = 2
)

// Stream states.
const (
	StateStarting = "starting"
	StateRunning  = "running"
	StateStalled  = "stalled"
	StateStopping = "stopping"
	StateStopped  = "stopped"
	StateFailed   = "failed"
	StateOrphaned = "orphaned"
	StateFull     = "full"
)

// Attribution modes of pcapng records.
const (
	AttributePort   = "port"
	AttributeStream = "stream"
)

// Clock gives the time. Tests replace it.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock is the system clock.
var RealClock Clock = realClock{}

// MergerConfig holds the settings of a merger.
type MergerConfig struct {
	Namespace string
	Window    time.Duration
	SnapLen   uint32
	MaxSize   int64
	Attribute string // pcapng: port or stream
	Names     bool   // pcapng: write name resolution blocks
	Token     uint64
	Filter    string
}

// StreamStatus holds the counters of one stream, or of a whole capture.
type StreamStatus struct {
	Host, Bridge string
	State        string
	Error        string
	Ports        int
	Packets      uint64 // records written
	Bytes        uint64 // original length of the records written
	KDrops       uint64
	PDrops       uint64
	QDrops       uint64 // queue and send drops on the member
	Lost         uint64
	Late         uint64
	Dup          uint64
	Lag          time.Duration
	Skew         time.Duration
	HasSkew      bool
}

type mstream struct {
	idx          int
	host, bridge string
	nextSeq      uint64
	pending      map[uint64]*Batch
	gapSince     time.Time
	through      int64
	lastRecv     time.Time
	final        bool
	err          string
	last         Batch // counters of the last batch
	recv         uint64
	bytes        uint64
	late, dup    uint64
	lostBatches  uint64
	lastStats    time.Time
	ifaceStream  uint32
	hasIface     bool
	ports        map[uint32]PortInfo
	portIfaces   map[uint32]uint32
	startTime    time.Time
	stalledState bool
}

type heapItem struct {
	ts     int64
	stream int
	seq    uint64
	off    int
	rec    []byte
	iface  uint32
	opts   pcapfile.RecordOptions
}

type recHeap []heapItem

func (h recHeap) Len() int { return len(h) }
func (h recHeap) Less(i, j int) bool {
	a, b := h[i], h[j]
	switch {
	case a.ts != b.ts:
		return a.ts < b.ts
	case a.stream != b.stream:
		return a.stream < b.stream
	case a.seq != b.seq:
		return a.seq < b.seq
	}
	return a.off < b.off
}
func (h recHeap) Swap(i, j int)       { h[i], h[j] = h[j], h[i] }
func (h *recHeap) Push(x interface{}) { *h = append(*h, x.(heapItem)) }
func (h *recHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Merger merges the batches of all streams of one capture into one file. It
// is not safe for concurrent use: one goroutine calls all its methods.
type Merger struct {
	cfg   MergerConfig
	w     pcapfile.Writer
	clock Clock
	ng    bool

	streams   map[string]*mstream
	order     []*mstream
	hostFinal map[string]bool
	hostErr   map[string]string

	h           recHeap
	heapBytes   int
	lastWritten int64
	lastFlush   time.Time

	full  bool
	err   error
	names map[string]bool
}

// NewMerger returns a merger that writes to w.
func NewMerger(cfg MergerConfig, w pcapfile.Writer, clock Clock) *Merger {
	if cfg.Window == 0 {
		cfg.Window = DefaultWindow
	}
	if cfg.Attribute == "" {
		cfg.Attribute = AttributePort
	}
	if clock == nil {
		clock = RealClock
	}

	return &Merger{
		cfg:         cfg,
		w:           w,
		clock:       clock,
		ng:          w.Format() == pcapfile.FormatPcapng,
		streams:     map[string]*mstream{},
		hostFinal:   map[string]bool{},
		hostErr:     map[string]string{},
		lastWritten: math.MinInt64,
		lastFlush:   clock.Now(),
		names:       map[string]bool{},
	}
}

// Full reports whether the file reached its maximum size.
func (m *Merger) Full() bool { return m.full }

// Err returns the first write error.
func (m *Merger) Err() error { return m.err }

// Size returns the size of the file.
func (m *Merger) Size() int64 { return m.w.Size() }

// HeapBytes returns the bytes of the records that wait in the merge buffer.
func (m *Merger) HeapBytes() int { return m.heapBytes }

func (m *Merger) stream(host, bridge string) *mstream {
	key := host + "/" + bridge
	s, ok := m.streams[key]
	if !ok {
		now := m.clock.Now()
		s = &mstream{
			idx:        len(m.order),
			host:       host,
			bridge:     bridge,
			nextSeq:    1,
			pending:    map[uint64]*Batch{},
			lastRecv:   now,
			lastStats:  now,
			startTime:  now,
			ports:      map[uint32]PortInfo{},
			portIfaces: map[uint32]uint32{},
		}
		m.streams[key] = s
		m.order = append(m.order, s)
	}

	return s
}

// Add takes a batch. It returns the highest in-order sequence number of the
// stream, for the ack, and false when the batch is not for this capture.
func (m *Merger) Add(b *Batch) (uint64, bool) {
	if b.Token != m.cfg.Token {
		return 0, false
	}

	now := m.clock.Now()

	if b.Bridge == "" {
		// host-level final batch
		if b.Final {
			m.hostFinal[b.Host] = true
			if b.Err != "" {
				m.hostErr[b.Host] = b.Err
			}
		}
		return b.Seq, true
	}

	s := m.stream(b.Host, b.Bridge)
	s.lastRecv = now

	if b.Seq < s.nextSeq || s.pending[b.Seq] != nil {
		s.dup++
		return s.nextSeq - 1, true
	}

	s.pending[b.Seq] = b
	m.drain(s)

	return s.nextSeq - 1, true
}

// drain processes the pending batches of s in sequence order.
func (m *Merger) drain(s *mstream) {
	for {
		b, ok := s.pending[s.nextSeq]
		if !ok {
			if len(s.pending) > 0 && s.gapSince.IsZero() {
				s.gapSince = m.clock.Now()
			}
			return
		}

		delete(s.pending, s.nextSeq)
		s.nextSeq++
		s.gapSince = time.Time{}

		m.process(s, b)
	}
}

func (m *Merger) process(s *mstream, b *Batch) {
	if len(b.PortTable) > 0 {
		var names []pcapfile.Name
		for _, p := range b.PortTable {
			s.ports[p.Index] = p
			for _, a := range p.Addrs {
				name := p.VM
				if name == "" {
					name = p.Port
				}
				key := a + "=" + name
				if a != "" && name != "" && !m.names[key] {
					m.names[key] = true
					names = append(names, pcapfile.Name{Addr: a, Names: []string{name}})
				}
			}
		}
		if m.ng && m.cfg.Names && len(names) > 0 {
			m.setErr(m.w.WriteNames(names))
		}
	}

	recs, err := splitRecords(b.Data, b.Format, b.Packets, m.cfg.SnapLen)
	if err != nil {
		s.lostBatches++
	} else {
		for off, r := range recs {
			iface := m.iface(s, r.port)
			opts := pcapfile.RecordOptions{Inbound: true, DropsBefore: uint64(r.dropsB4)}

			if r.ts < m.lastWritten {
				// late: write at once, out of order
				s.late++
				opts.Comment = "late: after the reorder window"
				m.write(s, r.rec, iface, opts)
				continue
			}

			heap.Push(&m.h, heapItem{ts: r.ts, stream: s.idx, seq: b.Seq, off: off, rec: r.rec, iface: iface, opts: opts})
			m.heapBytes += len(r.rec)
		}
	}

	if b.Through > s.through {
		s.through = b.Through
	}

	s.last = *b
	s.last.Data = nil
	s.last.PortTable = nil

	if b.Final {
		s.final = true
		s.err = b.Err
	}

	for m.heapBytes > MemoryCap {
		m.popUntil(func() bool { return m.heapBytes <= MemoryLow })
	}
}

// iface returns the interface for a record of s from port. Classic pcap has
// one interface.
func (m *Merger) iface(s *mstream, port uint32) uint32 {
	if !m.ng {
		return 0
	}

	if m.cfg.Attribute == AttributePort && port != 0 {
		if id, ok := s.portIfaces[port]; ok {
			return id
		}

		if p, ok := s.ports[port]; ok {
			desc := fmt.Sprintf("%v port %v, VLAN %v", s.host, p.Port, p.VLAN)
			if p.VM != "" {
				desc = fmt.Sprintf("%v interface %v, VLAN %v", p.VM, p.VMIface, p.VLAN)
			}

			id, err := m.w.AddInterface(pcapfile.Interface{
				Name:        s.host + "/" + s.bridge + "/" + p.Port,
				Description: desc,
				Filter:      m.cfg.Filter,
				SnapLen:     m.cfg.SnapLen,
				Comment:     "inferred from source MAC",
			})
			if err == nil {
				s.portIfaces[port] = id
				return id
			}
			m.setErr(err)
		}
	}

	if !s.hasIface {
		id, err := m.w.AddInterface(pcapfile.Interface{
			Name:        s.host + "/" + s.bridge,
			Description: fmt.Sprintf("%v edge ingress on %v", m.cfg.Namespace, s.host),
			Filter:      m.cfg.Filter,
			SnapLen:     m.cfg.SnapLen,
		})
		m.setErr(err)
		s.ifaceStream, s.hasIface = id, err == nil
	}

	return s.ifaceStream
}

func (m *Merger) setErr(err error) {
	if err != nil && m.err == nil {
		m.err = err
	}
}

// write writes one record, unless the file is full.
func (m *Merger) write(s *mstream, rec []byte, iface uint32, opts pcapfile.RecordOptions) {
	if m.full || m.err != nil {
		return
	}

	if m.cfg.MaxSize > 0 && m.w.Size()+int64(len(rec)+64) > m.cfg.MaxSize {
		m.full = true
		return
	}

	if err := m.w.WriteRecord(iface, rec, opts); err != nil {
		m.setErr(err)
		return
	}

	h, _ := pcapfile.ParseRecordHeader(rec)
	s.recv++
	s.bytes += uint64(h.OrigLen)
}

func (m *Merger) popUntil(done func() bool) {
	for m.h.Len() > 0 && !done() {
		it := heap.Pop(&m.h).(heapItem)
		m.heapBytes -= len(it.rec)

		if it.ts > m.lastWritten {
			m.lastWritten = it.ts
		}

		m.write(m.order[it.stream], it.rec, it.iface, it.opts)
	}
}

// live reports whether s holds back the watermark.
func (m *Merger) live(s *mstream, now time.Time) bool {
	return !s.final && !m.hostFinal[s.host] && now.Sub(s.lastRecv) < StallTimeout
}

// Watermark returns the time up to which the merger writes records.
func (m *Merger) Watermark() int64 {
	now := m.clock.Now()

	w := int64(math.MaxInt64)
	for _, s := range m.order {
		if m.live(s, now) && s.through < w {
			w = s.through
		}
	}

	if floor := now.Add(-m.cfg.Window).UnixNano(); w < floor {
		w = floor
	}

	return w
}

// Tick writes the records up to the watermark, skips sequence gaps that are
// older than GapTimeout, writes statistics and flushes the file.
func (m *Merger) Tick() {
	now := m.clock.Now()

	for _, s := range m.order {
		if !s.gapSince.IsZero() && now.Sub(s.gapSince) >= GapTimeout {
			// skip to the lowest pending batch
			min := uint64(math.MaxUint64)
			for seq := range s.pending {
				if seq < min {
					min = seq
				}
			}
			s.lostBatches += min - s.nextSeq
			s.nextSeq = min
			s.gapSince = time.Time{}
			m.drain(s)
		}
	}

	w := m.Watermark()
	m.popUntil(func() bool { return m.h[0].ts > w })

	if m.ng {
		for _, s := range m.order {
			if now.Sub(s.lastStats) >= StatsInterval && s.hasIface {
				s.lastStats = now
				m.writeStats(s, now, "")
			}
		}
	}

	if now.Sub(m.lastFlush) >= FlushInterval {
		m.lastFlush = now
		m.setErr(m.w.Flush())
	}
}

func (m *Merger) writeStats(s *mstream, now time.Time, comment string) {
	st := m.status(s, now)
	m.setErr(m.w.WriteStats(s.ifaceStream, pcapfile.Stats{
		Start:     s.startTime,
		End:       now,
		Received:  s.last.Captured,
		IfDrop:    st.KDrops + st.PDrops,
		OSDrop:    st.QDrops + st.Lost,
		Delivered: st.Packets,
	}, comment))
}

// Done reports whether every stream and host sent its final batch.
func (m *Merger) Done(hosts []string) bool {
	for _, h := range hosts {
		if !m.hostFinal[h] {
			return false
		}
	}

	return true
}

// Finish writes every record that waits, the final statistics, and flushes
// and closes the writer. hosts without a final batch are marked failed.
func (m *Merger) Finish(hosts []string) error {
	now := m.clock.Now()

	for _, h := range hosts {
		if !m.hostFinal[h] && m.hostErr[h] == "" {
			m.hostErr[h] = "no final batch"
		}
	}

	m.popUntil(func() bool { return false })

	if m.ng {
		for _, s := range m.order {
			if s.hasIface {
				m.writeStats(s, now, "")
			}
		}
	}

	m.setErr(m.w.Close())

	return m.err
}

func (m *Merger) status(s *mstream, now time.Time) StreamStatus {
	st := StreamStatus{
		Host:    s.host,
		Bridge:  s.bridge,
		State:   StateRunning,
		Error:   s.err,
		Ports:   s.last.Ports,
		Packets: s.recv,
		Bytes:   s.bytes,
		KDrops:  s.last.KernelDrops + s.last.IfDrops,
		PDrops:  s.last.PathDrops,
		QDrops:  s.last.QueueDrops + s.last.SendDrops,
		Late:    s.late,
		Dup:     s.dup,
		Skew:    time.Duration(s.last.SkewNS),
		HasSkew: true,
	}

	// lost: frames that the member read but the head never got. Before the
	// final batch, frames in flight would count, so only lost batches show.
	if s.final || m.hostFinal[s.host] {
		got := s.recv + uint64(m.h.countStream(s.idx))
		if sent := s.last.Captured; sent > st.QDrops+got {
			st.Lost = sent - st.QDrops - got
		}
	}

	if s.through > 0 {
		st.Lag = now.Sub(time.Unix(0, s.through))
	}

	switch {
	case s.err != "":
		st.State = StateFailed
	case s.final || m.hostFinal[s.host]:
		st.State = StateStopped
	case now.Sub(s.lastRecv) >= StallTimeout:
		st.State = StateStalled
	}

	if e := m.hostErr[s.host]; e != "" && st.Error == "" {
		st.Error = e
		st.State = StateFailed
	}

	return st
}

func (h recHeap) countStream(idx int) int {
	var n int
	for _, it := range h {
		if it.stream == idx {
			n++
		}
	}
	return n
}

// Status returns the status of each stream, sorted by host and bridge, and
// the totals.
func (m *Merger) Status() ([]StreamStatus, StreamStatus) {
	now := m.clock.Now()

	var res []StreamStatus
	var sum StreamStatus

	for _, s := range m.order {
		st := m.status(s, now)
		res = append(res, st)

		sum.Ports += st.Ports
		sum.Packets += st.Packets
		sum.Bytes += st.Bytes
		sum.KDrops += st.KDrops
		sum.PDrops += st.PDrops
		sum.QDrops += st.QDrops
		sum.Lost += st.Lost
		sum.Late += st.Late
		sum.Dup += st.Dup
		if st.Lag > sum.Lag {
			sum.Lag = st.Lag
		}
	}

	for host, e := range m.hostErr {
		var found bool
		for _, st := range res {
			found = found || st.Host == host
		}
		if !found {
			res = append(res, StreamStatus{Host: host, State: StateFailed, Error: e})
		}
	}

	sort.Slice(res, func(i, j int) bool {
		if res[i].Host != res[j].Host {
			return res[i].Host < res[j].Host
		}
		return res[i].Bridge < res[j].Bridge
	})

	return res, sum
}
