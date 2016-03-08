package main

import (
	"errors"
	"minicli"
)

// Tap field enumerating qos parameters
type Qos struct {
	// current command parameters
	params map[string]string

	// tbf qdisc
	tbfNs     []string
	tbfParams map[string]string

	// netem qdisc
	netemNs     []string
	netemParams map[string]string
}

// Qos initializer
func newQos() *Qos {
	return &Qos{params: make(map[string]string),
		tbfParams:   make(map[string]string),
		netemParams: make(map[string]string),
	}
}

// Generate a add command string from the qos.params map
// and the qdisc namespace
func (t *Tap) qosGetCmd(op string, ns []string) []string {

	if op == "remove" {
		t.qos = nil
		return []string{"tc", "qdisc", "del", "dev", t.name, "root"}
	}

	// Base cmd
	cmd := []string{"tc", "qdisc", op, "dev", t.name}

	// Add qdisc namespace
	for _, n := range ns {
		cmd = append(cmd, n)
	}

	// Add tc constraint parameters
	for p, v := range t.qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Given a tc qdisc (netem, tbf) generate the correct namespace for a tc command.
// This is required because to chain together qdiscs
func (t *Tap) qosNamespace(qdisc string) []string {

	if qdisc == "netem" {
		// This is the root qdisc
		if t.qos.tbfNs == nil {
			t.qos.netemNs = []string{"root", "handle", "1:", "netem"}
		} else {
			// Chain the netem qdisc to the existing tbf qdisc
			t.qos.netemNs = []string{"parent", "1:", "handle", "2:", "netem"}
		}
		// Update the command parameters
		t.qos.params = t.qos.netemParams

		return t.qos.netemNs
	}

	if qdisc == "tbf" {
		// This is the root qdisc
		if t.qos.netemNs == nil {
			t.qos.tbfNs = []string{"root", "handle", "1:", "tbf"}
		} else {
			// Chain the tbf qdisc to the existing tbf disc
			t.qos.tbfNs = []string{"parent", "1:", "handle", "2:", "tbf"}
		}
		// Update the command parameters
		t.qos.params = t.qos.tbfParams

		return t.qos.tbfNs
	}
	return nil
}

// Execute a qos command
// Called from the qos cli handlers
// Op represents either add, change, or remove operations
// Qdisc is the qdisc class of the cli argument (netem, tbf)
func (t *Tap) qosCmd(op, qdisc string) error {

	// Build the namespace for the qdisc
	ns := t.qosNamespace(qdisc)

	// Get the command
	cmd := t.qosGetCmd(op, ns)

	// Execute the qos command
	out, err := processWrapper(cmd...)
	if err != nil {
		// Clean up
		err = errors.New(out)
		processWrapper(t.qosGetCmd("remove", nil)...)
	}

	return err
}

// Remove qos contraints from all taps
func qosRemoveAll() {
	for _, b := range bridges {
		for _, t := range b.Taps {
			if t.qos != nil {
				cmd := t.qosGetCmd("remove", nil)
				processWrapper(cmd...)
			}
		}
	}
}

// List all taps which are associated with a qos constraint
func qosList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "max_bandwidth", "loss", "delay"}
	resp.Tabular = [][]string{}

	for _, b := range bridges {
		for _, t := range b.Taps {
			if t.qos != nil {
				loss := t.qos.netemParams["loss"]
				delay := t.qos.netemParams["delay"]
				rate := t.qos.tbfParams["rate"]
				resp.Tabular = append(resp.Tabular, []string{
					b.Name, t.name, rate, loss, delay,
				})
			}
		}
	}
}
