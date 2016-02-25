package main

import (
	"fmt"
	"minicli"
	"strconv"
	"time"
	"strings"
)

var qosCLIHandlers = []minicli.Handler{
	{
		HelpShort: "add qos constraints to an interface",
		HelpLong:  "add qos constraints to an interface",
		Patterns: []string{
			"qos <add,> <interface> <loss,> <percent>",
			"qos <add,> <interface> <delay,> <duration>",
			"qos <add,> <interface> <loss,> <percent> <delay,> <duration>",
			"qos <remove,> <interface>",
		}, Call: wrapSimpleCLI(cliQos),
	},
	{
		HelpShort: "list qos constraints on all interfaces",
		HelpLong:  "list qos constraints on all interfaces",
		Patterns: []string{
			"qos list",
		}, Call: wrapSimpleCLI(cliQosList),
	},
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

	bridgeLock.Lock()
	defer bridgeLock.Unlock()

	// Wildcard command
	if tap == "all" {
		if c.BoolArgs["remove"] {
			qosRemoveAll()
		} else {
			resp.Error = fmt.Sprintf("not implemented")
		}
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

	if c.BoolArgs["add"] {
		if t.qos == nil {
			t.qos = newQos()
		} else {
			t.qos.change = true
		}

		// Drop packets randomly with probability = loss
		if c.BoolArgs["loss"] {
			loss := c.StringArgs["percent"]

			_, err := strconv.ParseFloat(loss, 64)
			if err != nil {
				resp.Error = fmt.Sprintf("`%s` is not a valid loss percentage", loss)
				return resp
			}
			t.qos.params["loss"] = loss
		}

		if c.BoolArgs["delay"] {
			delay := c.StringArgs["duration"]
			_, err:= time.ParseDuration(delay)

			if err != nil {
				if strings.Contains(err.Error(), "time: missing unit in duration") {
					// Default to ms
					delay = fmt.Sprintf("%s%s", delay, "ms")
				} else {
					resp.Error = fmt.Sprintf("`%s` is not a valid delay parameter", delay)
					return resp
				}
			}
			t.qos.params["delay"] = delay
		}

		// Execute the qos command
		err := t.qosCmd()
		if err != nil {
			resp.Error = err.Error()
		}

	} else {
		// Remove command
		// Only remove qos from taps which had previous constraints
		if t.qos == nil {
			return resp
		} else {
			t.qos = nil
		}
		err := t.qosCmd()
		if err != nil {
			resp.Error = err.Error()
		}
	}

	return resp
}
