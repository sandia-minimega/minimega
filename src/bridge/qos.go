package bridge

import (
	"errors"
	"fmt"
	log "minilog"
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
	cmd := []string{"tc", "qdisc", tcAdd, "dev", t.Name}
	ns := []string{"root", "handle", "1:", "netem", "limit", "10"}
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
		action = tcUpdate
		ns = []string{"root", "handle", "1:", "netem", "limit", "10", "loss", op.Value}
		t.Qos.netemParams.loss = op.Value
	case Delay:
		action = tcUpdate
		ns = []string{"root", "handle", "1:", "netem", "limit", "10", "delay", op.Value}
		t.Qos.netemParams.delay = op.Value
	case Rate:
		action = tcUpdate
		ns = []string{"root", "handle", "1:", "netem", "limit", "10", "rate", op.Value}
		t.Qos.netemParams.rate = op.Value
	}

	cmd := []string{"tc", "qdisc", action, "dev", t.Name}
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
