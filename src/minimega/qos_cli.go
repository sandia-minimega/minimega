package main

import (
	"bridge"
	"errors"
	"fmt"
	"minicli"
	"strconv"
	"time"
)

// Used to calulate burst rate for the token bucket filter qdisc
const KERNEL_TIMER_FREQ uint64 = 250
const MIN_BURST_SIZE uint64 = 2048

var qosCLIHandlers = []minicli.Handler{
	{
		HelpShort: "add qos constraints to an interface",
		HelpLong: `
Add quality-of-service (qos) constraints on mega interfaces to emulate real
networks. Currently only applies qos constraints on the egress side / transmit
direction. Qos constraints can be stacked with multiple calls to <add>, and
must be specified explicitly. Any existing constraints will be overwritten by
additional calls to <add>. VM can be specified with the same target syntax as
the "vm start" api.

Note that qos is namespace aware, and any qos commands will be matched to
target vms within the currently active namespace.

qos constraints include:

- loss		: packets will be randomly dropped with a specified probability
- delay		: delay packets for specified unit of time (ms, ns, etc)
- rate		: impose a maximum bandwidth on an interface in kbit, mbit, or gbit

Note: due to limitations of the underlying tool, "tc", you can only add rate or
loss/delay to a VM. Enabling loss or delay will disable rate and vice versa.

Note: qos applies only to "egress" traffic -- "ingress" traffic is not policed
to the desired rate.

Examples:

	Randomly drop packets on the 0th interface for vms foo0, 1, and 2 with
	probably 0.25%

	qos add foo[0-2] 0 loss 0.25

	Add a 100ms delay to every packet on the 0th interface for vm foo and bar

	qos add foo,bar 0 delay 100ms

	Rate limit the 0th interface on all vms in the active namespace to 1mbit/s

	qos add all 0 rate 1 mbit`,
		Patterns: []string{
			"qos <add,> <target> <interface> <loss,> <percent>",
			"qos <add,> <target> <interface> <delay,> <duration>",
			"qos <add,> <target> <interface> <rate,> <bw> <kbit,mbit,gbit>",
		}, Call: wrapVMTargetCLI(cliUpdateQos),
	},
	{
		HelpShort: "clear qos constraints on an interface",
		HelpLong: `
Remove quality-of-service constraints on a mega interface. This command is
namespace aware and will only clear the qos from vms within the active
namespace.

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

	if c.StringArgs["interface"] == Wildcard {
		return fmt.Errorf("qos for wildcard taps not supported")
	}

	tap, err := strconv.ParseUint(c.StringArgs["interface"], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid tap index %s", c.StringArgs["interface"])
	}

	return makeErrSlice(vms.ClearQoS(target, uint(tap)))
}

func cliUpdateQos(c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["target"]

	// Wildcard command
	if c.StringArgs["interface"] == Wildcard {
		return errors.New("wildcard qos not implemented")
	}

	tap, err := strconv.ParseUint(c.StringArgs["interface"], 10, 32)
	if err != nil {
		return fmt.Errorf("invalid tap index %s", c.StringArgs["interface"])
	}

	// Build qos options
	op, err := cliParseQos(c)
	if err != nil {
		return err
	}

	return makeErrSlice(vms.UpdateQos(target, uint(tap), op))
}

func cliParseQos(c *minicli.Command) (bridge.QosOption, error) {
	op := bridge.QosOption{}

	if c.BoolArgs["rate"] {
		op.Type = bridge.Rate

		var unit string
		for _, v := range []string{"kbit", "mbit", "gbit"} {
			if c.BoolArgs[v] {
				unit = v
			}
		}

		rate := c.StringArgs["bw"]

		_, err := strconv.ParseUint(rate, 10, 64)
		if err != nil {
			return op, fmt.Errorf("invalid rate: `%v`", rate)
		}

		op.Value = rate + unit
	} else if c.BoolArgs["loss"] {
		op.Type = bridge.Loss

		loss := c.StringArgs["percent"]

		v, err := strconv.ParseFloat(loss, 64)
		if err != nil || v >= float64(100) || v < 0 {
			return op, fmt.Errorf("invalid loss: `%v`", loss)
		}

		op.Value = loss
	} else if c.BoolArgs["delay"] {
		op.Type = bridge.Delay

		delay := c.StringArgs["duration"]

		v, err := time.ParseDuration(delay)
		if err != nil {
			v2, err := time.ParseDuration(delay + "ms")
			if err != nil {
				return op, fmt.Errorf("invalid duration: `%v`", delay)
			}

			v = v2
			delay += "ms"
		}

		if v < 0 {
			return op, errors.New("delay cannot be negative")
		}

		op.Value = delay
	} else {
		return op, errors.New("unreachable")
	}

	return op, nil
}
