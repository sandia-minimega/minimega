package main

import (
	"bridge"
	"errors"
	"fmt"
	"minicli"
	"strconv"
	"time"
)

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

Note: qos applies only to traffic received by the VM (which is "egress" traffic
on the mega_tap interface on the host) -- traffic sent by the VM ("ingress" on
the mega_tap interface on the host) is not policed to the desired rate.

Examples:

	Randomly drop packets on the 0th interface for vms foo0, 1, and 2 with
	probability 25%

	qos add foo[0-2] 0 loss 25

	Add a 100ms delay to every packet on the 0th interface for vm foo and bar

	qos add foo,bar 0 delay 100ms

	Rate limit the 0th interface on all vms in the active namespace to 1mbit/s

	qos add all 0 rate 1 mbit

To clear active qos settings, use:

	clear qos <vm> <interface|all>

Example:

	clear qos foo all`,
		Patterns: []string{
			"qos <add,> <vm target> <interface> <loss,> <percent>",
			"qos <add,> <vm target> <interface> <delay,> <duration>",
			"qos <add,> <vm target> <interface> <rate,> <bw> <kbit,mbit,gbit>",
		},
		Call:    wrapVMTargetCLI(cliUpdateQos),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
	},
	{
		HelpShort: "clear qos constraints on an interface",
		HelpLong: `
Remove QoS constraints from a VM's interface. To clear QoS from all interfaces
for a VM, use the wildcard:

	clear qos foo all

See "vm start" for a full description of allowable targets.`,
		Patterns: []string{
			"clear qos <vm target> [tap index]",
		},
		Call:    wrapVMTargetCLI(cliClearQos),
		Suggest: wrapVMSuggest(VM_ANY_STATE, true),
	},
}

func cliClearQos(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["vm"]
	index := c.StringArgs["tap"]

	tap, err := strconv.ParseUint(index, 10, 32)
	if err != nil && index != Wildcard {
		return fmt.Errorf("invalid tap index %s", index)
	}

	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		if index == Wildcard {
			return true, vm.ClearAllQos()
		}

		return true, vm.ClearQos(uint(tap))
	})
}

func cliUpdateQos(ns *Namespace, c *minicli.Command, resp *minicli.Response) error {
	target := c.StringArgs["vm"]

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

	return ns.VMs.Apply(target, func(vm VM, wild bool) (bool, error) {
		return true, vm.UpdateQos(uint(tap), op)
	})
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
		return op, unreachable()
	}

	return op, nil
}
