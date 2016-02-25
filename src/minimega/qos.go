package main

import (
	"errors"
	"minicli"
)

// Tap field enumerating qos parameters
type Qos struct {
	params map[string]string
	change bool
}

// Qos initializer
func newQos() *Qos {
	return &Qos{params: make(map[string]string)}
}

// Generate an add command string from the qos.params map
// initialized in cliQos
func (t *Tap) qosAddCmd() []string {
	cmd := []string{"tc", "qdisc", "add", "dev", t.name, "root", "netem"}

	for p, v := range t.qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Generate a change command string from the qos.params map
// initialized in cliQos
func (t *Tap) qosChangeCmd() []string {
	cmd := []string{"tc", "qdisc", "change", "dev", t.name, "root", "netem"}

	for p, v := range t.qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Generate a qos remove command for a given tap name
func (t *Tap) qosRemoveCmd() []string {
	return []string{"tc", "qdisc", "del", "dev", t.name, "root"}
}

// Execute a qos command
// If qos.params is nil qos contraints will be removed from the given tap
// If qos is already associated with the given tap name, the original
// constraints will be removed and updated to the new command string
func (t *Tap) qosCmd() error {
	var cmd []string

	// Build the command
	if t.qos == nil {
		cmd = t.qosRemoveCmd()
	} else if t.qos.change {
		cmd = t.qosChangeCmd()
	} else {
		cmd = t.qosAddCmd()
	}

	// Execute the qos command
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		t.qos = nil
		processWrapper(t.qosRemoveCmd()...)
	}

	return err
}

// Remove qos contraints from all taps
func qosRemoveAll() {
	for _, b := range bridges {
		for _, t := range b.Taps {
			if t.qos != nil {
				cmd := t.qosRemoveCmd()
				processWrapper(cmd...)
				t.qos = nil
			}
		}
	}
}

// List all taps which are associated with a qos constraint
func qosList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "loss", "delay"}
	resp.Tabular = [][]string{}
	
	for _, b := range bridges {
		for _, t := range b.Taps {
			if t.qos != nil {
				loss := t.qos.params["loss"]
				delay := t.qos.params["delay"]
				resp.Tabular = append(resp.Tabular, []string{
					b.Name, t.name, loss, delay,
				})
			}
		}
	}
}
