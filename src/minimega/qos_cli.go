package main

import (
	"bridge"
	"errors"
	"fmt"
	"minicli"
	"strconv"
	"strings"
	"time"
)

var qosInfo = []string{"bridge", "tap", "max_bandwidth", "loss", "delay"}

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
- delay		: delay packets for specified unit of time (ms, ns, etc)
- rate		: impose a maximum bandwidth on an interface, in kbit, mbit, or gbit

Examples:

	Randomly drop packets on mega_tap1 with probably 0.25%
	qos add mega_tap1 loss 0.25

	Add a 100ms delay to every packet on the mega_tap1 interface
	qos add mega_tap1 delay 100ms

	Rate limit an interface to 1mbit/s
	qos add mega_tap1 rate 1mbit`,
		Patterns: []string{
			"qos <add,> <target> <interface> <loss,> <percent>",
			"qos <add,> <target> <interface> <delay,> <duration>",
			"qos <add,> <target> <interface> <rate,> <kbit, mbit, gbit>",
		}, Call: wrapVMTargetCLI(cliUpdateQos),
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
		}, Call: wrapVMTargetCLI(cliListQos),
	},
	{
		HelpShort: "clear qos constraints on an interface",
		HelpLong: `
Remove quality-of-service constraints on a mega interface.

Example:

	Remove all qos constraints from mega_tap1
	clear qos mega_tap1`,
		Patterns: []string{
			"clear qos <target> [interface]",
		}, Call: wrapVMTargetCLI(cliClearQos),
	},
}

func cliClearQos(c *minicli.Command, resp *minicli.Response) error {

	target := c.StringArgs["target"]

	tap, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		return errors.New("specify a valid tap index")
	}

	return makeErrSlice(vms.ClearQoS(target, tap))
}

func cliListQos(c *minicli.Command, resp *minicli.Response) error {
	vms.ListQoS(resp)
	return nil
}

func cliUpdateQos(c *minicli.Command, resp *minicli.Response) error {

	var err error
	target := c.StringArgs["target"]

	// Wildcard command
	if c.StringArgs["tap"] == Wildcard {
		return errors.New("wildcard qos not implemented")
	}

	tap, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		return errors.New("specify a valid tap index")
	}

	// Build qos parameters
	qosp, err := cliParseQos(c)
	if err != nil {
		return err
	}

	return makeErrSlice(vms.UpdateQos(target, tap, qosp))

}

func cliParseQos(c *minicli.Command) (*bridge.QosParams, error) {

	qosp := &bridge.QosParams{}

	// Determine qdisc and set the parameters
	if c.BoolArgs["rate"] {
		qosp.Qdisc = "tbf"

		var unit string
		rate := c.StringArgs["bw"]

		if c.BoolArgs["kbit"] {
			qosp.Bps = 1 << 10
			unit = "kbit"
		} else if c.BoolArgs["mbit"] {
			qosp.Bps = 1 << 20
			unit = "mbit"
		} else if c.BoolArgs["gbit"] {
			qosp.Bps = 1 << 30
			unit = "gbit"
		} else {
			return nil, fmt.Errorf("`%s` invalid: must specify rate as <kbit, mbit, or gbit>", rate)
		}

		burst, err := strconv.ParseUint(rate, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("`%s` is not a valid rate parameter", rate)
		}

		qosp.Burst = burst
		qosp.Rate = fmt.Sprintf("%s%s", rate, unit)

	} else {
		qosp.Qdisc = "netem"

		// Drop packets randomly with probability = loss
		if c.BoolArgs["loss"] {
			loss := c.StringArgs["percent"]
			v, err := strconv.ParseFloat(loss, 64)
			if err != nil || v >= float64(100) {
				return nil, fmt.Errorf("`%s` is not a valid loss percentage", loss)
			}
			qosp.Loss = loss
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
					return nil, fmt.Errorf("`%s` is not a valid delay parameter", delay)
				}
			}
			qosp.Delay = delay
		}
	}
	return qosp, nil
}
