package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
	"time"
)

// Used for queue length in qdisc netem
const (
	MIN_NETEM_LIMIT     = 10
	MAX_NETEM_LIMIT     = 1000
	DEFAULT_NETEM_LIMIT = 1000
)

// Traffic control actions
const (
	tcAdd    string = "add"
	tcDel    string = "del"
	tcUpdate string = "change"
)

// Qos option types
type QosType int

const (
	Rate QosType = iota
	Loss
	Delay
)

type QosOption struct {
	Type  QosType
	Value string
}

// Netem parameters
type qos struct {
	loss  string
	delay string
	rate  string
	limit string
}

func newQos() *qos {
	return &qos{}
}

// Set the initial qdisc namespace
func (t *Tap) initializeQos() error {
	t.Qos = newQos()
	t.Qos.limit = strconv.FormatUint(DEFAULT_NETEM_LIMIT, 10)
	cmd := []string{"tc", "qdisc", tcAdd, "dev", t.Name}
	ns := []string{"root", "handle", "1:", "netem", "limit", t.Qos.limit}
	return t.qosCmd(append(cmd, ns...))
}

func (t *Tap) destroyQos() error {
	if t.Qos == nil {
		return nil
	}
	t.Qos = nil
	cmd := []string{"tc", "qdisc", tcDel, "dev", t.Name, "root"}
	return t.qosCmd(cmd)
}

func (t *Tap) setQos(op QosOption) error {
	var action string
	var ns []string

	if t.Qos == nil {
		err := t.initializeQos()
		if err != nil {
			return err
		}
	}

	switch op.Type {
	case Loss:
		t.Qos.loss = op.Value
	case Delay:
		t.Qos.delay = op.Value
	case Rate:
		t.Qos.rate = op.Value
	}

	// only modify the limit if rate limiting is in effect
	if t.Qos.rate != "" {
		t.Qos.limit = getNetemLimit(t.Qos.rate, t.Qos.delay)
	} else {
		t.Qos.limit = strconv.FormatUint(DEFAULT_NETEM_LIMIT, 10)
	}

	action = tcUpdate
	cmd := []string{"tc", "qdisc", action, "dev", t.Name}
	ns = []string{"root", "handle", "1:", "netem"}

	// stack up parameters
	if t.Qos.limit != "" {
		ns = append(ns, "limit", t.Qos.limit)
	}
	if t.Qos.rate != "" {
		ns = append(ns, "rate", t.Qos.rate)
	}
	if t.Qos.loss != "" {
		ns = append(ns, "loss", t.Qos.loss)
	}
	if t.Qos.delay != "" {
		ns = append(ns, "delay", t.Qos.delay)
	}

	return t.qosCmd(append(cmd, ns...))
}

// Execute a qos command string
func (t *Tap) qosCmd(cmd []string) error {
	log.Debug("received qos command %v", cmd)
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		t.destroyQos()
	}
	return err
}

func (b *Bridge) ClearQos(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("clearing qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}
	return t.destroyQos()
}

func (b *Bridge) UpdateQos(tap string, op QosOption) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("updating qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}

	return t.setQos(op)
}

func (b *Bridge) GetQos(tap string) []QosOption {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	t, ok := b.taps[tap]
	if !ok {
		return nil
	}
	if t.Qos == nil {
		return nil
	}
	return b.getQos(t)
}

func (b *Bridge) getQos(t *Tap) []QosOption {
	var ops []QosOption

	if t.Qos.rate != "" {
		ops = append(ops, QosOption{Rate, t.Qos.rate})
	}
	if t.Qos.loss != "" {
		ops = append(ops, QosOption{Loss, t.Qos.loss})
	}
	if t.Qos.delay != "" {
		ops = append(ops, QosOption{Delay, t.Qos.delay})
	}
	return ops
}

// Tune netem's limit (queue length) to minimize latency,
// avoid unnecessary packet drops, and achieve reasonable TCP throughput
// We treat netem's limit parameter as a buffer size, even though the
// netem man page describes it differently (and incorrectly)
func getNetemLimit(rate string, delay string) string {
	r := rate[:len(rate)-4]
	unit := rate[len(rate)-4:]
	var bps uint64

	switch unit {
	case "kbit":
		bps = 1 << 10
	case "mbit":
		bps = 1 << 20
	case "gbit":
		bps = 1 << 30
	}
	rateUint, _ := strconv.ParseUint(r, 10, 64)

	d, _ := time.ParseDuration(delay)
	delayNsUint := uint64(d.Nanoseconds())
	// floor to 1 ms for purposes of sizing the limit
	if delayNsUint < 1e6 {
		delayNsUint = 1e6
	}

	// Bandwidth-delay product
	bdp := rateUint * bps * delayNsUint / 1e9

	// Limit is in packets, so divide BDP (in bits)
	// by typical packet size, roughly 10,000 bits
	// Empirically, then multiply by 1000, to avoid some observed premature drops
	// Buffers really should be tuned according to application, but
	// we can start off with something roughly reasonable...
	limit := bdp / 1e3
	log.Debug("rate %s, delay %s => bandwidth-delay product %d bits => auto-calculated limit %d packets", rate, delay, bdp, limit)

	if limit < MIN_NETEM_LIMIT {
		limit = MIN_NETEM_LIMIT
	}
	if limit > MAX_NETEM_LIMIT {
		limit = MAX_NETEM_LIMIT
	}
	return strconv.FormatUint(limit, 10)
}
