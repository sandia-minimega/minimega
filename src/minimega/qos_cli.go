package main

import (
	"errors"
	"fmt"
	"minicli"
	log "minilog"
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
			"qos <add,> <target> <interface> <rate,> <bw>",
		}, Call: wrapVMTargetCLI(cliQos),
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
		}, Call: wrapVMTargetCLI(cliQosList),
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
		}, Call: wrapVMTargetCLI(cliQosClear),
	},
}

func cliQosClear(c *minicli.Command, resp *minicli.Response) error {
	vmLock.Lock()
	defer vmLock.Unlock()

	tap := c.StringArgs["interface"]
	target := c.StringArgs["target"]

	applyFunc := func(vm VM, _ bool) (bool, error) {
		if tap == Wildcard {
			var seen map[string]bool

			// Remove qos from all taps
			for _, b := range bridges {
				if !seen[b.Name] {
					b.QosClearAll()
					seen[b.Name] = true
				}
			}

			return true, nil
		} else {

			bridgeName := vm.Config().GetBridgeFromTap(tap)

			if bridgeName == "" {
				return true, fmt.Errorf("interface %s on vm %s not found", tap, vm.GetName())
			}

			b, err := getBridge(bridgeName)

			if err != nil {
				return true, err
			}

			return true, b.QosClear(tap)
		}
	}
	return makeErrSlice(vms.apply(target, true, applyFunc))
}

func cliQosList(c *minicli.Command, resp *minicli.Response) error {
	var nsBridges map[string]bool

	resp.Header = []string{"bridge", "tap", "max_bandwidth", "loss", "delay"}
	resp.Tabular = [][]string{}

	// Build a list of bridges used in the current namespace
	for _, vm := range vms {
		if !inNamespace(vm) {
			continue
		}

		for _, nc := range vm.Config().Networks {
			if nsBridges[nc.Bridge] {
				continue
			}
			nsBridges[nc.Bridge] = true
		}
	}

	// Iterate over all bridges and collect qos information
	for nsBridge, _ := range nsBridges {
		b, err := getBridge(nsBridge)

		if err != nil {
			log.Error("qosList: failed to get bridge %s", nsBridge)
			continue
		}

		for _, v := range b.QosList() {
			resp.Tabular = append(resp.Tabular, v)
		}
	}
	return nil
}

func cliQos(c *minicli.Command, resp *minicli.Response) error {
	vmLock.Lock()
	defer vmLock.Unlock()

	var params map[string]string

	target := c.StringArgs["target"]
	tap := c.StringArgs["interface"]

	// Wildcard command
	if tap == Wildcard {
		return errors.New("wildcard qos not implemented")
	}

	params["tap"] = tap

	// Determine qdisc and set the parameters
	if c.BoolArgs["rate"] {

		// token bucket filter (tbf) qdisc operation
		params["qdisc"] = "tbf"

		// Add a bandwidth limit on the interface
		rate := c.StringArgs["bw"]
		unit := rate[len(rate)-4:]
		var bps uint64
		switch unit {
		case "kbit":
			bps = 1 << 10
		case "mbit":
			bps = 1 << 20
		case "gbit":
			bps = 1 << 30
		default:
			return fmt.Errorf("`%s` invalid: must specify rate as <kbit, mbit, or gbit>", rate)
		}

		_, err := strconv.ParseUint(rate[:len(rate)-4], 10, 64)
		if err != nil {
			return fmt.Errorf("`%s` is not a valid rate parameter", rate)
		}

		params["burst"] = rate[:len(rate)-4]
		params["bps"] = strconv.FormatUint(bps, 10)
		params["rate"] = rate

	} else {

		// netem qdisc operation
		params["qdisc"] = "netem"

		// Drop packets randomly with probability = loss
		if c.BoolArgs["loss"] {
			loss := c.StringArgs["percent"]
			v, err := strconv.ParseFloat(loss, 64)
			if err != nil || v >= float64(100) {
				return fmt.Errorf("`%s` is not a valid loss percentage", loss)
			}
			params["loss"] = loss
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
					return fmt.Errorf("`%s` is not a valid delay parameter", delay)
				}
			}
			params["delay"] = delay
		}
	}

	applyFunc := func(vm VM, _ bool) (bool, error) {
		// Get bridge
		bridgeName := vm.GetBridgeFromTap(tap)

		if bridgeName == "" {
			return true, fmt.Errorf("interface %s on vm %s not found", tap, vm.GetName())
		}

		b, err := getBridge(bridgeName)

		if err != nil {
			return true, err
		}

		// Execute the qos command
		return true, b.QosCommand(params)
	}

	return makeErrSlice(vms.apply(target, true, applyFunc))
}
