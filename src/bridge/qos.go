package bridge

import (
	"errors"
	"fmt"
	log "minilog"
	"strconv"
)

// Used to calulate burst rate for the token bucket filter
const KERNEL_TIMER_FREQ uint64 = 250
const MIN_BURST_SIZE uint64 = 2048

// Tap field enumerating qos parameters
type Qos struct {
	// current command parameters
	params map[string]string

	// tbf queue discipline
	tbfNs     []string
	tbfParams map[string]string

	// netem queue discipline
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
		t.Qos = nil
		return []string{"tc", "qdisc", "del", "dev", t.Name, "root"}
	}

	// Base cmd
	cmd := []string{"tc", "qdisc", op, "dev", t.Name}

	// Add qdisc namespace
	for _, n := range ns {
		cmd = append(cmd, n)
	}

	// Add tc constraint parameters
	for p, v := range t.Qos.params {
		cmd = append(cmd, p, v)
	}
	return cmd
}

// Given a tc qdisc (netem, tbf) generate the correct namespace for a tc command.
// This is required because in order to have both netem and tbf qdiscs they
// must be "chained" together.
func (t *Tap) qosNamespace(qdisc string) []string {
	var ns []string

	if qdisc == "netem" {
		// This is the root qdisc
		if t.Qos.tbfNs == nil {
			t.Qos.netemNs = []string{"root", "handle", "1:", "netem"}
			ns = t.Qos.netemNs
		} else {
			// Chain the netem qdisc to the existing tbf qdisc
			ns = []string{"parent", "1:", "handle", "2:", "netem"}
		}
		// Update the command parameters
		t.Qos.params = t.Qos.netemParams
	}

	if qdisc == "tbf" {
		// This is the root qdisc
		if t.Qos.netemNs == nil {
			t.Qos.tbfNs = []string{"root", "handle", "1:", "tbf"}
			ns = t.Qos.tbfNs
		} else {
			// Chain the tbf qdisc to the existing tbf disc
			ns = []string{"parent", "1:", "handle", "2:", "tbf"}
		}
		// Update the command parameters
		t.Qos.params = t.Qos.tbfParams
	}
	return ns
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

func (b *Bridge) QosClearAll() {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	for _, t := range b.taps {
		if t.Qos != nil {
			err := t.qosCmd("remove", "")
			if err != nil {
				log.Error("failed to remove qos from tap %s", t.Name)
			}
		}
	}
}

func (b *Bridge) QosClear(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("QosClear: tap %s not found", tap)
	}

	err := t.qosCmd("remove", "")
	if err != nil {
		return err
	}
	return nil
}

func (b *Bridge) QosList() [][]string {
	var resp [][]string
	for _, t := range b.taps {
		if t.Qos != nil {
			loss := t.Qos.netemParams["loss"]
			delay := t.Qos.netemParams["delay"]
			rate := t.Qos.tbfParams["rate"]
			resp = append(resp, []string{
				b.Name, t.Name, rate, loss, delay,
			})
		}
	}
	return resp
}

func (b *Bridge) QosCommand(params map[string]string) error {

	var op string

	t, ok := b.taps[params["tap"]]
	if !ok {
		return fmt.Errorf("QosCommand: tap %s not found", params["tap"])
	}

	if t.Qos == nil {
		t.Qos = newQos()
	}

	// Determine qdisc and operation
	if params["qdisc"] == "tbf" {

		// token bucket filter (tbf) qdisc operation
		if len(t.Qos.tbfParams) == 0 {
			op = "add"
		} else {
			op = "change"
		}

		bps, _ := strconv.ParseUint(params["bps"], 10, 64)
		burst, _ := strconv.ParseUint(params["burst"], 10, 64)

		// Burst size is in bytes
		burst = ((burst * bps) / KERNEL_TIMER_FREQ) / 8
		if burst < MIN_BURST_SIZE {
			burst = MIN_BURST_SIZE
		}

		// Default parameters
		// Burst must be at least rate / hz
		// See http://unix.stackexchange.com/questions/100785/bucket-size-in-tbf
		t.Qos.tbfParams["rate"] = params["rate"]
		t.Qos.tbfParams["burst"] = fmt.Sprintf("%db", burst)
		t.Qos.tbfParams["latency"] = "100ms"

	} else {

		// netem qdisc operation
		if len(t.Qos.netemParams) == 0 {
			op = "add"
		} else {
			op = "change"
		}

		// Drop packets randomly with probability = loss
		if loss, ok := params["loss"]; ok {
			t.Qos.netemParams["loss"] = loss
		}

		// Add delay of time duration to each packet
		if delay, ok := params["delay"]; ok {
			t.Qos.netemParams["delay"] = delay
		}
	}

	// Execute the qos command
	return t.qosCmd(op, params["qdisc"])
}
