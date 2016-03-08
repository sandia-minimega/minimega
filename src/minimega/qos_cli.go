package main

import (
	"fmt"
	"minicli"
	"strconv"
	"strings"
	"time"
)

var qosCLIHandlers = []minicli.Handler{
	{
		HelpShort: "add qos constraints to an interface",
		HelpLong: `
Add quality-of-service (qos) constraints on mega interfaces to
simulate network impairments. Qos constrains cannot be stacked, and must
be specified explicitly. Any existing constraints will be overwritten by
additional calls to <add>.

Qos constraints include:

- loss		: packets will be randomly dropped with a specified probability
- delay		: delay packets for configured unit of time
- rate		: impose a maximum bandwidth on an interface

Examples:

	Randomly drop packets on mega_tap1 with probably 0.25%
	qos add mega_tap1 loss 0.25

	Add a 100ms delay to every packet on the mega_tap1 interface
	qos add mega_tap1 delay 100ms`,
		Patterns: []string{
			"qos <add,> <interface> <loss,> <percent>",
			"qos <add,> <interface> <delay,> <duration>",
			"qos <add,> <interface> <rate,> <bw>",
		}, Call: wrapSimpleCLI(cliQos),
	},
	{
		HelpShort: "list qos constraints on all interfaces",
		HelpLong: `
List quality-of-service constraints on all mega interfaces in tabular form.
Columns returned by qos list include:

- host		: the host the the VM is running on
- bridge	: bridge name
- tap		: tap name
- rate		: maximum bandwidth of the interface
- loss		: probability of dropping packets
- delay		: packet delay in milliseconds`,
		Patterns: []string{
			"qos list",
		}, Call: wrapSimpleCLI(cliQosList),
	},
	{
		HelpShort: "clear qos constraints on an interface",
		HelpLong: `
Remove quality-of-service constraints on a mega interface.

Example:

	Remove all qos constraints from mega_tap1
	clear qos mega_tap1`,
		Patterns: []string{
			"clear qos [interface]",
		}, Call: wrapSimpleCLI(cliQosClear),
	},
}

func cliQosClear(c *minicli.Command) *minicli.Response {

	resp := &minicli.Response{Host: hostname}
	tap := c.StringArgs["interface"]

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	if tap == "all" {
		qosRemoveAll()
	} else {
		// Get *Tap
		b, err := getBridgeFromTap(tap)
		if err != nil {
			resp.Error = err.Error()
			return resp
		}

		t, ok := b.Taps[tap]
		if !ok {
			resp.Error = fmt.Sprintf("qosCmd: tap %s not found", tap)
			return resp
		}

		// Only remove qos from taps which had previous constraints
		if t.qos == nil {
			return resp
		}
		err = t.qosCmd("remove", "")
		if err != nil {
			resp.Error = err.Error()
		}
	}
	return resp
}

func cliQosList(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	qosList(resp)
	return resp
}

func cliQos(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	tap := c.StringArgs["interface"]

	var qdisc, op string

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	// Wildcard command
	if tap == "all" {
		resp.Error = fmt.Sprintf("not implemented")
		return resp
	}

	// Get *Tap
	b, err := getBridgeFromTap(tap)
	if err != nil {
		resp.Error = err.Error()
		return resp
	}

	t, ok := b.Taps[tap]
	if !ok {
		resp.Error = fmt.Sprintf("qosCmd: tap %s not found", tap)
		return resp
	}

	if t.qos == nil {
		t.qos = newQos()
	}

	// Determine qdisc and operation
	if c.BoolArgs["rate"] {
		qdisc = "tbf"
		if len(t.qos.tbfParams) == 0 {
			op = "add"
		} else {
			op = "change"
		}
	} else {
		qdisc = "netem"
		if len(t.qos.netemParams) == 0 {
			op = "add"
		} else {
			op = "change"
		}
	}

	// Drop packets randomly with probability = loss
	if c.BoolArgs["loss"] {
		loss := c.StringArgs["percent"]

		_, err := strconv.ParseFloat(loss, 64)
		if err != nil {
			resp.Error = fmt.Sprintf("`%s` is not a valid loss percentage", loss)
			return resp
		}
		t.qos.netemParams["loss"] = loss
	}

	// Add delay of time duration to each packet
	if c.BoolArgs["delay"] {
		delay := c.StringArgs["duration"]
		_, err := time.ParseDuration(delay)

		if err != nil {
			if strings.Contains(err.Error(), "time: missing unit in duration") {
				// Default to ms
				delay = fmt.Sprintf("%s%s", delay, "ms")
			} else {
				resp.Error = fmt.Sprintf("`%s` is not a valid delay parameter", delay)
				return resp
			}
		}
		t.qos.netemParams["delay"] = delay
	}

	// Add a bandwidth limit on the interface using the token bucket filter (tbf) qdisc
	if c.BoolArgs["rate"] {
		rate := c.StringArgs["bw"]

		// TODO - Update parameters. Using defaults right now
		t.qos.tbfParams["rate"] = rate
		t.qos.tbfParams["burst"] = "32kbit"
		t.qos.tbfParams["latency"] = "5ms"

	}
	// Execute the qos command
	err = t.qosCmd(op, qdisc)
	if err != nil {
		resp.Error = err.Error()
	}

	return resp
}
