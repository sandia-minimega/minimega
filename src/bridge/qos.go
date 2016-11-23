package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
)

// Used for queue length in qdisc netem
const (
	MIN_NETEM_LIMIT     = 1
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

type netemParams struct {
	loss  string
	delay string
	rate  string
	limit string
}

// Tap field enumerating qos parameters
type qos struct {
	*netemParams // embed
}

func newQos() *qos {
	return &qos{
		netemParams: &netemParams{},
	}
}

// Set the initial qdisc namespace
func (t *Tap) initializeQos() error {
	t.Qos = newQos()
	t.Qos.netemParams.limit = fmt.Sprintf("%d", DEFAULT_NETEM_LIMIT)
	cmd := []string{"tc", "qdisc", tcAdd, "dev", t.Name}
	ns := []string{"root", "handle", "1:", "netem", "limit", t.Qos.netemParams.limit}
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
	var ps []string

	if t.Qos == nil {
		err := t.initializeQos()
		if err != nil {
			return err
		}
	}

	switch op.Type {
	case Loss:
		t.Qos.netemParams.loss = op.Value
	case Delay:
		t.Qos.netemParams.delay = op.Value
	case Rate:
		t.Qos.netemParams.rate = op.Value
		t.Qos.netemParams.limit = getNetemLimit(op.Value)
	}

	action = tcUpdate
	cmd := []string{"tc", "qdisc", action, "dev", t.Name}
	ns = []string{"root", "handle", "1:", "netem"}

	// stack up parameters
	if t.Qos.netemParams.limit != "" {
		ps = []string{"limit", t.Qos.netemParams.limit}
		ns = append(ns, ps...)
	}
	if t.Qos.netemParams.rate != "" {
		ps = []string{"rate", t.Qos.netemParams.rate}
		ns = append(ns, ps...)
	}
	if t.Qos.netemParams.loss != "" {
		ps = []string{"loss", t.Qos.netemParams.loss}
		ns = append(ns, ps...)
	}
	if t.Qos.netemParams.delay != "" {
		ps = []string{"delay", t.Qos.netemParams.delay}
		ns = append(ns, ps...)
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

	if t.Qos.netemParams.rate != "" {
		ops = append(ops, QosOption{Rate, t.Qos.netemParams.rate})
	}
	if t.Qos.netemParams.loss != "" {
		ops = append(ops, QosOption{Loss, t.Qos.netemParams.loss})
	}
	if t.Qos.netemParams.delay != "" {
		ops = append(ops, QosOption{Delay, t.Qos.netemParams.delay})
	}
	return ops
}

// Empirically tune netem's limit (queue length) to minimize latency AND avoid unnecessary packet drops
func getNetemLimit(rate string) string {
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
	limit, _ := strconv.ParseUint(r, 10, 64)

	// Limit is in number of packets
	limit = (limit * bps) / 10000000
	if limit < MIN_NETEM_LIMIT {
		limit = MIN_NETEM_LIMIT
	}
	if limit > MAX_NETEM_LIMIT {
		limit = MAX_NETEM_LIMIT
	}
	return fmt.Sprintf("%d", limit)
}
