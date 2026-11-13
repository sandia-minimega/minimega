// Copyright 2026 National Technology & Engineering Solutions of Sandia, LLC (NTESS).
// Under the terms of Contract DE-NA0003525 with NTESS, the U.S. Government retains certain
// rights in this software.

package nscapture

import (
	"errors"
	"net"
	"sync"
	"time"

	"github.com/sandia-minimega/minimega/v2/internal/pcapfile"
	log "github.com/sandia-minimega/minimega/v2/pkg/minilog"
)

// Default values for members.
const (
	BatchTarget     = 256 << 10
	BatchCap        = 1 << 20
	BatchAge        = 50 * time.Millisecond
	HeartbeatPeriod = 500 * time.Millisecond
	QueueCap        = 32 << 20
	AckStall        = 15 * time.Second
	SendRetryLimit  = 30 * time.Second
	DrainLimit      = 5 * time.Second
	OrphanTimeout   = 120 * time.Second
)

// sendRetryLimit is SendRetryLimit; tests make it shorter.
var sendRetryLimit = SendRetryLimit

// Sender sends a message to a host. An error means that the message did not
// reach the next hop or that host has no route.
type Sender interface {
	Send(host string, body interface{}) error
}

// StreamConfig holds the settings of a member stream.
type StreamConfig struct {
	Namespace, Head, Host, Bridge string
	ID                            int
	Token                         uint64
	Format                        uint8
}

// Stream batches the records of one host and bridge and sends them to the
// head. It is a pcapfile.Writer, so that a capture engine writes to it.
type Stream struct {
	cfg    StreamConfig
	sender Sender
	clock  Clock

	mu   sync.Mutex
	cond *sync.Cond

	cur        []byte
	curPackets uint32
	curStart   time.Time
	maxTs      int64
	lastIdle   time.Time
	lastSend   time.Time
	newRecords bool

	queue      []*Batch
	queueBytes int
	seq        uint64 // last seq given to a batch
	sent       uint64 // last seq sent
	acked      uint64
	window     int
	lastAck    time.Time
	sentAt     map[uint64]int64
	skew       int64
	bestRTT    int64
	bestRTTAt  time.Time

	captured, capturedBytes uint64
	queueDrops, sendDrops   uint64
	dropsBefore             uint32
	kdrops, ifdrops         uint64
	pathDrops               uint64
	ports                   []PortInfo
	portByMAC               map[string]uint32
	portsChanged            bool

	closed   bool
	closeErr string
	unknown  bool
	done     chan struct{}

	// OnUnknown runs when the head does not know the capture.
	OnUnknown func()
}

// NewStream starts the sender of a stream.
func NewStream(cfg StreamConfig, sender Sender, clock Clock) *Stream {
	if clock == nil {
		clock = RealClock
	}
	if cfg.Format == 0 {
		cfg.Format = FormatClassic
	}

	now := clock.Now()
	s := &Stream{
		cfg:       cfg,
		sender:    sender,
		clock:     clock,
		curStart:  now,
		lastIdle:  now,
		lastSend:  now,
		lastAck:   now,
		window:    DefaultAckLimit,
		sentAt:    map[uint64]int64{},
		portByMAC: map[string]uint32{},
		done:      make(chan struct{}),
	}
	s.cond = sync.NewCond(&s.mu)

	go s.sendLoop()

	return s
}

// SetPorts sets the edge ports of the stream, for pcapng attribution and
// names.
func (s *Stream) SetPorts(ports []PortInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ports = ports
	s.portByMAC = map[string]uint32{}
	for _, p := range ports {
		if mac, err := net.ParseMAC(p.MAC); err == nil {
			s.portByMAC[string(mac)] = p.Index
		}
	}
	s.portsChanged = true
}

// SetDrops sets the drop counters that the packet source reports.
func (s *Stream) SetDrops(kernel, iface, path uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.kdrops, s.ifdrops, s.pathDrops = kernel, iface, path
}

// Fail sets the error that the final batch carries.
func (s *Stream) Fail(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closeErr == "" {
		s.closeErr = err
	}
}

func (s *Stream) Format() string {
	if s.cfg.Format == FormatNg {
		return pcapfile.FormatPcapng
	}
	return pcapfile.FormatPcap
}

