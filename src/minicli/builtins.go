// Copyright (2015) Sandia Corporation.
// Under the terms of Contract DE-AC04-94AL85000 with Sandia Corporation,
// the U.S. Government retains certain rights in this software.

package minicli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type filter struct {
	Negate   bool
	Fuzzy    bool
	Col, Val string
}

var builtinCLIHandlers = []Handler{
	{ // csv
		HelpShort: "enable or disable CSV mode",
		HelpLong: `
Enable or disable CSV mode. Enabling CSV mode disables JSON mode, if enabled.`,
		Patterns: []string{
			".csv [true,false]",
			".csv <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliModeHelper(c, out, csvMode)
		},
	},
	{ // json
		HelpShort: "enable or disable JSON mode",
		HelpLong: `
Enable or disable JSON mode. Enabling JSON mode disables CSV mode, if enabled.`,
		Patterns: []string{
			".json [true,false]",
			".json <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliModeHelper(c, out, jsonMode)
		},
	},
	{ // headers
		HelpShort: "enable or disable headers for tabular data",
		HelpLong: `
Enable or disable headers for tabular data.`,
		Patterns: []string{
			".headers [true,false]",
			".headers <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliFlagHelper(c, out, func(f *Flags) *bool { return &f.Headers })
		},
	},
	{ // annotate
		HelpShort: "enable or disable hostname annotation",
		HelpLong: `
Enable or disable hostname annotation for responses.`,
		Patterns: []string{
			".annotate [true,false]",
			".annotate <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliFlagHelper(c, out, func(f *Flags) *bool { return &f.Annotate })
		},
	},
	{ // sort
		HelpShort: "enable or disable sorting of tabular data",
		HelpLong: `
Enable or disable sorting of tabular data based on the value in the first
column. Sorting is based on string comparison.`,
		Patterns: []string{
			".sort [true,false]",
			".sort <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliFlagHelper(c, out, func(f *Flags) *bool { return &f.Sort })
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
			".compress <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			cliFlagHelper(c, out, func(f *Flags) *bool { return &f.Compress })
		},
	},
	{ // filter
		HelpShort: "filter tabular data by column value",
		HelpLong: `
Filters tabular data based on the value in a particular column. For example, to
search for vms in a particular state:

	.filter state=running vm info

Filters can also be inverted:

	.filter state!=running vm info

Filters are case insensitive and may be stacked:

	.filter state=RUNNING .filter vcpus=4 vm info

If the column value is a list or an object (i.e. "[...]", "{...}"), then
.filter implicitly uses substring matching.

Substring matching can be specified explicity:

	.filter state~run vm info
	.filter state!~run vm info`,
		Patterns: []string{
			".filter <filter> (command)",
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
	{ // record
		HelpShort: "enable or disable history recording",
		HelpLong: `
Enable or disable the recording of a given command in the command history.`,
		Patterns: []string{
			".record [true,false]",
			".record <true,false> (command)",
		},
		Call: func(c *Command, out chan<- Responses) {
			if c.Subcommand != nil {
				c.Record = c.BoolArgs["true"]
			} else if !c.BoolArgs["true"] {
				// Don't record `.record false` in history
				c.Record = false
			}
			cliFlagHelper(c, out, func(f *Flags) *bool { return &f.Record })
		},
	},
	{ // alias
		HelpShort: "create an alias",
		HelpLong: `
Create a new alias similar to bash aliases. Aliases can be used as a shortcut
to avoid typing out a long command. Only one alias is applied per command and
only to the beginning of a command. For example:

.alias vmr=.filter state=running vm info

The alias is interpreted as the text up to the first "=". Runing .alias without
any argument will list the existing aliases.

This alias allows the user to type "vmr" rather than the using a filter to list
the running VMs.

.unalias removes a previously set alias.

Note: we *strongly* recommend that you avoid aliases, unless you are using the
shell interactively. Aliases save typing which should not be necessary if you
are writing a script.`,
		Patterns: []string{
			".alias",
			".alias <alias>...",
		},
		Call: cliAlias,
	},
	{ // unalias
		HelpShort: "remove an alias",
		HelpLong: `
Removes an alias by name. See .alias for a listing of aliases.`,
		Patterns: []string{
			".unalias <alias>",
		},
		Call: cliUnalias,
	},
}

var hostname string

// Match tests is a case-insensitive match for s. If s is a list or an object,
// then enables fuzzy filtering implicitly.
func (f filter) Match(s string) bool {
	s = strings.ToLower(s)

	fuzzy := hasPrefixSuffix(s, "[", "]") || hasPrefixSuffix(s, "{", "}")
	fuzzy = fuzzy || f.Fuzzy

	match := (s == f.Val) || (fuzzy && strings.Contains(s, f.Val))
	return f.Negate != match
}

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

func runSubCommand(c *Command) <-chan Responses {
	pipe := make(chan Responses)

	go func() {
		c.Subcommand.Call(c.Subcommand, pipe)
		close(pipe)
	}()

	return pipe
}

// hasPrefixSuffix wraps strings.HasPrefix and strings.HasSuffix.
func hasPrefixSuffix(s, prefix, suffix string) bool {
	return strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)
}

func parseFilter(s string) (filter, error) {
	filters := []struct {
		Sep    string
		Filter filter
	}{
		// Important that negate versions come first for proper Split
		{"!~", filter{Fuzzy: true, Negate: true}},
		{"~", filter{Fuzzy: true}},
		{"!=", filter{Negate: true}},
		{"=", filter{}},
	}

	for _, f := range filters {
		parts := strings.Split(s, f.Sep)
		if len(parts) == 2 {
			f.Filter.Col = strings.ToLower(parts[0])
			f.Filter.Val = strings.ToLower(parts[1])
			return f.Filter, nil
		}
	}

	return filter{}, errors.New("invalid filter, see help")
}

// filterResp filters Response r based on the filter f. Returns bool for
// whether to keep the response or not or an error.
func filterResp(f filter, r *Response) (bool, error) {
	// HAX: Special case for when the column name is host which is not part of
	// the actual tabular data.
	if f.Col == "host" {
		return f.Match(r.Host), nil
	}

	// Can't filter if it's a non-tabular response
	if r.Header == nil || r.Tabular == nil {
		return true, nil
	}

	for i, header := range r.Header {
		if strings.ToLower(header) != f.Col {
			continue
		}

		// Found right column, filter tabular rows
		tabular := [][]string{}

		for _, row := range r.Tabular {
			if f.Match(row[i]) {
				tabular = append(tabular, row)
			}
		}

		r.Tabular = tabular
		return true, nil
	}

	// Didn't find the requested column in the responses
	return false, fmt.Errorf("no such column `%s`", f.Col)
}

func cliFilter(c *Command, out chan<- Responses) {
	f, err := parseFilter(c.StringArgs["filter"])
	if err != nil {
		resp := &Response{
			Host:  hostname,
			Error: err.Error(),
		}
		out <- Responses{resp}
		return
	}

outer:
	for resps := range runSubCommand(c) {
		newResps := Responses{}

		for _, r := range resps {
			keep, err := filterResp(f, r)

			if err != nil {
				resp := &Response{Host: hostname, Error: err.Error()}
				out <- Responses{resp}
				continue outer
			}

			if keep {
				newResps = append(newResps, r)
			}
		}

		out <- newResps
	}
}

func cliColumns(c *Command, out chan<- Responses) {
	columns := strings.Split(c.StringArgs["columns"], ",")

outer:
	for resps := range runSubCommand(c) {
		for _, r := range resps {
			if r.Header == nil {
				continue
			}

			if r.Tabular == nil {
				r.Header = columns
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

func cliModeHelper(c *Command, out chan<- Responses, newMode int) {
	if c.Subcommand == nil {
		resp := &Response{
			Host: hostname,
		}

		flagsLock.Lock()
		defer flagsLock.Unlock()

		if c.BoolArgs["true"] {
			defaultFlags.Mode = newMode
		} else if c.BoolArgs["false"] && defaultFlags.Mode == newMode {
			defaultFlags.Mode = defaultMode
		} else {
			resp.Response = strconv.FormatBool(defaultFlags.Mode == newMode)
		}

		out <- Responses{resp}
		return
	}

	for r := range runSubCommand(c) {
		if len(r) > 0 {
			if r[0].Flags == nil {
				r[0].Flags = copyFlags()
			}

			if c.BoolArgs["true"] {
				r[0].Mode = newMode
			} else if c.BoolArgs["false"] && r[0].Mode == newMode {
				r[0].Mode = defaultMode
			}

			out <- r
		}
	}
}

func cliFlagHelper(c *Command, out chan<- Responses, get func(*Flags) *bool) {
	if c.Subcommand == nil {
		resp := &Response{
			Host: hostname,
		}

		flagsLock.Lock()
		defer flagsLock.Unlock()

		if c.BoolArgs["true"] || c.BoolArgs["false"] {
			// Update the flag, can just get value for "true" since the default
			// value is false.
			*get(&defaultFlags) = c.BoolArgs["true"]
		} else {
			resp.Response = strconv.FormatBool(*get(&defaultFlags))
		}

		out <- Responses{resp}
		return
	}

	for r := range runSubCommand(c) {
		if len(r) > 0 {
			if r[0].Flags == nil {
				r[0].Flags = copyFlags()
			}

			*get(r[0].Flags) = c.BoolArgs["true"]
		}

		out <- r
	}
}

func cliAlias(c *Command, out chan<- Responses) {
	aliasesLock.Lock()
	defer aliasesLock.Unlock()

	resp := &Response{Host: hostname}

	alias := strings.Join(c.ListArgs["alias"], " ")
	parts := strings.SplitN(alias, "=", 2)

	if alias == "" {
		resp.Header = []string{"alias", "expansion"}

		for k, v := range aliases {
			resp.Tabular = append(resp.Tabular, []string{k, v})
		}
	} else if len(parts) != 2 {
		resp.Error = "expected alias of format `alias=expansion`"
	} else {
		aliases[parts[0]] = parts[1]
	}

	out <- Responses{resp}
	return
}

func cliUnalias(c *Command, out chan<- Responses) {
	aliasesLock.Lock()
	defer aliasesLock.Unlock()

	// don't care if doesn't exist
	delete(aliases, c.StringArgs["alias"])

	resp := &Response{Host: hostname}

	out <- Responses{resp}
	return
}
