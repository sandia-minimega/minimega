package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
	"time"
)

// Used for queue length in qdisc netem
// Empirically determined; lower values than minNetemLimit resulted in
// unnecessary packet drops due to queue overfilling before it could be drained,
// even without congestion (possibly due to limited tick granularity?)
// maxNetemLimit is just set at the default Netem limit for now, which worked
// without issue at line rate (20-40 Gbps) in testing
const (
	minNetemLimit     = 10
	maxNetemLimit     = 1000
	defaultNetemLimit = 1000
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

// Set the initial qdisc namespace
func (t *Tap) initializeQos() error {
	t.Qos = &qos{}
	t.Qos.limit = strconv.FormatUint(defaultNetemLimit, 10)
	cmd := []string{
		"tc", "qdisc", "add", "dev", t.Name,
		"root", "handle", "1:", "netem", "limit", t.Qos.limit,
	}
	return t.qosCmd(cmd)
}

func (t *Tap) destroyQos() error {
	if t.Qos == nil {
		return nil
	}
	t.Qos = nil
	cmd := []string{"tc", "qdisc", "del", "dev", t.Name, "root"}
	return t.qosCmd(cmd)
}

func (t *Tap) setQos(op QosOption) error {
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
		t.Qos.limit = strconv.FormatUint(defaultNetemLimit, 10)
	}

	cmd := []string{
		"tc", "qdisc", "change", "dev", t.Name,
		"root", "handle", "1:", "netem",
	}

	// stack up parameters
	if t.Qos.limit != "" {
		cmd = append(cmd, "limit", t.Qos.limit)
	}
	if t.Qos.rate != "" {
		cmd = append(cmd, "rate", t.Qos.rate)
	}
	if t.Qos.loss != "" {
		cmd = append(cmd, "loss", t.Qos.loss)
	}
	if t.Qos.delay != "" {
		cmd = append(cmd, "delay", t.Qos.delay)
	}

	return t.qosCmd(cmd)
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

	if limit < minNetemLimit {
		limit = minNetemLimit
	}
	if limit > maxNetemLimit {
		limit = maxNetemLimit
	}
	return strconv.FormatUint(limit, 10)
}
