package main

import (
	"fmt"
	"minicli"
	"strconv"
)

var qosCLIHandlers = []minicli.Handler{
	{
		HelpShort: "add qos constraints to an interface",
		HelpLong:  "Fritz Fritz Fritz",
		Patterns: []string{
			"qos list",
			"qos <add,> <interface> <loss,> <percent>",
			"qos <add,> <interface> <delay,> <ms>",
			"qos <add,> <interface> <loss,> <percent> <delay,> <ms>",
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
			delay := c.StringArgs["ms"]

			_, err := strconv.ParseUint(delay, 10, 64)
			if err != nil {
				resp.Error = fmt.Sprintf("`%s` is not a valid delay parameter", delay)
				return resp
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