func (s *Stream) AddInterface(pcapfile.Interface) (uint32, error) { return 0, nil }
func (s *Stream) WriteStats(uint32, pcapfile.Stats, string) error { return nil }
func (s *Stream) WriteNames([]pcapfile.Name) error                { return nil }
func (s *Stream) Size() int64                                     { return 0 }

// WriteRecord adds a classic record to the current batch.
func (s *Stream) WriteRecord(_ uint32, rec []byte, _ pcapfile.RecordOptions) error {
	h, err := pcapfile.ParseRecordHeader(rec)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return pcapfile.ErrClosed
	}

	s.captured++
	s.capturedBytes += uint64(h.OrigLen)

	if s.queueBytes+len(s.cur) > QueueCap {
		s.queueDrops++
		s.dropsBefore++
		return nil
	}

	if s.curPackets == 0 {
		s.curStart = s.clock.Now()
	}

	if s.cfg.Format == FormatNg {
		var port uint32
		if len(rec) >= pcapfile.RecordHeaderLen+12 {
			port = s.portByMAC[string(rec[pcapfile.RecordHeaderLen+6:pcapfile.RecordHeaderLen+12])]
		}
		s.cur = appendNgRecord(s.cur, rec, port, s.dropsBefore)
		s.dropsBefore = 0
	} else {
		s.cur = append(s.cur, rec...)
	}

	s.curPackets++
	s.newRecords = true
	if ts := h.Time.UnixNano(); ts > s.maxTs {
		s.maxTs = ts
	}

	if len(s.cur) >= BatchTarget || len(s.cur)+pcapfile.RecordHeaderLen+ngRecordExtra+65535 > BatchCap {
		s.enqueueLocked(false)
	}

	return nil
}

// Flush sends the current batch when it is old enough, or a heartbeat when
// the stream is idle.
func (s *Stream) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()

	if !s.newRecords {
		s.lastIdle = now
	}
	s.newRecords = false

	switch {
	case s.curPackets > 0 && now.Sub(s.curStart) >= BatchAge:
		s.enqueueLocked(false)
	case s.curPackets == 0 && now.Sub(s.lastSend) >= HeartbeatPeriod:
		s.enqueueLocked(false)
	}

	return nil
}

// Close sends the final batch and waits until the queue is sent, at most
// DrainLimit.
func (s *Stream) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}

	s.enqueueLocked(true)
	s.closed = true
	s.cond.Broadcast()
	s.mu.Unlock()

	select {
	case <-s.done:
	case <-time.After(DrainLimit):
		log.Warn("capture %v: stream %v did not drain in %v", s.cfg.ID, s.cfg.Bridge, DrainLimit)
	}

	return nil
}

// Done is closed when the sender has sent the final batch.
func (s *Stream) Done() <-chan struct{} { return s.done }

// through is the time up to which all records are in sent batches.
func (s *Stream) throughLocked() int64 {
	t := s.lastIdle.Add(-100 * time.Millisecond).UnixNano()
	if s.maxTs > 0 && s.maxTs-int64(10*time.Millisecond) > t {
		t = s.maxTs - int64(10*time.Millisecond)
	}

	return t
}

func (s *Stream) enqueueLocked(final bool) {
	s.seq++

	b := &Batch{
		Namespace:     s.cfg.Namespace,
		Head:          s.cfg.Head,
		ID:            s.cfg.ID,
		Token:         s.cfg.Token,
		Host:          s.cfg.Host,
		Bridge:        s.cfg.Bridge,
		Seq:           s.seq,
		Through:       s.throughLocked(),
		Packets:       s.curPackets,
		Data:          s.cur,
		Captured:      s.captured,
		CapturedBytes: s.capturedBytes,
		KernelDrops:   s.kdrops,
		IfDrops:       s.ifdrops,
		PathDrops:     s.pathDrops,
		QueueDrops:    s.queueDrops,
		SendDrops:     s.sendDrops,
		Ports:         len(s.ports),
		SkewNS:        s.skew,
		Final:         final,
		Err:           s.closeErr,
		Format:        s.cfg.Format,
	}

	if s.portsChanged && s.cfg.Format == FormatNg {
		b.PortTable = append([]PortInfo(nil), s.ports...)
		s.portsChanged = false
	}

	s.queue = append(s.queue, b)
	s.queueBytes += len(b.Data)
	s.cur = nil
	s.curPackets = 0
	s.lastSend = s.clock.Now()

	s.cond.Broadcast()
}

