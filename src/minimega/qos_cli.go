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

var qosCLIHandlers = []minicli.Handler{
	{
		HelpShort: "add qos constraints to an interface",
		HelpLong: `
Add quality-of-service (qos) constraints on mega interfaces to
simulate network impairments. Qos constraints cannot be stacked, and must
be specified explicitly. Any existing constraints will be overwritten by
additional calls to <add>. Virtual machines can be specified with the same
target syntax as the "vm start" api.

Note that qos is namespace aware, and specifying the wildcard as the target
will apply qos to all virtual machines within the active namespace.

Qos constraints include:

- loss		: packets will be randomly dropped with a specified probability
- delay		: delay packets for specified unit of time (ms, ns, etc)
- rate		: impose a maximum bandwidth on an interface, in kbit, mbit, or gbit

Examples:

	Randomly drop packets on the 0th interface for vms foo0, 1, and 2 with probably 0.25%
	qos add foo[0-2] 0 loss 0.25

	Add a 100ms delay to every packet on the 0th interface for vm foo and bar
	qos add foo,bar 0 delay 100ms

	Rate limit the 0th interface on all vms in the active namespace to 1mbit/s
	qos add all 0 rate 1 mbit`,
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
This command is namespace aware and will only list the qos constraints within
the active namespace.
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
Remove quality-of-service constraints on a mega interface. This command is namespace
aware and will only clear the qos from vms within the active namespace.

Example:

	Remove all qos constraints on the 1st interface for the vms foo and bar
	clear qos foo,bar 1`,
		Patterns: []string{
			"clear qos <target> [interface]",
		}, Call: wrapVMTargetCLI(cliClearQos),
	},
}

func cliClearQos(c *minicli.Command, resp *minicli.Response) error {

	target := c.StringArgs["target"]

	if tap == Wildcard {
		vms.ClearAllQos()
		return nil
	}

	tap, err := strconv.Atoi(c.StringArgs["tap"])
	if err != nil {
		return errors.New("invalid tap index %s", c.StringArgs["tap"])
	}

	if tap < 0 {
		return errors.New("invalid tap index %d", tap)
	}

	return makeErrSlice(vms.ClearQoS(target, t))
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
		return errors.New("invalid tap index %s", c.StringArgs["tap"])
	}

	if tap < 0 {
		return errors.New("invalid tap index %d", tap)
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
