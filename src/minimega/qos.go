package main

import (
	"errors"
	"fmt"
	"minicli"
)

// Map of tap names to bridges
var qosTaps map[string]*Bridge

// Tap field enumerating qos parameters
type Qos struct {
	params map[string]string
}

func init() {
	qosTaps = make(map[string]*Bridge)
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
	// This also guards qosTaps modifications
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
		delete(qosTaps, t)
	}

	// Update the tap qos field
	tap.qos = qos
	b.Taps[t] = tap

	out, err := processWrapper(cmd...)
	if err != nil {
		return errors.New(out)
	}

	// Update qosTaps
	if qos != nil {
		qosTaps[t] = b
	} else {
		delete(qosTaps, t)
	}

	return nil
}

// Remove qos contraints from all taps
func qosRemoveAll() {
	for t, b := range qosTaps {
		b.Lock()
		cmd := qosRemoveCmd(t)
		processWrapper(cmd...)
		tap := b.Taps[t]
		tap.qos = nil
		b.Taps[t] = tap
		delete(qosTaps, t)
		b.Unlock()
	}
}

// List all taps which are associated with a qos constraint
func qosList(resp *minicli.Response) {
	resp.Header = []string{"bridge", "tap", "loss", "delay"}
	resp.Tabular = [][]string{}

	// Not thread safe
	for t, b := range qosTaps {

		// Get the qos params
		qos := b.Taps[t].qos
		loss := qos.params["loss"]
		delay := qos.params["delay"]

		resp.Tabular = append(resp.Tabular, []string{
			b.Name, t, loss, delay,
		})
	}
}