// Ack takes an ack from the head.
func (s *Stream) Ack(c Control) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()

	if c.Unknown {
		if !s.unknown {
			s.unknown = true
			if s.OnUnknown != nil {
				go s.OnUnknown()
			}
		}
		return
	}

	if c.Seq > s.acked {
		s.acked = c.Seq
		s.lastAck = now
	}
	if c.Window > 0 {
		s.window = c.Window
	}

	// skew from the ack with the smallest round trip in the last 30 s
	if t1, ok := s.sentAt[c.Seq]; ok && c.HeadRecv != 0 && c.HeadSend != 0 {
		t4 := now.UnixNano()
		rtt := (t4 - t1) - (c.HeadSend - c.HeadRecv)
		if s.bestRTT == 0 || rtt <= s.bestRTT || now.Sub(s.bestRTTAt) > 30*time.Second {
			s.bestRTT, s.bestRTTAt = rtt, now
			s.skew = ((c.HeadRecv - t1) + (c.HeadSend - t4)) / 2
		}
	}
	for seq := range s.sentAt {
		if seq <= s.acked {
			delete(s.sentAt, seq)
		}
	}

	s.cond.Broadcast()
}

// Counters returns the captured, queue drop and send drop counters.
func (s *Stream) Counters() (captured, queueDrops, sendDrops uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.captured, s.queueDrops, s.sendDrops
}

var errStreamClosed = errors.New("stream closed")

// next waits for the next batch that the window allows.
func (s *Stream) next() (*Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		if s.unknown {
			return nil, errStreamClosed
		}

		if len(s.queue) > 0 {
			inFlight := s.sent - s.acked
			if inFlight < uint64(s.window) {
				break
			}

			if s.clock.Now().Sub(s.lastAck) >= AckStall {
				log.Warn("capture %v: no ack from %v for %v, continuing", s.cfg.ID, s.cfg.Head, AckStall)
				s.acked = s.sent
				s.lastAck = s.clock.Now()
				continue
			}
		} else if s.closed {
			return nil, errStreamClosed
		}

		s.waitLocked(10 * time.Millisecond)
	}

	b := s.queue[0]
	s.queue = s.queue[1:]
	s.queueBytes -= len(b.Data)
	s.sent = b.Seq
	if s.sent-s.acked == 1 {
		s.lastAck = s.clock.Now()
	}

	return b, nil
}

// waitLocked waits on the cond, at most d.
func (s *Stream) waitLocked(d time.Duration) {
	t := time.AfterFunc(d, func() {
		s.mu.Lock()
		s.cond.Broadcast()
		s.mu.Unlock()
	})
	s.cond.Wait()
	t.Stop()
}

func (s *Stream) sendLoop() {
	defer close(s.done)

	for {
		b, err := s.next()
		if err != nil {
			return
		}

		if !s.send(b) {
			s.mu.Lock()
			s.sendDrops += uint64(b.Packets)
			s.mu.Unlock()
		}
	}
}

// send sends b with retries and the same Seq, for up to SendRetryLimit.
func (s *Stream) send(b *Batch) bool {
	start := time.Now()
	backoff := 100 * time.Millisecond

	for {
		now := s.clock.Now()
		b.SentAt = now.UnixNano()

		s.mu.Lock()
		s.sentAt[b.Seq] = b.SentAt
		unknown := s.unknown
		s.mu.Unlock()

		if unknown {
			return false
		}

		err := s.sender.Send(s.cfg.Head, *b)
		if err == nil {
			return true
		}

		if time.Since(start) >= sendRetryLimit {
			log.Error("capture %v: dropping batch %v of %v: %v", s.cfg.ID, b.Seq, s.cfg.Bridge, err)
			return false
		}

		time.Sleep(backoff)
		if backoff *= 2; backoff > 2*time.Second {
			backoff = 2 * time.Second
		}
	}
}
