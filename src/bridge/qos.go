package bridge

import (
	"errors"
	"fmt"
	log "minilog"
)

// Used to calulate burst rate for the token bucket filter
const KERNEL_TIMER_FREQ uint64 = 250
const MIN_BURST_SIZE uint64 = 2048

type QosParams struct {
	Qdisc string
	Loss  string
	Delay string
	Rate  string
	Bps   uint64
	Burst uint64
}

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

func (b *Bridge) ClearQos(tap string) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}

	return b.clearQos(t)
}

func (b *Bridge) clearQos(t *Tap) error {
	return t.qosCmd("remove", "")
}

func (b *Bridge) UpdateQos(tap string, qosp *QosParams) error {
	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	log.Info("updating qos for tap %s", tap)

	t, ok := b.taps[tap]
	if !ok {
		return fmt.Errorf("tap %s not found", tap)
	}

	return b.updateQos(t, qosp)
}

func (b *Bridge) updateQos(t *Tap, qosp *QosParams) error {
	var op string

	if t.Qos == nil {
		t.Qos = newQos()
	}

	if qosp.Qdisc == "tbf" {
		// token bucket filter (tbf) qdisc operation
		if len(t.Qos.tbfParams) == 0 {
			op = "add"
		} else {
			op = "change"
		}

		// Burst size is in bytes
		burst := ((qosp.Burst * qosp.Bps) / KERNEL_TIMER_FREQ) / 8
		if burst < MIN_BURST_SIZE {
			burst = MIN_BURST_SIZE
		}

		// Default parameters
		// Burst must be at least rate / hz
		// See http://unix.stackexchange.com/questions/100785/bucket-size-in-tbf
		t.Qos.tbfParams["rate"] = qosp.Rate
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
		if qosp.Loss != "" {
			t.Qos.netemParams["loss"] = qosp.Loss
		}

		// Add delay of time duration to each packet
		if qosp.Delay != "" {
			t.Qos.netemParams["delay"] = qosp.Delay
		}
	}

	// Execute the qos command
	return t.qosCmd(op, qosp.Qdisc)
}

func (b *Bridge) GetQos(tap string) *QosParams {
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

func (b *Bridge) getQos(t *Tap) *QosParams {
	q := &QosParams{}

	if t.Qos.netemParams != nil {
		q.Loss = t.Qos.netemParams["loss"]
		q.Delay = t.Qos.netemParams["delay"]
	}

	if t.Qos.tbfParams != nil {
		q.Rate = t.Qos.tbfParams["rate"]
	}

	return q
}
