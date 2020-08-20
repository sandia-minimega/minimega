// Copyright (2017) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
)

// #include <unistd.h>
import "C"

var clkTck = int64(C.sysconf(C._SC_CLK_TCK))

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

// tc parameters
type qos struct {
	Loss  string
	Delay string
	Rate  string
}

func (t *Tap) removeQos() error {
	if t.qos == nil {
		return nil
	}
	t.qos = nil
	cmd := []string{"tc", "qdisc", "del", "dev", t.Name, "root"}
	return t.qosCmd(cmd)
}

func (t *Tap) addQos(op QosOption) error {
	if t.qos == nil {
		t.qos = &qos{}
	}

	// Rate and Loss/Delay are mutually exclusive... warn if we previously had
	// a rate qos and are replacing it with loss/delay (or vice versa).
	switch op.Type {
	case Loss, Delay:
		if t.qos.Rate != "" {
			log.Warn("replacing rate qos with loss/delay on %v", t.Name)
			t.qos.Rate = ""
		}
	case Rate:
		if t.qos.Loss != "" || t.qos.Delay != "" {
			log.Warn("replacing loss/delay qos with rate on %v", t.Name)
			t.qos.Loss = ""
			t.qos.Delay = ""
		}
	}

	switch op.Type {
	case Loss:
		t.qos.Loss = op.Value
		return t.qosNetem()
	case Delay:
		t.qos.Delay = op.Value
		return t.qosNetem()
	case Rate:
		t.qos.Rate = op.Value
		return t.qosTbf()
	}

	return errors.New("unreachable")
}

func (t *Tap) qosTbf() error {
	var rate int64
	var unit string
	for i := range t.qos.Rate {
		c := t.qos.Rate[i]
		if c < '0' || c > '9' {
			unit = t.qos.Rate[i:]
			break
		}
		rate = rate*10 + int64(c) - '0'
		if rate < 0 {
			return errors.New("overflow")
		}
	}

	log.Debug("parsed rate: %v, unit: %v", rate, unit)

	switch unit {
	case "gbit":
		rate *= 1000
		fallthrough
	case "mbit":
		rate *= 1000
		fallthrough
	case "kbit":
		rate *= 1000
	default:
		return errors.New("invalid rate unit")
	}

	// compute minimum burst by dividing rate by HZ, convert to kbit
	burst := strconv.FormatFloat(float64(rate)/float64(clkTck)/1000.0, 'f', 3, 64) + "kbit"

	log.Debug("computed burst for rate %v (%v): %v", t.qos.Rate, rate, burst)

	cmd := []string{
		"tc", "qdisc", "replace", "root", "dev", t.Name, "tbf",
		"rate", t.qos.Rate, "burst", burst, "latency", "20ms",
	}

	return t.qosCmd(cmd)
}

func (t *Tap) qosNetem() error {
	cmd := []string{
		"tc", "qdisc", "replace", "root", "dev", t.Name, "netem",
	}

	if t.qos.Delay != "" {
		cmd = append(cmd, "delay", t.qos.Delay)
	}
	if t.qos.Loss != "" {
		cmd = append(cmd, "loss", t.qos.Loss)
	}

	return t.qosCmd(cmd)
}

// Execute a qos command string
func (t *Tap) qosCmd(cmd []string) error {
	log.Debug("received qos command for %v: `%v`", t.Name, cmd)
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		t.removeQos()
	}
	return err
}

func (t *Tap) getQos() []QosOption {
	var ops []QosOption

	if t.qos.Rate != "" {
		ops = append(ops, QosOption{Rate, t.qos.Rate})
	}
	if t.qos.Loss != "" {
		ops = append(ops, QosOption{Loss, t.qos.Loss})
	}
	if t.qos.Delay != "" {
		ops = append(ops, QosOption{Delay, t.qos.Delay})
	}

	return ops
}

func (b *Bridge) RemoveQos(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("clearing qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}
	return t.removeQos()
}

func (b *Bridge) UpdateQos(tap string, op QosOption) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("updating qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}

	return t.addQos(op)
}

func (b *Bridge) GetQos(tap string) []QosOption {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	t, ok := b.taps[tap]
	if !ok {
		return nil
	}
	if t.qos == nil {
		return nil
	}
	return t.getQos()
}
