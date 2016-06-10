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

// Used to calulate burst rate for the token bucket filter qdisc
const KERNEL_TIMER_FREQ uint64 = 250
const MIN_BURST_SIZE uint64 = 2048

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
			"qos <add,> <target> <interface> <rate,> <kbit,mbit,gbit>",
		}, Call: wrapVMTargetCLI(cliUpdateQos),
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

	if c.StringArgs["tap"] == Wildcard {
		return fmt.Errorf("qos for wildcard taps not supported")
	}

	tap, err := strconv.ParseUint(c.StringArgs["tap"], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid tap index %s", c.StringArgs["tap"])
	}

	return makeErrSlice(vms.ClearQoS(target, uint(tap)))
}

func cliUpdateQos(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	// Wildcard command
	if c.StringArgs["tap"] == Wildcard {
		return errors.New("wildcard qos not implemented")
	}

	tap, err := strconv.ParseUint(c.StringArgs["tap"], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid tap index %s", c.StringArgs["tap"])
	}

	// Build qos parameters
	qos, err := cliParseQos(c)
	if err != nil {
		return err
	}

	return makeErrSlice(vms.UpdateQos(target, uint(tap), qos))
}

func cliParseQos(c *minicli.Command) (*bridge.Qos, error) {
	qos := &bridge.Qos{}

	// Determine qdisc and set the parameters
	if c.BoolArgs["rate"] {

		qos.TbfParams = &bridge.TbfParams{}

		var unit string
		var bps uint64
		rate := c.StringArgs["bw"]

		if c.BoolArgs["kbit"] {
			bps = 1 << 10
			unit = "kbit"
		} else if c.BoolArgs["mbit"] {
			bps = 1 << 20
			unit = "mbit"
		} else if c.BoolArgs["gbit"] {
			bps = 1 << 30
			unit = "gbit"
		} else {
			return nil, fmt.Errorf("`%s` invalid: must specify rate as <kbit, mbit, or gbit>", rate)
		}

		burst, err := strconv.ParseUint(rate, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("`%s` is not a valid rate parameter", rate)
		}

		// Burst size is in bytes
		burst = ((burst * bps) / KERNEL_TIMER_FREQ) / 8
		if burst < MIN_BURST_SIZE {
			burst = MIN_BURST_SIZE
		}

		qos.Burst = fmt.Sprintf("%db", burst)
		qos.Rate = fmt.Sprintf("%s%s", rate, unit)

	} else {
		qos.NetemParams = &bridge.NetemParams{}

		// Drop packets randomly with probability = loss
		if c.BoolArgs["loss"] {
			loss := c.StringArgs["percent"]
			v, err := strconv.ParseFloat(loss, 64)
			if err != nil || v >= float64(100) || v < 0 {
				return nil, fmt.Errorf("`%s` is not a valid loss percentage", loss)
			}
			qos.Loss = loss
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
			qos.Delay = delay
		}
	}
	return qos, nil
}
