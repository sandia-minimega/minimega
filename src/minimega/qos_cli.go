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
		HelpLong:  "Fritz Fritz Fritz",
		Patterns: []string{
			"qos list",
			"qos <add,> <interface> <loss,> <percent>",
			"qos <add,> <interface> <delay,> <duration>",
			"qos <add,> <interface> <loss,> <percent> <delay,> <duration>",
			"qos <remove,> <interface>",
		}, Call: wrapSimpleCLI(cliQos),
	},
}

func cliQos(c *minicli.Command) *minicli.Response {
	resp := &minicli.Response{Host: hostname}
	qos := newQos()
	tapName := c.StringArgs["interface"]

	if c.BoolArgs["add"] {
		// Drop packets randomly with probability = loss
		if c.BoolArgs["loss"] {
			loss := c.StringArgs["percent"]

			_, err := strconv.ParseFloat(loss, 64)
			if err != nil {
				resp.Error = fmt.Sprintf("`%s` is not a valid loss percentage", loss)
				return resp
			}
			qos.params["loss"] = loss
		}

		if c.BoolArgs["delay"] {
			delay := c.StringArgs["duration"]
			_, err:= time.ParseDuration(delay)

			if err != nil {
				if strings.Contains(err.Error(), "time: missing unit in duration") {
					// Default to ms
					delay = fmt.Sprintf("%s%s", delay, "ms")
				} else {
					resp.Error = fmt.Sprintf("`%s` is not a valid delay parameter", c.StringArgs["duration"])
					return resp
				}
			}
			qos.params["delay"] = delay
		}

		// Execute the qos command
		err := qosCmd(qos, tapName)
		if err != nil {
			resp.Error = err.Error()
		}

	} else if c.BoolArgs["remove"] {
		// Remove command
		if tapName != "all" {
			err := qosCmd(nil, tapName)
			if err != nil {
				resp.Error = err.Error()
			}
		} else {
			// Remove all qos
			qosRemoveAll()
		}
	} else {
		// List command
		qosList(resp)
	}

	return resp
}
