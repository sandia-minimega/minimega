package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
)

// Used to calulate burst rate for the token bucket filter qdisc
const KERNEL_TIMER_FREQ uint64 = 250
const MIN_BURST_SIZE uint64 = 2048

// Traffic control actions
var (
	tcAdd    string = "add"
	tcDel    string = "del"
	tcUpdate string = "change"
)

type QosType int

const (
	Rate QosType = iota
	Loss
	Delay
	Remove
	Init
)

type QosOption struct {
	Type  QosType
	Value string
}

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

func NewQos() *Qos {
	return &Qos{NetemParams: &NetemParams{}}
}

// Set the initial qdisc namespace
func (t *Tap) initializeQos() error {
	t.Qos = NewQos()
	op := QosOption{Type: Init}
	cmd := t.getQosCmd(tcAdd, op)
	return t.qosCmd(cmd)
}

// Generate a qos command string given a QosOption
func (t *Tap) getQosCmd(action string, op QosOption) []string {
	var ns []string

	cmd := []string{"tc", "qdisc", action, "dev", t.Name}

	switch op.Type {
	case Init:
		ns = []string{"root", "handle", "1:", "netem", "loss", "0"}
	case Remove:
		ns = []string{"root"}
	case Loss:
		ns = []string{"root", "handle", "1:", "netem", "loss", op.Value}
	case Delay:
		ns = []string{"root", "handle", "1:", "netem", "delay", op.Value}
	case Rate:
		// Burst is set dynamically in updateQos
		ns = []string{"root", "parent", "1:", "handle", "2:", "tbf",
			"rate", op.Value, "latency", "100ms", "burst", t.Qos.TbfParams.Burst}
	}
	return append(cmd, ns...)
}

// Execute a qos command string
func (t *Tap) qosCmd(cmd []string) error {
	log.Error("recieved qos command %v", cmd)
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		processWrapper(t.getQosCmd(tcDel, QosOption{Type: Remove})...)
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
	op := QosOption{Type: Remove}
	cmd := t.getQosCmd(tcDel, op)
	return t.qosCmd(cmd)
}

func (b *Bridge) UpdateQos(tap string, op QosOption) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Error("updating qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}

	return b.updateQos(t, op)
}

func (b *Bridge) updateQos(t *Tap, op QosOption) error {
	if t.Qos == nil {
		err := t.initializeQos()
		if err != nil {
			return err
		}
	}

	var action string

	if op.Type == Rate {
		if t.Qos.TbfParams == nil {
			action = tcAdd
			t.Qos.TbfParams = &TbfParams{}
		} else {
			action = tcUpdate
		}
		t.Qos.TbfParams.Rate = op.Value
		t.Qos.TbfParams.Burst = getQosBurst(op.Value)
	} else {
		// the netem qdisc will always be an update action as there
		// is a default netem rule applied in initializeQos()
		action = tcUpdate

		if op.Type == Loss {
			t.Qos.NetemParams.Loss = op.Value
		}
		if op.Type == Delay {
			t.Qos.NetemParams.Delay = op.Value
		}
	}

	cmd := t.getQosCmd(action, op)
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

func getQosBurst(rate string) string {
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
	burst, _ := strconv.ParseUint(r, 10, 64)

	// Burst size is in bytes
	burst = ((burst * bps) / KERNEL_TIMER_FREQ) / 8
	if burst < MIN_BURST_SIZE {
		burst = MIN_BURST_SIZE
	}
	return fmt.Sprintf("%db", burst)
}
