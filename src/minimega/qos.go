package main

import (
	"errors"
	"fmt"
	"minicli"
)

// Tap field enumerating qos parameters
type Qos struct {
	params map[string]string
}

// Qos initializer
func newQos() *Qos {
	return &Qos{params: make(map[string]string)}
}

// Generate an add command string from the qos.params map
// initialized in cliQos
func qosAddCmd(qos *Qos, t string) []string {
	cmd := []string{"tc", "qdisc", "add", "dev", t, "root", "netem"}

	for p, v := range qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Generate a qos remove command for a given tap name
func qosRemoveCmd(t string) []string {
	return []string{"tc", "qdisc", "del", "dev", t, "root"}
}

// Execute a qos command
// If qos is nil qos contraints will be removed from the given tap
// If qos is already associated with the given tap name, the original
// constraints will be removed and updated to the new command string
func qosCmd(qos *Qos, t string) error {
	var cmd []string

	// Build the command
	if qos == nil {
		cmd = qosRemoveCmd(t)
	} else {
		cmd = qosAddCmd(qos, t)
	}

	// Get bridge, grab bridge lock
	b, err := getBridgeFromTap(t)
	if err != nil {
		return err
	}
	b.Lock()
	defer b.Unlock()

	tap, ok := b.Taps[t]
	if !ok {
		return errors.New(
			fmt.Sprintf("qosCmd: tap %s not found", t),
		)
	}

	// If there is already a qos parameter associated with this tap
	// clear it before updating to the new parameter
	if tap.qos != nil && qos != nil {
		clearCmd := qosRemoveCmd(t)
		out, err := processWrapper(clearCmd...)
		if err != nil {
			return errors.New(out)
		}
		tap.qos = nil
	}

	// Only remove qos from taps which have constraints
	if qos == nil {
		if tap.qos == nil {
			return nil
		}
	}

	// Update the tap qos field
	tap.qos = qos
	b.Taps[t] = tap

	out, err := processWrapper(cmd...)
	if err != nil {
		return errors.New(out)
	}

	return nil
}

// Remove qos contraints from all taps
func qosRemoveAll() {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, b := range bridges {
		for tapName, t := range b.Taps {
			if t.qos != nil {
				cmd := qosRemoveCmd(tapName)
				processWrapper(cmd...)
				t.qos = nil
				b.Taps[tapName] = t
			}
		}
	}
}

// List all taps which are associated with a qos constraint
func qosList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "loss", "delay"}
	resp.Tabular = [][]string{}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()
	
	for _, b := range bridges {
		for tapName, t := range b.Taps {
			if t.qos != nil {
				loss := t.qos.params["loss"]
				delay := t.qos.params["delay"]
				resp.Tabular = append(resp.Tabular, []string{
					b.Name, tapName, loss, delay,
				})
			}
		}
	}
}
