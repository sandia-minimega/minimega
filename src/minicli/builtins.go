// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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
	{ // sort
		HelpShort: "enable or disable sorting of tabular data",
		HelpLong: `
Enable or disable sorting of tabular data based on the value in the first
column. Sorting is based on string comparison.`,
		Patterns: []string{
			".sort [true,false]",
		},
		Call: func(c *Command, out chan Responses) {
			cliFlagHelper(c, out, &sortRows)
		},
	},
	{ // filter
		HelpShort: "filter tabular data by column value",
		HelpLong: `
Filters tabular data based on the value in a particular column. For example, to
search for vms in a particular state:

	.filter state=running vm info

Filters are case insensitive and may be stacked:

	.filter state=running .filter vcpus=4 vm info`,
		Patterns: []string{
			".filter <column=value> (command)",
		},
		Call: cliFilter,
	},
	{ // columns
		HelpShort: "show certain columns from tabular data",
		HelpLong: `
Filter tabular data using particular column names. For example, to only display
only the vm ID and state:

	.columns id,state vm info

Column names are comma-seperated. .columns can be used in conjunction with
.filter to slice a subset of the rows and columns from a command, however,
these commands are not always interchangeable. For example, the following is
acceptable:

	.columns id,state .filter vcpus=4 vm info

While the following is not:

	.filter vcpus=4 .columns id,state vm info

This is because .columns strips all columns except for ID and state from the
tabular data.

Note: the annotate flag controls the presence of the host column.`,
		Patterns: []string{
			".columns <columns as csv> (command)",
		},
		Call: cliColumns,
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

func cliFilter(c *Command, out chan Responses) {
	parts := strings.Split(c.StringArgs["column=value"], "=")
	if len(parts) != 2 {
		resp := &Response{
			Host:  hostname,
			Error: "malformed filter term, expected column=value",
		}
		out <- Responses{resp}
		return
	}

	col, filter := strings.ToLower(parts[0]), strings.ToLower(parts[1])

	pipe := make(chan Responses)
	go func() {
		c.Subcommand.Call(c.Subcommand, pipe)
		close(pipe)
	}()

outer:
	for resps := range pipe {
		newResps := Responses{}

		for _, r := range resps {
			// HAX: Special case for when the column name is host which is not
			// part of the actual tabular data.
			if col == "host" && r.Host != filter {
				continue
			} else if r.Header != nil && r.Tabular != nil {
				var found bool

				for j, header := range r.Header {
					// Found right column, check whether filter matches
					if strings.ToLower(header) == col {
						tabular := [][]string{}

						for _, row := range r.Tabular {
							if strings.ToLower(row[j]) == filter {
								tabular = append(tabular, row)
							}
						}

						r.Tabular = tabular
						found = true
						break
					}
				}

				// Didn't find the requested column in the responses
				if !found {
					resp := &Response{
						Host:  hostname,
						Error: fmt.Sprintf("no such column `%s`", col),
					}
					out <- Responses{resp}
					continue outer
				}
			}

			newResps = append(newResps, r)
		}

		out <- newResps
	}
}

func cliColumns(c *Command, out chan Responses) {
	columns := strings.Split(c.StringArgs["columns"], ",")

	pipe := make(chan Responses)
	go func() {
		c.Subcommand.Call(c.Subcommand, pipe)
		close(pipe)
	}()

outer:
	for resps := range pipe {
		for _, r := range resps {
			if r.Header == nil || r.Tabular == nil {
				continue
			}

			// Rebuild tabular data with specified columns
			tabular := make([][]string, len(r.Tabular))
			for _, col := range columns {
				var found bool

				for j, header := range r.Header {
					// Found right column, copy the tabular data
					if header == col {
						for k, row := range r.Tabular {
							tabular[k] = append(tabular[k], row[j])
						}

						found = true
						break
					}
				}

				// Didn't find the requested column in the responses
				if !found {
					resp := &Response{
						Host:  hostname,
						Error: fmt.Sprintf("no such column `%s`", col),
					}
					out <- Responses{resp}
					continue outer
				}
			}

			r.Tabular = tabular
			r.Header = columns
		}

		out <- resps
	}
}

func cliModeHelper(c *Command, out chan Responses, newMode int) {
	resp := &Response{
		Host: hostname,
	}

	if c.BoolArgs["true"] {
		mode = newMode
	} else if c.BoolArgs["false"] {
		mode = defaultMode
	} else {
		resp.Response = strconv.FormatBool(mode == newMode)
	}

	out <- Responses{resp}
}

func cliFlagHelper(c *Command, out chan Responses, flag *bool) {
	resp := &Response{
		Host: hostname,
	}

	if c.BoolArgs["true"] || c.BoolArgs["false"] {
		// Update the flag, can just get value for "true" since the default
		// value is false.
		*flag = c.BoolArgs["true"]
	} else {
		resp.Response = strconv.FormatBool(*flag)
	}

	out <- Responses{resp}
}
