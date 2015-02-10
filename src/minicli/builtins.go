// Copyright (2012) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"os"
	"strconv"
)

var builtinCLIHandlers = []Handler{
	{ // csv
		HelpShort: "enable or disable CSV mode",
		HelpLong: `
Enable or disable CSV mode. Enabling CSV mode disables JSON mode, if enabled.`,
		Patterns: []string{
			".csv [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliModeHelper(c, out, csvMode)
		},
	},
	{ // json
		HelpShort: "enable or disable JSON mode",
		HelpLong: `
Enable or disable JSON mode. Enabling JSON mode disables CSV mode, if enabled.`,
		Patterns: []string{
			".json [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliModeHelper(c, out, jsonMode)
		},
	},
	{ // header
		HelpShort: "enable or disable headers for tabular data",
		HelpLong: `
Enable or disable headers for tabular data.`,
		Patterns: []string{
			".headers [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliFlagHelper(c, out, &headers)
		},
	},
	{ // annotate
		HelpShort: "enable or disable hostname annotation",
		HelpLong: `
Enable or disable hostname annotation for responses.`,
		Patterns: []string{
			".annotate [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliFlagHelper(c, out, &annotate)
		},
	},
	{ // compress
		HelpShort: "enable or disable output compression",
		HelpLong: `
Enable or disable output compression of like output from multiple responses.
For example, if you executed a command using mesh, such as:

	mesh send node[0-9] version

You would expect to get the same minimega version for all 10 nodes. Rather than
print out the same version 10 times, minicli with compression enabled would print:

	node[0-9]: minimega <version>

Assuming that all the minimega instances are running the same version. If one node was running
a different version or has an error, compression is still useful:

	node[0-4,6-9]: minimega <version>
	node5: minimega <version>

Or,

	node[0-3,9]: minimega <version>
	node[4-8]: Error: <error>

Compression is not applied when the output mode is JSON.`,
		Patterns: []string{
			".compress [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliFlagHelper(c, out, &compress)
		},
	},
}

var hostname string

func init() {
	var err error

	for i := range builtinCLIHandlers {
		MustRegister(&builtinCLIHandlers[i])
	}

	hostname, err = os.Hostname()
	if err != nil {
		hostname = "???"
	}
}

func cliModeHelper(c *Command, out chan Responses, newMode int) {
	if c.BoolArgs["true"] {
		mode = newMode
	} else if c.BoolArgs["false"] {
		mode = defaultMode
	} else {
		resp := &Response{
			Host:     hostname,
			Response: strconv.FormatBool(mode == newMode),
		}
		out <- Responses{resp}
	}
}

func cliFlagHelper(c *Command, out chan Responses, flag *bool) {
	if c.BoolArgs["true"] || c.BoolArgs["false"] {
		// Update the flag, can just get value for "true" since the default
		// value is false.
		*flag = c.BoolArgs["true"]
	} else {
		resp := &Response{
			Host:     hostname,
			Response: strconv.FormatBool(*flag),
		}
		out <- Responses{resp}
	}
}
