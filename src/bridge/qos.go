package bridge

import (
	"errors"
	"fmt"
	log "minilog"
)

var (
	netemNs string = "root handle 1: netem delay %s loss %s"
	tbfNs   string = "parent 1: handle 2: tbf rate %s latency 100ms burst %s"
)

type TbfParams struct {
	Rate  string
	Burst string
}

type NetemParams struct {
	Loss  string
	Delay string
}

// Tap field enumerating qos parameters
type Qos struct {
	*TbfParams   // embed
	*NetemParams // embed
}

// Set the initial qdisc namespace
func (t *Tap) qosInitialize() error {
	t.Qos = &Qos{NetemParams: &NetemParams{Loss: "0ms", Delay: "0%"}}
	cmd := t.qosGetCmd("update", "netem")
	return t.qosCmd(cmd)
}

// Generate a qos command string given the requested qdisc
func (t *Tap) qosGetCmd(op, qdisc string) []string {
	var ns string

	if op == "remove" {
		return []string{"tc", "qdisc", "del", "dev", t.Name, "root"}
	}

	// Base cmd
	cmd := fmt.Sprintf("tc qdisc change dev %s", t.Name)

	if qdisc == "tbf" {
		ns = fmt.Sprintf(tbfNs, t.Qos.Rate, t.Qos.Burst)
	} else {
		ns = fmt.Sprintf(netemNs, t.Qos.Delay, t.Qos.Loss)
	}
	return []string{cmd, ns}
}

// Execute a qos command string
func (t *Tap) qosCmd(cmd []string) error {
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		processWrapper(t.qosGetCmd("remove", "")...)
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
	return b.clearQos(t)
}

func (b *Bridge) clearQos(t *Tap) error {
	t.Qos = nil
	cmd := t.qosGetCmd("remove", "")
	return t.qosCmd(cmd)
}

func (b *Bridge) UpdateQos(tap string, qos *Qos) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("updating qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}
	return b.updateQos(t, qos)
}

func (b *Bridge) updateQos(t *Tap, qos *Qos) error {
	var qdisc string

	if t.Qos == nil {
		err := t.qosInitialize()
		if err != nil {
			return err
		}
	}

	if qos.TbfParams != nil {
		t.Qos.TbfParams = qos.TbfParams
		qdisc = "tbf"
	} else {
		t.Qos.NetemParams = qos.NetemParams
		qdisc = "netem"
	}

	cmd := t.qosGetCmd("update", qdisc)
	return t.qosCmd(cmd)
}

func (b *Bridge) GetQos(tap string) *Qos {
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

// Return a copy of the tap's Qos struct
func (b *Bridge) getQos(t *Tap) *Qos {
	qos := &Qos{}
	qos.TbfParams = t.Qos.TbfParams
	qos.NetemParams = t.Qos.NetemParams
	return qos
}