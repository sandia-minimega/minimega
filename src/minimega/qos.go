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
func (qos *Qos) qosAddCmd(t string) []string {
	cmd := []string{"tc", "qdisc", "add", "dev", t, "root", "netem"}

	for p, v := range qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Generate a change command string from the qos.params map
// initialized in cliQos
func (qos *Qos) qosChangeCmd(oldQos *Qos, t string) []string {
	cmd := []string{"tc", "qdisc", "change", "dev", t, "root", "netem"}

	// Populate the newQos map with existing qos params
	for p, v := range oldQos.params {
		if _, ok := qos.params[p]; !ok {
			qos.params[p] = v
		}
	}

	for p, v := range qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}


// Generate a qos remove command for a given tap name
func (qos *Qos) qosRemoveCmd(t string) []string {
	return []string{"tc", "qdisc", "del", "dev", t, "root"}
}

// Execute a qos command
// If qos.params is nil qos contraints will be removed from the given tap
// If qos is already associated with the given tap name, the original
// constraints will be removed and updated to the new command string
func (qos *Qos) qosCmd(t string) error {
	var cmd []string

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	b, err := getBridgeFromTap(t)
	if err != nil {
		return err
	}

	tap, ok := b.Taps[t]
	if !ok {
		return errors.New(
			fmt.Sprintf("qosCmd: tap %s not found", t),
		)
	}

	// Only remove qos from taps which have constraints
	if qos.params == nil {
		if tap.qos == nil {
			return nil
		}
	}

	// Build the command
	if qos.params == nil {
		cmd = qos.qosRemoveCmd(t)
		qos = nil
	} else if tap.qos != nil {
		cmd = qos.qosChangeCmd(tap.qos, t)
	} else {
		cmd = qos.qosAddCmd(t)
	}

	// Execute the qos command
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		qos = nil
		processWrapper(qos.qosRemoveCmd(t)...)
	}

	// Update the tap qos field
	tap.qos = qos
	b.Taps[t] = tap

	return err
}

// Remove qos contraints from all taps
func qosRemoveAll() {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, b := range bridges {
		for tapName, t := range b.Taps {
			if t.qos != nil {
				cmd := t.qos.qosRemoveCmd(tapName)
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
